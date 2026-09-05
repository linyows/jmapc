package jmapc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is an Observer that keeps what it was told.
type recorder struct {
	mu        sync.Mutex
	requests  []RequestInfo
	responses []ResponseInfo
	attempts  []AttemptInfo
	answers   []Answer
	waits     []WaitInfo
}

func (r *recorder) observer() *Observer {
	return &Observer{
		Request: func(ctx context.Context, info RequestInfo) (context.Context, func(ResponseInfo)) {
			r.mu.Lock()
			r.requests = append(r.requests, info)
			r.mu.Unlock()
			return ctx, func(done ResponseInfo) {
				r.mu.Lock()
				r.responses = append(r.responses, done)
				r.mu.Unlock()
			}
		},
		Attempt: func(ctx context.Context, info AttemptInfo) (context.Context, func(AttemptInfo, Answer)) {
			return ctx, func(info AttemptInfo, answer Answer) {
				r.mu.Lock()
				r.attempts = append(r.attempts, info)
				r.answers = append(r.answers, answer)
				r.mu.Unlock()
			}
		},
		Wait: func(ctx context.Context, info WaitInfo) func() {
			r.mu.Lock()
			r.waits = append(r.waits, info)
			r.mu.Unlock()
			return func() {}
		},
	}
}

func (r *recorder) kinds() []RequestKind {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []RequestKind
	for _, a := range r.attempts {
		out = append(out, a.Kind)
	}
	return out
}

func TestObserverSeesTheCallsARequestCarries(t *testing.T) {
	ts := newTestServer(t)
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[["Mailbox/get",{"list":[]},"one"]]}`)
	}
	rec := &recorder{}
	c := ts.client(WithObserver(rec.observer()))

	_, err := c.Do(context.Background(), &Request{
		Using: []string{CapabilityCore, CapabilityMail},
		MethodCalls: []Invocation{
			{Name: "Mailbox/get", CallID: "one", Args: map[string]any{"accountId": "a1"}},
		},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if len(rec.requests) != 1 {
		t.Fatalf("the observer saw %d requests, want 1", len(rec.requests))
	}
	req := rec.requests[0]
	if len(req.Calls) != 1 || req.Calls[0].Name != "Mailbox/get" || req.Calls[0].CallID != "one" {
		t.Errorf("calls = %+v, want the one Mailbox/get under the id one", req.Calls)
	}
	if len(req.Using) != 2 {
		t.Errorf("using = %v, want the two capabilities the request declared", req.Using)
	}
	if len(rec.responses) != 1 {
		t.Fatalf("the observer saw %d answers, want 1", len(rec.responses))
	}
	if rec.responses[0].Err != nil {
		t.Errorf("the answer carried an error: %v", rec.responses[0].Err)
	}
	if rec.responses[0].Duration <= 0 {
		t.Error("the answer carried no duration")
	}
}

// The session fetch is a request of its own, and the observer should see it as
// one: it is a round trip the caller did not ask for and cannot otherwise see.
func TestObserverSeesEachHTTPRequest(t *testing.T) {
	ts := newTestServer(t)
	rec := &recorder{}
	c := ts.client(WithObserver(rec.observer()))

	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	want := []RequestKind{KindSession, KindAPI}
	got := rec.kinds()
	if len(got) != len(want) {
		t.Fatalf("attempts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attempt %d was for %q, want %q", i, got[i], want[i])
		}
	}
	for i, answer := range rec.answers {
		if answer.Status != http.StatusOK {
			t.Errorf("attempt %d came back %d, want 200", i, answer.Status)
		}
	}
}

// A retry is another attempt, and the wait before it is a wait the caller
// paid for without being able to see it.
func TestObserverSeesARetryAndItsWait(t *testing.T) {
	ts := newTestServer(t)
	var hits int
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[]}`)
	}
	rec := &recorder{}
	c := ts.client(WithObserver(rec.observer()), WithRetryPolicy(RetryPolicy{
		Attempts: 2,
		Wait:     func(int, time.Duration) time.Duration { return time.Millisecond },
	}))

	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	var api []AttemptInfo
	for _, a := range rec.attempts {
		if a.Kind == KindAPI {
			api = append(api, a)
		}
	}
	if len(api) != 2 {
		t.Fatalf("the observer saw %d API attempts, want 2", len(api))
	}
	if api[0].Attempt != 1 || api[1].Attempt != 2 {
		t.Errorf("attempts were numbered %d and %d, want 1 and 2", api[0].Attempt, api[1].Attempt)
	}
	if len(rec.waits) != 1 {
		t.Fatalf("the observer saw %d waits, want the one before the retry", len(rec.waits))
	}
	if rec.waits[0].Reason != WaitAfterRefusal {
		t.Errorf("the wait was for %q, want %q", rec.waits[0].Reason, WaitAfterRefusal)
	}
	if rec.waits[0].Delay <= 0 {
		t.Error("the wait carried no delay, and the client knew how long it would be")
	}
}

// A request that goes straight through waited for nothing, and reporting a
// wait of no length would bury the ones that matter.
func TestObserverIsToldOfNoWaitWhereThereWasNone(t *testing.T) {
	ts := newTestServer(t)
	rec := &recorder{}
	c := ts.client(WithObserver(rec.observer()))

	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(rec.waits) != 0 {
		t.Errorf("the observer was told of %d waits, want none", len(rec.waits))
	}
}

// The method errors a server reports are the ones a caller most wants counted,
// and they arrive with a 200 that no HTTP-level instrumentation would flag.
func TestObserverSeesMethodErrors(t *testing.T) {
	ts := newTestServer(t)
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[["error",{"type":"unknownMethod"},"one"]]}`)
	}
	rec := &recorder{}
	c := ts.client(WithObserver(rec.observer()))

	_, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Nope/get", CallID: "one", Args: map[string]any{}}},
	})
	if err == nil {
		t.Fatal("Do returned no error for a call the server refused")
	}
	if len(rec.responses) != 1 {
		t.Fatalf("the observer saw %d answers, want 1", len(rec.responses))
	}
	if len(rec.responses[0].Errors) != 1 {
		t.Fatalf("the answer carried %d method errors, want 1", len(rec.responses[0].Errors))
	}
	if rec.responses[0].Err != nil {
		t.Errorf("a request answered with a method error was reported as failing: %v", rec.responses[0].Err)
	}
}

func TestSlogObserverWritesWhatWasSent(t *testing.T) {
	ts := newTestServer(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := ts.client(WithObserver(SlogObserver(logger)))

	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	var sawRequest, sawAttempt bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the log record is not JSON: %v", err)
		}
		switch record["msg"] {
		case "jmap request":
			sawRequest = true
			calls, _ := record["calls"].([]any)
			if len(calls) != 1 || calls[0] != "Core/echo" {
				t.Errorf("the record names %v, want the one Core/echo", record["calls"])
			}
		case "http attempt":
			sawAttempt = true
		}
	}
	if !sawRequest || !sawAttempt {
		t.Errorf("the log holds a request record: %v, and an attempt record: %v\n%s",
			sawRequest, sawAttempt, buf.String())
	}
}

// An observer with nothing filled in must not be a way to crash a client.
func TestAnEmptyObserverIsHarmless(t *testing.T) {
	ts := newTestServer(t)
	c := ts.client(WithObserver(&Observer{}))
	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

// Waiting for a slot is time the caller spends inside Do with nothing to show
// for it, so it is the wait most worth reporting.
func TestObserverSeesTheWaitForASlot(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {"maxConcurrentRequests": 1}},
		  "accounts": {}, "primaryAccounts": {}, "username": "someone",
		  "apiUrl": %q, "state": "s1"
		}`, srv.URL+"/api")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, `{"sessionState":"s1","methodResponses":[]}`)
	})

	rec := &recorder{}
	c := New(srv.URL+"/.well-known/jmap", WithObserver(rec.observer()))
	if _, err := c.Session(context.Background()); err != nil {
		t.Fatalf("the session was not readable: %v", err)
	}

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Do(context.Background(), echo()); err != nil {
				t.Errorf("the request failed: %v", err)
			}
		}()
	}
	wg.Wait()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.waits) == 0 {
		t.Fatal("the observer was told of no wait, and two of three requests had to wait for the slot")
	}
	for _, wait := range rec.waits {
		if wait.Reason != WaitForSlot {
			t.Errorf("the wait was for %q, want %q", wait.Reason, WaitForSlot)
		}
		if wait.Kind != KindAPI {
			t.Errorf("the wait was on a %q request, want %q", wait.Kind, KindAPI)
		}
	}
}

type markKey struct{}

// A tracer starts a span for the request and another for each attempt, and the
// second is only a child of the first where the context the first was started
// under reaches it.
func TestTheContextARequestHookReturnsReachesTheAttempts(t *testing.T) {
	ts := newTestServer(t)
	var marks []string
	c := ts.client(WithObserver(&Observer{
		Request: func(ctx context.Context, info RequestInfo) (context.Context, func(ResponseInfo)) {
			return context.WithValue(ctx, markKey{}, "request"), nil
		},
		Attempt: func(ctx context.Context, info AttemptInfo) (context.Context, func(AttemptInfo, Answer)) {
			mark, _ := ctx.Value(markKey{}).(string)
			marks = append(marks, string(info.Kind)+":"+mark)
			return ctx, nil
		},
	}))

	if _, err := c.Do(context.Background(), echo()); err != nil {
		t.Fatalf("Do: %v", err)
	}

	want := []string{"session:request", "api:request"}
	if len(marks) != len(want) {
		t.Fatalf("attempts = %v, want %v", marks, want)
	}
	for i := range want {
		if marks[i] != want[i] {
			t.Errorf("attempt %d ran under %q, want %q", i, marks[i], want[i])
		}
	}
}
