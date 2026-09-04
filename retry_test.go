package jmapc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// answer is what a test server sends for one request.
type answer struct {
	status int
	// after is the Retry-After header, sent where it is not empty.
	after string
	body  string
}

// answering serves a session and then works through the answers, repeating the
// last one once they run out.
func answering(t *testing.T, answers ...answer) (*Client, *[]string, *atomic.Int64) {
	t.Helper()
	var bodies []string
	var requests atomic.Int64
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {}, "primaryAccounts": {}, "username": "someone",
		  "apiUrl": %q, "state": "s1"
		}`, srv.URL+"/api")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := int(requests.Add(1))
		bodies = append(bodies, string(body))
		a := answers[min(n, len(answers))-1]
		if a.after != "" {
			w.Header().Set("Retry-After", a.after)
		}
		if a.status != 0 && a.status != http.StatusOK {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(a.status)
			fmt.Fprint(w, `{"type":"urn:ietf:params:jmap:error:limit","limit":"maxConcurrentRequests"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if a.body == "" {
			a.body = `{"sessionState":"s1","methodResponses":[]}`
		}
		fmt.Fprint(w, a.body)
	})
	// Three attempts, and no waiting between them: what the waits are is
	// settled where they are worked out rather than here.
	return New(srv.URL+"/.well-known/jmap", WithRetryPolicy(RetryPolicy{
		Attempts: 3,
		Wait:     func(int, time.Duration) time.Duration { return 0 },
	})), &bodies, &requests
}

// echo is a request to send, the smallest one there is.
func echo() *Request {
	return &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "c0", Args: map[string]any{"hello": true}}},
	}
}

// TestRetryOnBeingTurnedAway covers the two answers that say the server did
// nothing: it is asked again, and the caller sees the answer that worked.
func TestRetryOnBeingTurnedAway(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		c, bodies, requests := answering(t, answer{status: status}, answer{status: http.StatusOK})
		if _, err := c.Do(context.Background(), echo()); err != nil {
			t.Fatalf("%d: the request failed: %v", status, err)
		}
		if n := requests.Load(); n != 2 {
			t.Errorf("%d: the request was sent %d times, want twice", status, n)
		}
		// The body has to be sent again as it was, which is what the retry is
		// for; a request sent again empty would be answered rather than
		// refused, and the test would pass on nothing.
		if (*bodies)[0] == "" || (*bodies)[0] != (*bodies)[1] {
			t.Errorf("%d: the second request carried %q, want the first again", status, (*bodies)[1])
		}
	}
}

// TestRetryGivesUp checks that a server saying no throughout is reported rather
// than asked forever.
func TestRetryGivesUp(t *testing.T) {
	c, _, requests := answering(t, answer{status: http.StatusTooManyRequests})
	_, err := c.Do(context.Background(), echo())
	var reqErr *RequestError
	if !errors.As(err, &reqErr) || reqErr.Status != http.StatusTooManyRequests {
		t.Fatalf("the request answered %v, want the refusal", err)
	}
	if n := requests.Load(); n != 3 {
		t.Errorf("the request was sent %d times, want the three attempts it was allowed", n)
	}
}

// TestWhatIsNotRetried covers the answers a client must not repeat a request
// over. A request that may have been carried out is not safe to send again: a
// /set sent twice creates twice.
func TestWhatIsNotRetried(t *testing.T) {
	c, _, requests := answering(t, answer{status: http.StatusInternalServerError})
	if _, err := c.Do(context.Background(), echo()); err == nil {
		t.Fatal("the request answered nothing, want the failure")
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("a request that may have been carried out was sent %d times", n)
	}
}

// TestRetryAfterBeyondWaiting checks the server that is not asking the client
// to wait but telling the caller to come back later.
func TestRetryAfterBeyondWaiting(t *testing.T) {
	c, _, requests := answering(t,
		answer{status: http.StatusTooManyRequests, after: "3600"},
		answer{status: http.StatusOK})
	if _, err := c.Do(context.Background(), echo()); err == nil {
		t.Fatal("the request answered nothing, want the refusal")
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("the request was sent %d times, want the one refusal to stand", n)
	}
}

// TestRetryAfter covers the header as RFC 9110 writes it, in seconds and as a
// date, and the answers that say nothing at all.
func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"soon", 0},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
	}
	for _, c := range cases {
		resp := &http.Response{Header: http.Header{}}
		if c.value != "" {
			resp.Header.Set("Retry-After", c.value)
		}
		if got := retryAfter(resp, now); got != c.want {
			t.Errorf("Retry-After %q read as %v, want %v", c.value, got, c.want)
		}
	}
}

// TestDefaultWait checks the wait a policy uses where the server asked for
// nothing: it doubles, so a server that is busy is not asked again every
// fifth of a second for as long as it is.
func TestDefaultWait(t *testing.T) {
	var p RetryPolicy
	if got := p.wait(2, 2*time.Second); got != 2*time.Second {
		t.Errorf("with a Retry-After the wait is %v, want what the server asked for", got)
	}
	for attempt, want := range map[int]time.Duration{
		2: 200 * time.Millisecond, 3: 400 * time.Millisecond, 4: 800 * time.Millisecond,
		10: 30 * time.Second, 20: 30 * time.Second,
	} {
		if got := p.wait(attempt, 0); got != want {
			t.Errorf("the wait before attempt %d is %v, want %v", attempt, got, want)
		}
	}
}

// TestConcurrentRequestsAreHeldToWhatTheServerTakes checks that the client
// waits for a slot rather than being refused: RFC 8620 has the server say how
// many requests it takes at once, and a client that ignores it is asking for a
// 429 it could have avoided.
func TestConcurrentRequestsAreHeldToWhatTheServerTakes(t *testing.T) {
	const limit = 2
	var inFlight, most atomic.Int64
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {"maxConcurrentRequests": %d}},
		  "accounts": {}, "primaryAccounts": {}, "username": "someone",
		  "apiUrl": %q, "state": "s1"
		}`, limit, srv.URL+"/api")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		now := inFlight.Add(1)
		for {
			seen := most.Load()
			if now <= seen || most.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"sessionState":"s1","methodResponses":[]}`)
	})

	c := New(srv.URL + "/.well-known/jmap")
	// The session says the limit, so it has to have been read before the
	// requests that are held to it.
	if _, err := c.Session(context.Background()); err != nil {
		t.Fatalf("the session was not readable: %v", err)
	}

	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Do(context.Background(), echo()); err != nil {
				t.Errorf("the request failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := most.Load(); got > limit {
		t.Errorf("%d requests were in flight at once, and the server takes %d", got, limit)
	}
}

// TestNoLimitWithoutASession checks that a client told not to look at the
// session before sending is not made to fetch one in order to count.
func TestNoLimitWithoutASession(t *testing.T) {
	ts := newTestServer(t)
	c := New(ts.URL+"/.well-known/jmap", WithAPIURL(ts.URL+"/api"), WithoutPreflightChecks())
	if _, err := c.Do(context.Background(), echo()); err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	if n := ts.sessionHits.Load(); n != 0 {
		t.Errorf("the session was fetched %d times by a client told not to", n)
	}
}
