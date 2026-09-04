package jmaptest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linyows/jmapc"
)

// recorder stands in for the test a server was given, so that a test about
// what the server reports can read it rather than fail because of it.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (r *recorder) said(what string) bool {
	for _, e := range r.errs {
		if strings.Contains(e, what) {
			return true
		}
	}
	return false
}

// send makes one request to the server and returns the response.
func send(t *testing.T, c *jmapc.Client, calls ...jmapc.Invocation) (*jmapc.Response, error) {
	t.Helper()
	return c.Do(context.Background(), &jmapc.Request{
		Using:       []string{jmapc.CapabilityCore, jmapc.CapabilityMail},
		MethodCalls: calls,
	})
}

// TestBackReferencesAreResolved covers what a stub written by hand usually
// does not: the argument one call leaves to the server is filled in from the
// answer to the call before it, so a chained query reaches the handler with the
// ids in it.
func TestBackReferencesAreResolved(t *testing.T) {
	srv := New(t)
	srv.Reply("Email/query", map[string]any{
		"accountId": AccountID, "queryState": "q1", "position": 0,
		"ids": []string{"m1", "m2"},
	})
	var got []jmapc.ID
	srv.Handle("Email/get", func(c *Call) (any, error) {
		got = c.IDs()
		return map[string]any{"accountId": AccountID, "state": "s1", "notFound": []string{},
			"list": []map[string]any{{"id": "m1"}, {"id": "m2"}}}, nil
	})

	if _, err := send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{"accountId": AccountID}},
		jmapc.Invocation{Name: "Email/get", CallID: "fetch", Args: map[string]any{
			"accountId": AccountID,
			"#ids":      jmapc.ResultReference{ResultOf: "search", Name: "Email/query", Path: "/ids"},
		}},
	); err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	if len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Errorf("the get was given %v, want the ids the query answered with", got)
	}
	// What the client asked for is there to be asserted on as well as what it
	// was given.
	if ref, ok := srv.Call("Email/get").Reference("ids"); !ok || ref.Path != "/ids" {
		t.Errorf("the back reference was not recorded: %v", ref)
	}
	if n := srv.Requests(); n != 1 {
		t.Errorf("the two calls took %d requests, want one", n)
	}
}

// TestPointer covers the paths a back reference selects with, including the
// one JMAP adds to JSON pointer: "*" maps the rest over an array and flattens
// what comes back by one level.
func TestPointer(t *testing.T) {
	var response any
	if err := json.Unmarshal([]byte(`{
	  "ids": ["m1", "m2"],
	  "list": [{"id": "m1", "threadId": "t1", "attachments": [{"blobId": "b1"}, {"blobId": "b2"}]},
	           {"id": "m2", "threadId": "t2", "attachments": [{"blobId": "b3"}]}],
	  "created": {"draft": {"id": "m9"}}
	}`), &response); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	cases := []struct{ path, want string }{
		{"/ids", `["m1","m2"]`},
		{"/list/*/id", `["m1","m2"]`},
		{"/list/0/threadId", `"t1"`},
		{"/list/*/attachments/*/blobId", `["b1","b2","b3"]`},
		{"/created/draft/id", `"m9"`},
	}
	for _, c := range cases {
		selected, err := pointer(response, c.path)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		got, err := json.Marshal(selected)
		if err != nil {
			t.Fatalf("encoding: %v", err)
		}
		if string(got) != c.want {
			t.Errorf("%s selected %s, want %s", c.path, got, c.want)
		}
	}
	for _, path := range []string{"/nothing", "/list/9/id", "/ids/*/id"} {
		if _, err := pointer(response, path); err == nil {
			t.Errorf("%s selected something, want an error", path)
		}
	}
}

// TestNonsenseFailsTheTest covers the reason for checking at all: a server that
// took whatever it was given would let a client send something no server would
// accept and call the test passed.
func TestNonsenseFailsTheTest(t *testing.T) {
	rec := &recorder{TB: t}
	srv := New(rec)
	srv.Reply("Email/query", map[string]any{"accountId": AccountID})

	_, err := send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{
			"accountId": AccountID,
			"filter":    map[string]any{"hasAttachmnt": true},
		}})
	var methodErrs jmapc.MethodErrors
	if !errors.As(err, &methodErrs) {
		t.Fatalf("the request answered %v, want the call refused", err)
	}
	if !rec.said("hasAttachmnt") {
		t.Errorf("the test was not told what was wrong: %v", rec.errs)
	}
}

// TestUncheckedServerTakesAnything covers the way out, for a method jmapc has
// never heard of.
func TestUncheckedServerTakesAnything(t *testing.T) {
	rec := &recorder{TB: t}
	srv := New(rec, WithoutChecks())
	srv.Reply("Email/query", map[string]any{"accountId": AccountID, "queryState": "q", "position": 0, "ids": []string{}})

	if _, err := send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{
			"accountId": AccountID, "filter": map[string]any{"hasAttachmnt": true},
		}}); err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	if len(rec.errs) != 0 {
		t.Errorf("a server told not to check reported %v", rec.errs)
	}
}

// TestUnansweredCallFailsTheTest checks what happens when the test forgot to
// say what a method answers, which is the mistake this package makes easiest.
func TestUnansweredCallFailsTheTest(t *testing.T) {
	rec := &recorder{TB: t}
	srv := New(rec)
	if _, err := send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{"accountId": AccountID}}); err == nil {
		t.Error("the call was answered by nothing and reported nothing")
	}
	if !rec.said("nothing answers Email/query") {
		t.Errorf("the test was not told what was missing: %v", rec.errs)
	}
}

// TestRefusalsReachTheClient covers the two levels of failure a server reports:
// one call refused, and a request the server would not look at.
func TestRefusalsReachTheClient(t *testing.T) {
	srv := New(t)
	srv.Fail("Email/query", "accountNotFound")
	_, err := send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{"accountId": AccountID}})
	var methodErrs jmapc.MethodErrors
	if !errors.As(err, &methodErrs) || methodErrs[0].Type != "accountNotFound" {
		t.Fatalf("the call answered %v, want accountNotFound", err)
	}

	srv.FailRequest(&jmapc.RequestError{Status: http.StatusTooManyRequests, Type: jmapc.ErrTypeLimit, Limit: "maxConcurrentRequests"})
	_, err = send(t, srv.Client(),
		jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{"accountId": AccountID}})
	var reqErr *jmapc.RequestError
	if !errors.As(err, &reqErr) || reqErr.Limit != "maxConcurrentRequests" {
		t.Fatalf("the request answered %v, want the limit", err)
	}
}

// TestCapabilityTheSessionDoesNotHave checks a request declaring something the
// session does not advertise, which a server refuses whole.
func TestCapabilityTheSessionDoesNotHave(t *testing.T) {
	srv := New(t, WithSession(func(s *jmapc.Session) {
		delete(s.Capabilities, jmapc.CapabilityMail)
	}))
	// The client checks this before sending, so the check is turned off to see
	// what the server does with it.
	c := srv.Client(jmapc.WithoutPreflightChecks())
	_, err := send(t, c, jmapc.Invocation{Name: "Email/query", CallID: "search", Args: map[string]any{}})
	var reqErr *jmapc.RequestError
	if !errors.As(err, &reqErr) || reqErr.Type != jmapc.ErrTypeUnknownCapability {
		t.Fatalf("the request answered %v, want an unknown capability", err)
	}
}

// TestPushReachesAWatcher covers the push endpoint, which is what a watch waits
// on: an event says a type has moved on, and says nothing about what changed.
func TestPushReachesAWatcher(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := srv.Client().EventSource(ctx, &jmapc.EventSourceOptions{Types: []string{"Email"}})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	go srv.Push(AccountID, map[string]string{"Email": "s2"})

	change, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, ok := change.StateOf(AccountID, "Email"); !ok || state != "s2" {
		t.Errorf("the event said %q (present %v), want s2", state, ok)
	}
}
