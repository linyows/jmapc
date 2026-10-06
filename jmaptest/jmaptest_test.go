package jmaptest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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
// answer to the call before it, so a chained request reaches the handler with the
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
		t.Errorf("the get was given %v, want the ids the request answered with", got)
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

// TestPushFollowsTheTypesSubscribedTo checks that a client is pushed the types
// it asked for and no others, as RFC 8620, Section 7.3 has a server do. A
// client that forgot to filter would otherwise pass against the stub and fail
// against a server.
func TestPushFollowsTheTypesSubscribedTo(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := srv.Client().EventSource(ctx, &jmapc.EventSourceOptions{Types: []string{"Email"}})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	go func() {
		srv.Push(AccountID, map[string]string{"Mailbox": "m2"})
		srv.Push(AccountID, map[string]string{"Email": "e2", "Mailbox": "m3"})
	}()

	change, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, ok := change.StateOf(AccountID, "Email"); !ok || state != "e2" {
		t.Errorf("the event said %q (present %v) for Email, want e2", state, ok)
	}
	if state, ok := change.StateOf(AccountID, "Mailbox"); ok {
		t.Errorf("the event carried Mailbox at %q, which the client did not subscribe to", state)
	}
	// The push of Mailbox alone was sent nothing, so this event is the second
	// push, under an id of its own.
	if id := stream.LastEventID(); id != "e2" {
		t.Errorf("LastEventID = %q, want e2", id)
	}
}

// TestEachPushHasAnIDOfItsOwn checks the ids a client resumes from: two pushes
// between two requests are two events, and resuming after the first must not
// name the second.
func TestEachPushHasAnIDOfItsOwn(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := srv.Client().EventSource(ctx, nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	go func() {
		srv.Push(AccountID, map[string]string{"Email": "e2"})
		srv.Push(AccountID, map[string]string{"Email": "e3"})
	}()
	var ids []string
	for range 2 {
		if _, err := stream.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		ids = append(ids, stream.LastEventID())
	}
	if ids[0] == ids[1] {
		t.Errorf("both events have the id %q", ids[0])
	}
}

// TestAStreamClosesAfterAStateWhereAsked checks closeafter=state, with which a
// server ends the stream once it has pushed a change.
func TestAStreamClosesAfterAStateWhereAsked(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := srv.Client().EventSource(ctx, &jmapc.EventSourceOptions{CloseAfterState: true})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	go srv.Push(AccountID, map[string]string{"Email": "e2"})
	if _, err := stream.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next after the change: %v, want io.EOF", err)
	}
}

// TestAStreamIsPingedWhereAsked checks ping, with which a server writes an
// event at the interval asked for, so that a client can tell a quiet stream
// from a dropped one.
func TestAStreamIsPingedWhereAsked(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.BaseURL()+"/events?types=*&closeafter=no&ping=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	scan := bufio.NewScanner(resp.Body)
	for scan.Scan() {
		if scan.Text() == "event: ping" {
			return
		}
	}
	t.Errorf("the stream ended without a ping: %v", scan.Err())
}

// TestPathsOfYourOwnBesideThePathsItServes covers a client half converted to
// jmapc: the generated half reaches the server through Client, and the half
// still written by hand reaches paths of its own. Both have to answer in one
// test, which is what BaseURL and Mux are for.
func TestPathsOfYourOwnBesideThePathsItServes(t *testing.T) {
	srv := New(t)
	srv.Reply("Email/query", jmapc.EmailQueryResponse{
		AccountID: AccountID,
		IDs:       []jmapc.ID{"m1"},
	})

	// Where the half that has not been converted yet looks for the API,
	// wherever the project happened to put it.
	var oldCalls int
	srv.Mux().HandleFunc("/jmap", func(w http.ResponseWriter, r *http.Request) {
		oldCalls++
		fmt.Fprint(w, `{"sessionState":"old","methodResponses":[["Email/get",{},"r1"]]}`)
	})

	// The generated half.
	if _, err := send(t, srv.Client(), jmapc.Invocation{Name: "Email/query", CallID: "c0"}); err != nil {
		t.Fatalf("the generated half: %v", err)
	}

	// The half still written by hand, which knows nothing of jmapc and is
	// pointed at the same server by BaseURL.
	resp, err := http.Post(srv.BaseURL()+"/jmap", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("the half not converted yet: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the path of the test's own answered %s", resp.Status)
	}

	if oldCalls != 1 {
		t.Errorf("the path of the test's own answered %d requests, want 1", oldCalls)
	}
	if got := srv.Requests(); got != 1 {
		t.Errorf("jmaptest answered %d requests, want the 1 the generated half made", got)
	}
	if srv.Call("Email/query") == nil {
		t.Error("jmaptest did not record the call the generated half made")
	}
}

// TestServedWhereAClientLooks covers the other half of a migration: a client
// that speaks JMAP already but looks for it at addresses of its own, deriving
// both from a base URL. Mounting the handlers there serves them under both
// names, so such a client reaches this server without being rewritten first.
func TestServedWhereAClientLooks(t *testing.T) {
	srv := New(t)
	srv.Mux().HandleFunc("/jmap/session", srv.ServeSession)
	srv.Mux().HandleFunc("/jmap", srv.ServeAPI)
	srv.Reply("Email/query", jmapc.EmailQueryResponse{
		AccountID: AccountID,
		IDs:       []jmapc.ID{"m1"},
	})

	own := jmapc.New(srv.BaseURL()+"/jmap/session", jmapc.WithBearerToken("token"))
	if _, err := send(t, own, jmapc.Invocation{Name: "Email/query", CallID: "c0"}); err != nil {
		t.Fatalf("a client looking for the session at its own address: %v", err)
	}
	if srv.Call("Email/query") == nil {
		t.Error("the call did not reach the server")
	}

	// The API answers under its own name too, which is what a client that
	// posts straight to it uses.
	resp, err := http.Post(srv.BaseURL()+"/jmap", "application/json",
		strings.NewReader(`{"using":["urn:ietf:params:jmap:core","urn:ietf:params:jmap:mail"],`+
			`"methodCalls":[["Email/query",{"accountId":"account"},"c1"]]}`))
	if err != nil {
		t.Fatalf("posting to the API under its other name: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the API under its other name answered %s", resp.Status)
	}
	if got := srv.Requests(); got != 2 {
		t.Errorf("the server answered %d requests, want 2", got)
	}
}

// TestAPushToAConnectionThatEndedReturns checks a push that finds a client
// whose connection has ended since it was taken from the list, as one closed
// after a state is: the push is dropped rather than waiting for a reader that
// is gone.
func TestAPushToAConnectionThatEndedReturns(t *testing.T) {
	srv := New(t)
	ended := &watcher{events: make(chan string), done: make(chan struct{})}
	close(ended.done)
	srv.mu.Lock()
	srv.watchers[ended] = true
	srv.mu.Unlock()

	pushed := make(chan struct{})
	go func() {
		srv.Push(AccountID, map[string]string{"Email": "e2"})
		close(pushed)
	}()
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		t.Fatal("the push waited on a connection that had ended")
	}
}

// TestAPingIsSentAsTheJSONItSays checks the interval a ping reports, which is
// the number the server pings at rather than the text the client asked with:
// "01" is a number to ask with and not one JSON writes, and an interval past
// what a Duration holds is pinged at the most the server does, rather than
// taking the handler down.
func TestAPingIsSentAsTheJSONItSays(t *testing.T) {
	srv := New(t)
	for _, tt := range []struct{ ping, want string }{
		{"01", `{"interval":1}`},
		{"9223372037", `{"interval":86400}`},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			srv.BaseURL()+"/events?types=*&closeafter=no&ping="+tt.ping, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatalf("ping=%s: GET /events: %v", tt.ping, err)
		}
		if tt.ping == "01" {
			scan := bufio.NewScanner(resp.Body)
			for scan.Scan() && scan.Text() != "event: ping" {
			}
			if scan.Scan(); scan.Text() != "data: "+tt.want {
				t.Errorf("ping=%s: the ping said %q, want data: %s", tt.ping, scan.Text(), tt.want)
			}
		} else if resp.StatusCode != http.StatusOK {
			t.Errorf("ping=%s: answered %d, want the stream", tt.ping, resp.StatusCode)
		}
		resp.Body.Close()
		cancel()
	}
}

// TestAPingIsSentOnlyAfterAnIdleInterval checks that a ping waits for the
// interval to pass with nothing sent, as RFC 8620 has it: a state sent starts
// the interval again, rather than being followed at once by a ping.
func TestAPingIsSentOnlyAfterAnIdleInterval(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.BaseURL()+"/events?types=*&closeafter=no&ping=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	start := time.Now()
	go func() {
		time.Sleep(700 * time.Millisecond)
		srv.Push(AccountID, map[string]string{"Email": "e2"})
	}()
	scan := bufio.NewScanner(resp.Body)
	var state time.Time
	for scan.Scan() {
		switch scan.Text() {
		case "event: state":
			state = time.Now()
		case "event: ping":
			if state.IsZero() {
				t.Fatalf("a ping came %v in, before the state pushed at 700ms", time.Since(start))
			}
			if gap := time.Since(state); gap < 900*time.Millisecond {
				t.Errorf("a ping came %v after the state, want a full interval", gap)
			}
			return
		}
	}
	t.Errorf("the stream ended without a ping: %v", scan.Err())
}

// TestPushesMadeTogetherArriveInTheOrderOfTheirIDs checks pushes made at once
// from several goroutines: a client reads them in the order of their ids, so
// that resuming after one never skips another it has not read.
func TestPushesMadeTogetherArriveInTheOrderOfTheirIDs(t *testing.T) {
	srv := New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := srv.Client().EventSource(ctx, nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	const pushes = 50
	var wg sync.WaitGroup
	for i := range pushes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.Push(AccountID, map[string]string{"Email": fmt.Sprintf("e%d", i)})
		}()
	}
	last := 0
	for range pushes {
		if _, err := stream.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		var n int
		if _, err := fmt.Sscanf(stream.LastEventID(), "e%d", &n); err != nil {
			t.Fatalf("event id %q: %v", stream.LastEventID(), err)
		}
		if n <= last {
			t.Fatalf("event e%d came after e%d", n, last)
		}
		last = n
	}
	wg.Wait()
}
