package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// serverSaying serves a session with the capabilities and limits a test wants
// to check a query against, and counts what it was asked for.
func serverSaying(t *testing.T, capabilities, accounts, primary string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprintf(w, `{
		  "capabilities": %s,
		  "accounts": %s,
		  "primaryAccounts": %s,
		  "username": "someone",
		  "apiUrl": %q,
		  "state": "sess1"
		}`, capabilities, accounts, primary, srv.URL+"/api")
	})
	return srv, &hits
}

// mailServer serves a session that would take the queries these tests write.
func mailServer(t *testing.T, core string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	return serverSaying(t,
		fmt.Sprintf(`{"urn:ietf:params:jmap:core": %s, "urn:ietf:params:jmap:mail": {}}`, core),
		`{"a1": {"name": "someone", "isPersonal": true}}`,
		`{"urn:ietf:params:jmap:mail": "a1"}`)
}

// TestCheckAgainstAServer covers the checks a build cannot make: what this
// server supports, and how much of it it does at once.
func TestCheckAgainstAServer(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/ListMailboxes.jmap.json": listMailboxes})
	queries := filepath.Join(dir, "queries")

	srv, _ := mailServer(t, `{"maxCallsInRequest": 16}`)
	out, _, err := capture(t, []string{"check", "-queries", queries, "-session", srv.URL + "/.well-known/jmap"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "checked 1 query against") || !strings.Contains(out, "as someone") {
		t.Errorf("check said %q, want it to name the server and the user", out)
	}
}

// TestCheckReportsWhatTheServerWouldRefuse checks a query that is right about
// JMAP and wrong about the server in front of it.
func TestCheckReportsWhatTheServerWouldRefuse(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/ListMailboxes.jmap.json": listMailboxes})
	queries := filepath.Join(dir, "queries")

	srv, _ := serverSaying(t,
		`{"urn:ietf:params:jmap:core": {}}`,
		`{"a1": {"name": "someone"}}`,
		`{}`)
	_, problems, err := capture(t, []string{"check", "-queries", queries, "-session", srv.URL + "/.well-known/jmap"})
	if err == nil {
		t.Fatal("expected the check to fail")
	}
	if !strings.Contains(err.Error(), "the server would not accept") {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(problems, "does not advertise urn:ietf:params:jmap:mail") {
		t.Errorf("the problems were reported as %q", problems)
	}
}

// TestCheckReachesNothingWithoutBeingAsked checks that a check stays a local
// check unless the command line says otherwise. The credentials are read from
// the environment, but the server is not: a build that reaches the network
// because of what is set around it is a build that fails somewhere it has
// never been told about.
func TestCheckReachesNothingWithoutBeingAsked(t *testing.T) {
	dir := workspace(t, map[string]string{"queries/ListMailboxes.jmap.json": listMailboxes})
	srv, hits := mailServer(t, `{}`)
	t.Setenv("JMAP_SESSION_URL", srv.URL+"/.well-known/jmap")

	out, _, err := capture(t, []string{"check", "-queries", filepath.Join(dir, "queries")})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "checked 1 query\n") {
		t.Errorf("check said %q", out)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the check asked the server %d times without being told to", n)
	}
}
