package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linyows/jmapc"
)

// markRead leaves both the mailbox and the message to the caller, so that a run
// of it exercises a parameter in an argument and a parameter in a member name
// at once.
const markRead = `{
  "_doc": "MarkRead marks one message as read.",
  "methodCalls": [["Email/set", {"update": {"{{emailId}}": {"keywords/$seen": true}}}, "mark"]],
  "_returns": "mark"
}`

const searchMailboxes = `{
  "methodCalls": [["Mailbox/query", {"filter": {"name": "{{name}}"}, "limit": "{{limit}}"}, "search"]],
  "_returns": "search"
}`

// capture runs a command with the output redirected, and returns what it wrote.
func capture(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	stdout, stderr = &out, &errOut
	t.Cleanup(func() { stdout, stderr = io.Discard, io.Discard })
	err := run(args)
	return out.String(), errOut.String(), err
}

// server stands in for a JMAP server: it answers the session resource with one
// account, and hands the request to the handler the test gives it.
func server(t *testing.T, api func(w http.ResponseWriter, body []byte)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone",
		  "apiUrl": %q,
		  "state": "sess1"
		}`, srv.URL+"/api")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request: %v", err)
			return
		}
		api(w, body)
	})
	return srv
}

// TestRunDryRun checks the request a query stands for, which is what a dry run
// is for: seeing what would go out without anything going out.
func TestRunDryRun(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/SearchMailboxes.jmap.json": searchMailboxes})

	out, note, err := capture(t, []string{"run", "SearchMailboxes",
		"-queries", filepath.Join(dir, "queries"), "-dry-run", "-p", "name=Work", "-p", "limit=3"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var req jmapc.Request
	if err := json.Unmarshal([]byte(out), &req); err != nil {
		t.Fatalf("the request is not JSON: %v\n%s", err, out)
	}
	if len(req.MethodCalls) != 1 || req.MethodCalls[0].Name != "Mailbox/query" {
		t.Fatalf("method calls = %v", req.MethodCalls)
	}
	raw, _ := req.MethodCalls[0].RawArgs()
	var args bytes.Buffer
	if err := json.Compact(&args, raw); err != nil {
		t.Fatalf("the arguments are not JSON: %v", err)
	}
	for _, want := range []string{`"limit":3`, `"name":"Work"`, `"accountId":"` + accountPlaceholder + `"`} {
		if !strings.Contains(args.String(), want) {
			t.Errorf("the arguments do not contain %s:\n%s", want, args.String())
		}
	}
	// The account id is the one value a dry run cannot know, so it says so
	// rather than leaving the reader to wonder where it came from.
	if !strings.Contains(note, accountPlaceholder) {
		t.Errorf("nothing was said about the account id: %q", note)
	}
}

// TestRunSends checks the whole path: the parameters reach the server in the
// request, and the response reaches the terminal.
func TestRunSends(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/MarkRead.jmap.json": markRead})
	var got []byte
	srv := server(t, func(w http.ResponseWriter, body []byte) {
		got = body
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[
			["Email/set", {"accountId": "a1", "newState": "s2", "updated": {"m1": null}}, "mark"]]}`)
	})

	out, _, err := capture(t, []string{"run", "MarkRead",
		"-queries", filepath.Join(dir, "queries"), "-session", srv.URL + "/.well-known/jmap",
		"-token", "secret", "-p", "emailId=m1"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// The parameter names the record to update, which is a member name rather
	// than a value, and the account id came from the session.
	for _, want := range []string{`"m1":{`, `"accountId":"a1"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the request does not contain %s:\n%s", want, got)
		}
	}
	if !strings.Contains(out, `"newState": "s2"`) {
		t.Errorf("the response was not printed:\n%s", out)
	}
}

// TestRunReportsRefusals covers the failure that answers 200. A /set lists the
// records it would not act on, and a run that read only the transport error
// would call that a success.
func TestRunReportsRefusals(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/MarkRead.jmap.json": markRead})
	srv := server(t, func(w http.ResponseWriter, body []byte) {
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[
			["Email/set", {"accountId": "a1", "newState": "s1", "notUpdated":
				{"m1": {"type": "notFound"}}}, "mark"]]}`)
	})

	out, _, err := capture(t, []string{"run", "MarkRead",
		"-queries", filepath.Join(dir, "queries"), "-session", srv.URL + "/.well-known/jmap",
		"-p", "emailId=m1"})
	var refused *jmapc.SetErrors
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want a SetErrors", err)
	}
	if len(refused.Failures) != 1 || refused.Failures[0].Key != "m1" {
		t.Errorf("failures = %v", refused.Failures)
	}
	// The response is printed all the same: the part of the request the server
	// did carry out still happened.
	if !strings.Contains(out, "notUpdated") {
		t.Errorf("the response was not printed:\n%s", out)
	}
}

// TestRunChecksParameters checks that a value is held to the type of the
// argument it stands in for, before anything is sent.
func TestRunChecksParameters(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/SearchMailboxes.jmap.json": searchMailboxes})
	queries := filepath.Join(dir, "queries")

	_, _, err := capture(t, []string{"run", "SearchMailboxes", "-queries", queries,
		"-dry-run", "-p", "name=Work", "-p", "limit=soon"})
	if err == nil || !strings.Contains(err.Error(), "not a whole number") {
		t.Errorf("err = %v, want the limit to be rejected", err)
	}

	_, _, err = capture(t, []string{"run", "SearchMailboxes", "-queries", queries, "-dry-run"})
	if err == nil || !strings.Contains(err.Error(), "name (String), limit (UnsignedInt)") {
		t.Errorf("err = %v, want the parameters it takes", err)
	}

	_, _, err = capture(t, []string{"run", "SearchMailboxes", "-queries", queries,
		"-dry-run", "-p", "name=Work", "-p", "limit=3", "-p", "lmit=3"})
	if err == nil || !strings.Contains(err.Error(), `has no parameter "lmit"`) {
		t.Errorf("err = %v, want the misspelled parameter reported", err)
	}
}

// TestRunNamesTheQueriesItHas checks what a run says when the query is not
// there, since the case a shell completes to is easy to get wrong.
func TestRunNamesTheQueriesItHas(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/SearchMailboxes.jmap.json": searchMailboxes})
	_, _, err := capture(t, []string{"run", "searchmailboxes", "-queries", filepath.Join(dir, "queries")})
	if err == nil || !strings.Contains(err.Error(), "did you mean SearchMailboxes?") {
		t.Errorf("err = %v, want a suggestion", err)
	}
}

// TestRunWithoutAServer checks that a run with nowhere to send says so, rather
// than failing somewhere further in.
func TestRunWithoutAServer(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/SearchMailboxes.jmap.json": searchMailboxes})
	t.Setenv("JMAP_SESSION_URL", "")
	_, _, err := capture(t, []string{"run", "SearchMailboxes", "-queries", filepath.Join(dir, "queries"),
		"-p", "name=Work", "-p", "limit=3"})
	if err == nil || !strings.Contains(err.Error(), "no server to send to") {
		t.Errorf("err = %v, want the missing server reported", err)
	}
}
