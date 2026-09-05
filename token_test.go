package jmapc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer records the Authorization header of every request, and refuses
// the tokens a test tells it to.
type tokenServer struct {
	*testServer
	mu       sync.Mutex
	seen     []string
	rejected map[string]bool
}

func newTokenServer(t *testing.T) *tokenServer {
	t.Helper()
	ts := &tokenServer{testServer: newTestServer(t), rejected: map[string]bool{}}
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		auth := r.Header.Get("Authorization")
		ts.seen = append(ts.seen, auth)
		refused := ts.rejected[auth]
		ts.mu.Unlock()
		if refused {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[]}`)
	}
	return ts
}

func (ts *tokenServer) refuse(auth string) {
	ts.mu.Lock()
	ts.rejected[auth] = true
	ts.mu.Unlock()
}

func (ts *tokenServer) headers() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.seen...)
}

func TestTokenSourceAuthenticatesTheRequest(t *testing.T) {
	ts := newTokenServer(t)
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		return Token{Value: "t1"}, nil
	}))
	if _, err := c.Do(context.Background(), echo()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := ts.headers(); len(got) != 1 || got[0] != "Bearer t1" {
		t.Errorf("the server saw %v, want one Bearer t1", got)
	}
}

// A token with no expiry is held until a server refuses it, so a source that
// reads one from a file is not read on every request.
func TestTokenSourceIsCalledOnceForATokenThatHolds(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int64
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		calls.Add(1)
		return Token{Value: "t1"}, nil
	}))
	for range 3 {
		if _, err := c.Do(context.Background(), echo()); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("the source was called %d times, want 1", n)
	}
}

// A token is replaced before it expires rather than after it is refused, since
// one that expires while the request is in flight has already failed.
func TestTokenSourceIsCalledAgainBeforeExpiry(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int64
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		n := calls.Add(1)
		return Token{Value: fmt.Sprintf("t%d", n), Expiry: time.Now().Add(tokenMargin / 2)}, nil
	}))
	// The session is fetched first, and takes the first token with it.
	if _, err := c.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}
	for range 2 {
		if _, err := c.Do(context.Background(), echo()); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	want := []string{"Bearer t2", "Bearer t3"}
	got := ts.headers()
	if len(got) != len(want) {
		t.Fatalf("the server saw %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d carried %q, want %q", i, got[i], want[i])
		}
	}
}

// A source that reports no expiry learns the token has ended from the server,
// and the request that found out is sent again with the new one.
func TestARefusedTokenIsReplacedAndTheRequestSentAgain(t *testing.T) {
	ts := newTokenServer(t)
	ts.refuse("Bearer t1")
	var calls atomic.Int64
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		return Token{Value: fmt.Sprintf("t%d", calls.Add(1))}, nil
	}))
	if _, err := c.Do(context.Background(), echo()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	want := []string{"Bearer t1", "Bearer t2"}
	got := ts.headers()
	if len(got) != len(want) {
		t.Fatalf("the server saw %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d carried %q, want %q", i, got[i], want[i])
		}
	}
}

// Replacing the token twice would mean the source keeps returning one the
// server does not accept, so the refusal is reported instead of looping.
func TestARefusedTokenIsReplacedOnlyOnce(t *testing.T) {
	ts := newTokenServer(t)
	ts.refuse("Bearer t1")
	ts.refuse("Bearer t2")
	var calls atomic.Int64
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		return Token{Value: fmt.Sprintf("t%d", calls.Add(1))}, nil
	}))
	_, err := c.Do(context.Background(), echo())
	var reqErr *RequestError
	if !errors.As(err, &reqErr) || reqErr.Status != http.StatusUnauthorized {
		t.Fatalf("Do returned %v, want a 401 RequestError", err)
	}
	if n := len(ts.headers()); n != 2 {
		t.Errorf("the server saw %d requests, want 2", n)
	}
}

// A source that exchanges a refresh token cannot be asked to do so several
// times at once: some servers accept a refresh token only once.
func TestRequestsArrivingTogetherShareOneCallToTheSource(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int64
	release := make(chan struct{})
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		calls.Add(1)
		<-release
		return Token{Value: "t1"}, nil
	}))
	// The session is fetched first, so that the requests below race for the
	// token rather than for the session.
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(release)
	}()
	if _, err := c.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Do(context.Background(), echo()); err != nil {
				t.Errorf("Do: %v", err)
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("the source was called %d times, want 1", n)
	}
}

func TestAFailingTokenSourceStopsTheRequest(t *testing.T) {
	ts := newTokenServer(t)
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		return Token{}, errors.New("the refresh token has been revoked")
	}))
	_, err := c.Do(context.Background(), echo())
	if err == nil {
		t.Fatal("Do succeeded with a token source that failed")
	}
	if !strings.Contains(err.Error(), "the refresh token has been revoked") {
		t.Errorf("error = %v, want it to carry what the source reported", err)
	}
	if n := len(ts.headers()); n != 0 {
		t.Errorf("the server saw %d requests, want none", n)
	}
}

func TestAnEmptyTokenIsReported(t *testing.T) {
	ts := newTokenServer(t)
	c := ts.client(WithTokenSource(func(context.Context) (Token, error) {
		return Token{}, nil
	}))
	_, err := c.Do(context.Background(), echo())
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("Do returned %v, want an error about an empty token", err)
	}
}
