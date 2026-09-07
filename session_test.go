package jmapc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveSessionURL(t *testing.T) {
	base, err := url.Parse("https://example.com/jmap/session")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"empty is left alone", "", ""},
		{"already absolute is left alone", "https://other.example.com/jmap", "https://other.example.com/jmap"},
		{"path-absolute resolves against the host", "/jmap", "https://example.com/jmap"},
		{
			"a URI template's braces survive resolution",
			"/jmap/download/{accountId}/{blobId}/{name}?accept={type}",
			"https://example.com/jmap/download/{accountId}/{blobId}/{name}?accept={type}",
		},
		{"protocol-relative keeps the base scheme", "//other.example.com/jmap", "https://other.example.com/jmap"},
		{"relative resolves against the session URL's directory", "api", "https://example.com/jmap/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSessionURL(base, tt.ref); got != tt.want {
				t.Errorf("resolveSessionURL(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

// stateServer is a JMAP server whose session changes: the state it carries,
// the account it names, and whether the session resource answers at all.
type stateServer struct {
	*httptest.Server
	// sessionHits counts requests for the session resource, which is what
	// shows whether the client fetched it again.
	sessionHits atomic.Int64
	// state is the session's state, and the sessionState every response
	// carries with it.
	state atomic.Value
	// account is the id of the one account the session names.
	account atomic.Value
	// broken makes the session resource answer 500, standing in for a server
	// that cannot be asked for its session just now.
	broken atomic.Bool
	// slow holds the session resource open long enough for the requests
	// behind the first one to arrive while it is still being fetched.
	slow atomic.Bool
}

func newStateServer(t *testing.T) *stateServer {
	t.Helper()
	ts := &stateServer{}
	ts.state.Store("sess1")
	ts.account.Store("a1")
	mux := http.NewServeMux()
	ts.Server = httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		ts.sessionHits.Add(1)
		if ts.slow.Load() {
			time.Sleep(50 * time.Millisecond)
		}
		if ts.broken.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		account := ts.account.Load().(string)
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
		  "accounts": {%q: {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": %q},
		  "username": "someone",
		  "apiUrl": %q,
		  "state": %q
		}`, account, account, ts.URL+"/api", ts.state.Load().(string))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"sessionState":%q,"methodResponses":[]}`, ts.state.Load().(string))
	})
	return ts
}

func (ts *stateServer) client(opts ...Option) *Client {
	return New(ts.URL+"/.well-known/jmap", opts...)
}

// change is the server's session changing: a new account, reported under a new
// state.
func (ts *stateServer) change(state, account string) {
	ts.state.Store(state)
	ts.account.Store(account)
}

// request sends a request whose response carries the server's sessionState.
func (ts *stateServer) request(t *testing.T, c *Client) {
	t.Helper()
	if _, err := c.Do(context.Background(), &Request{Using: []string{CapabilityCore}}); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

// TestSessionIsFetchedAgainWhenAResponseReportsAChange covers the account that
// moved: the session naming the old one is answered against until a response
// says the session has changed, and the client fetches it again.
func TestSessionIsFetchedAgainWhenAResponseReportsAChange(t *testing.T) {
	ts := newStateServer(t)
	c := ts.client()
	ctx := context.Background()

	first, err := c.Session(ctx)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if id, _ := first.PrimaryAccountID(CapabilityMail); id != "a1" {
		t.Fatalf("the primary account is %q, want a1", id)
	}

	ts.change("sess2", "a2")
	ts.request(t, c)

	next, err := c.Session(ctx)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if id, _ := next.PrimaryAccountID(CapabilityMail); id != "a2" {
		t.Errorf("the primary account is %q, want a2: the session was not fetched again", id)
	}
	if got := ts.sessionHits.Load(); got != 2 {
		t.Errorf("the session was fetched %d times, want 2", got)
	}
}

// TestSessionIsKeptWhereNothingChanged checks that the session a server keeps
// reporting the same state for is fetched once, so that following changes
// costs nothing where there are none.
func TestSessionIsKeptWhereNothingChanged(t *testing.T) {
	ts := newStateServer(t)
	c := ts.client()

	for range 3 {
		ts.request(t, c)
	}
	if _, err := c.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got := ts.sessionHits.Load(); got != 1 {
		t.Errorf("the session was fetched %d times, want 1", got)
	}
}

// TestWithoutSessionRefreshKeepsTheSessionItStartedWith checks the opt-out.
func TestWithoutSessionRefreshKeepsTheSessionItStartedWith(t *testing.T) {
	ts := newStateServer(t)
	c := ts.client(WithoutSessionRefresh())
	ctx := context.Background()

	ts.request(t, c)
	ts.change("sess2", "a2")
	ts.request(t, c)

	s, err := c.Session(ctx)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if id, _ := s.PrimaryAccountID(CapabilityMail); id != "a1" {
		t.Errorf("the primary account is %q, want a1: the session was fetched again", id)
	}
	if got := ts.sessionHits.Load(); got != 1 {
		t.Errorf("the session was fetched %d times, want 1", got)
	}
	// The session the client was told to keep is still the one an explicit
	// refresh replaces.
	if _, err := c.RefreshSession(ctx); err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}
	if s, _ := c.Session(ctx); s.State != "sess2" {
		t.Errorf("the session is at %q after RefreshSession, want sess2", s.State)
	}
}

// TestSessionSurvivesAFailedRefresh covers the server that cannot be asked for
// its session just when the client learns that it changed. The session held is
// out of date, which is what it was a moment ago, so the request goes on rather
// than failing on the fetch.
func TestSessionSurvivesAFailedRefresh(t *testing.T) {
	ts := newStateServer(t)
	c := ts.client()
	ctx := context.Background()

	ts.request(t, c)
	ts.change("sess2", "a2")
	ts.request(t, c)
	ts.broken.Store(true)

	s, err := c.Session(ctx)
	if err != nil {
		t.Fatalf("Session returned an error rather than the session it holds: %v", err)
	}
	if id, _ := s.PrimaryAccountID(CapabilityMail); id != "a1" {
		t.Errorf("the primary account is %q, want the a1 the client already held", id)
	}

	// The session is still out of date, so the next caller tries again, and
	// this time the server answers.
	ts.broken.Store(false)
	s, err = c.Session(ctx)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if id, _ := s.PrimaryAccountID(CapabilityMail); id != "a2" {
		t.Errorf("the primary account is %q, want a2", id)
	}
}

// TestOneFetchServesTheRequestsThatArriveTogether checks that a change found by
// many requests at once costs one fetch of the session resource between them,
// rather than one each.
func TestOneFetchServesTheRequestsThatArriveTogether(t *testing.T) {
	ts := newStateServer(t)
	c := ts.client()

	ts.request(t, c)
	ts.change("sess2", "a2")
	ts.request(t, c)
	ts.slow.Store(true)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Session(context.Background()); err != nil {
				t.Errorf("Session: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := ts.sessionHits.Load(); got != 2 {
		t.Errorf("the session was fetched %d times, want 2: the requests did not share one fetch", got)
	}
}

// TestASessionWithoutAStateIsNotFetchedAgain covers the server that reports no
// state of its own. Nothing can be concluded from comparing against it, and
// comparing anyway would fetch the session after every response.
func TestASessionWithoutAStateIsNotFetchedAgain(t *testing.T) {
	ts := newStateServer(t)
	ts.state.Store("")
	c := ts.client()

	ts.request(t, c)
	ts.request(t, c)

	if got := ts.sessionHits.Load(); got != 1 {
		t.Errorf("the session was fetched %d times, want 1", got)
	}
}
