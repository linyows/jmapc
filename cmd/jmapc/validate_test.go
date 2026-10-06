package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// serverSaying serves a session with the capabilities and limits a test wants
// to check a request against, and counts what it was asked for.
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

// mailServer serves a session that would take the requests these tests write.
func mailServer(t *testing.T, core string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	return serverSaying(t,
		fmt.Sprintf(`{"urn:ietf:params:jmap:core": %s, "urn:ietf:params:jmap:mail": {}}`, core),
		`{"a1": {"name": "someone", "isPersonal": true}}`,
		`{"urn:ietf:params:jmap:mail": "a1"}`)
}

// TestValidateAgainstAServer covers the checks a build cannot make: what this
// server supports, and how much of it it does at once.
func TestValidateAgainstAServer(t *testing.T) {
	dir := workspace(t, map[string]string{"requests/ListMailboxes.jmap.json": listMailboxes})
	requests := filepath.Join(dir, "requests")

	srv, _ := mailServer(t, `{"maxCallsInRequest": 16}`)
	out, _, err := capture(t, []string{"validate", "-requests", requests, "-session", srv.URL + "/.well-known/jmap"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "validated 1 request against") || !strings.Contains(out, "as someone") {
		t.Errorf("check said %q, want it to name the server and the user", out)
	}
}

// TestValidateReportsWhatTheServerWouldRefuse checks a request that is right
// about JMAP and wrong about the server in front of it.
func TestValidateReportsWhatTheServerWouldRefuse(t *testing.T) {
	dir := workspace(t, map[string]string{"requests/ListMailboxes.jmap.json": listMailboxes})
	requests := filepath.Join(dir, "requests")

	srv, _ := serverSaying(t,
		`{"urn:ietf:params:jmap:core": {}}`,
		`{"a1": {"name": "someone"}}`,
		`{}`)
	_, problems, err := capture(t, []string{"validate", "-requests", requests, "-session", srv.URL + "/.well-known/jmap"})
	if err == nil {
		t.Fatal("expected validate to fail")
	}
	if !strings.Contains(err.Error(), "the server would not accept") {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(problems, "does not advertise urn:ietf:params:jmap:mail") {
		t.Errorf("the problems were reported as %q", problems)
	}
}

// TestValidateReachesNothingWithoutBeingAsked checks that validating stays local
// unless the command line says otherwise. The credentials are read from
// the environment, but the server is not: a build that reaches the network
// because of what is set around it is a build that fails somewhere it has
// never been told about.
func TestValidateReachesNothingWithoutBeingAsked(t *testing.T) {
	dir := workspace(t, map[string]string{"requests/ListMailboxes.jmap.json": listMailboxes})
	srv, hits := mailServer(t, `{}`)
	t.Setenv("JMAP_SESSION_URL", srv.URL+"/.well-known/jmap")

	out, _, err := capture(t, []string{"validate", "-requests", filepath.Join(dir, "requests")})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "validated 1 request\n") {
		t.Errorf("check said %q", out)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("validate asked the server %d times without being told to", n)
	}
}

// TestValidateRefusesWhatGeneratingRefuses checks the requests that only
// generating them shows to be wrong: a name the generator writes a file of its
// own under. Validate refuses them for the language it is given, as generate
// does, and writes nothing. Two names differing only in case are refused the
// same way, but cannot be written side by side on a file system that ignores
// case, so they are left to the generator's own tests.
func TestValidateRefusesWhatGeneratingRefuses(t *testing.T) {
	for _, tc := range []struct {
		lang  string
		names []string
		want  string
	}{
		{"go", []string{"Verify"}, "verifies the requests"},
		{"typescript", []string{"Client"}, "client.ts"},
		{"rust", []string{"Mod"}, "mod.rs"},
		{"rust", []string{"Types"}, "types"},
		{"go", []string{"Properties"}, "properties_gen.go"},
	} {
		t.Run(tc.lang+"/"+strings.Join(tc.names, "+"), func(t *testing.T) {
			// A set of properties, so that the Go client has a file for the
			// sets for a request named Properties to collide with.
			files := map[string]string{"requests/properties.json": `{"MailboxName": {"type": "Mailbox", "properties": ["id", "name"]}}`}
			for _, name := range tc.names {
				files["requests/"+name+".jmap.json"] = listMailboxes
			}
			dir := workspace(t, files)
			out := filepath.Join(dir, "client")
			_, _, err := capture(t, []string{"validate", "-requests", filepath.Join(dir, "requests"), "-lang", tc.lang, "-out", out})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("validate gave %v, want an error saying %q", err, tc.want)
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("validate wrote the generated client")
			}
		})
	}

	// The same name is free where the language generates nothing under it.
	dir := workspace(t, map[string]string{"requests/Client.jmap.json": listMailboxes})
	if _, errOut, err := capture(t, []string{"validate", "-requests", filepath.Join(dir, "requests"), "-lang", "go"}); err != nil {
		t.Errorf("validate refused a Go request named Client: %v\n%s", err, errOut)
	}
}

// TestValidateRefusesTwoKindsOfCredentials checks that validate settles its
// credentials as run does: -token and -user given together are refused.
func TestValidateRefusesTwoKindsOfCredentials(t *testing.T) {
	dir := workspace(t, map[string]string{"requests/ListMailboxes.jmap.json": listMailboxes})
	_, _, err := capture(t, []string{"validate", "-requests", filepath.Join(dir, "requests"),
		"-session", "http://127.0.0.1:1/.well-known/jmap", "-token", "t", "-user", "alice:pw"})
	if err == nil || !strings.Contains(err.Error(), "both given") {
		t.Errorf("err = %v, want the two flags refused", err)
	}
}
