package jmapc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// watchServer serves a session advertising a push endpoint, and one event
// stream per connection: a body the test wrote, and then a connection that
// stays open rather than being reopened forever once the test has run out of
// things to push.
type watchServer struct {
	*httptest.Server
	mu sync.Mutex
	// bodies are served to successive connections, in order.
	bodies []string
	// lastIDs records what each connection resumed from.
	lastIDs []string
	// status is served instead of a stream, so that a refusal can be
	// exercised.
	status int
}

func newWatchServer(t *testing.T, bodies ...string) *watchServer {
	t.Helper()
	ws := &watchServer{bodies: bodies, status: http.StatusOK}
	mux := http.NewServeMux()
	ws.Server = httptest.NewServer(mux)
	t.Cleanup(ws.Close)

	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone",
		  "apiUrl": %q,
		  "eventSourceUrl": %q,
		  "state": "sess1"
		}`, ws.URL+"/api", ws.URL+"/events?types={types}&closeafter={closeafter}&ping={ping}")
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		ws.mu.Lock()
		attempt := len(ws.lastIDs)
		ws.lastIDs = append(ws.lastIDs, r.Header.Get("Last-Event-ID"))
		var body string
		if attempt < len(ws.bodies) {
			body = ws.bodies[attempt]
		}
		status := ws.status
		last := attempt >= len(ws.bodies)-1
		ws.mu.Unlock()

		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			fmt.Fprint(w, `{"type":"urn:ietf:params:jmap:error:forbidden"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// The last stream the test wrote is held open, so that a watch with
		// nothing left to read waits rather than reconnecting in a loop.
		if last {
			<-r.Context().Done()
		}
	})
	return ws
}

// connections returns how many times the watch subscribed, and what each
// connection resumed from.
func (ws *watchServer) connections() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return append([]string(nil), ws.lastIDs...)
}

// event renders one state change as the server pushes it.
func event(id, accountID, typeName, state string) string {
	return fmt.Sprintf("id: %s\nevent: state\ndata: {\"@type\":\"StateChange\",\"changed\":{%q:{%q:%q}}}\n\n",
		id, accountID, typeName, state)
}

// answers replies to successive catch-ups, and stops the watch once the last
// of them has been used, so that a test says what it expects to happen rather
// than how long to wait for it.
type answers struct {
	steps []step
	seen  []string
	stop  context.CancelFunc
}

type step struct {
	newState string
	more     bool
	err      error
}

func (a *answers) catchUp(ctx context.Context, since string) (string, bool, error) {
	a.seen = append(a.seen, since)
	if len(a.seen) >= len(a.steps) {
		defer a.stop()
	}
	s := a.steps[min(len(a.seen)-1, len(a.steps)-1)]
	return s.newState, s.more, s.err
}

// watch runs a watch against the server until the answers run out, and returns
// the states each catch-up was asked about.
func watch(t *testing.T, ws *watchServer, state string, steps ...step) ([]string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := &answers{steps: steps, stop: cancel}
	err := New(ws.URL+"/session").Watch(ctx, "a1", "Email", state, a.catchUp,
		// A test has no use for the wait that keeps a real client from
		// hammering a server that is down.
		WithReconnect(func(int) time.Duration { return 0 }))
	return a.seen, err
}

// TestWatchCatchesUpOnConnectingAndOnEvents covers the shape of the thing: what
// changed while there was no connection was pushed to nobody, so a connection
// is followed by a catch-up, and an event that says the type has moved on is
// followed by another.
func TestWatchCatchesUpOnConnectingAndOnEvents(t *testing.T) {
	ws := newWatchServer(t, event("s1", "a1", "Email", "e2"))
	seen, err := watch(t, ws, "e1",
		step{newState: "e1"}, // nothing happened while the client was away
		step{newState: "e2"}, // and then the server said Email had moved on
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch: %v", err)
	}
	if len(seen) != 2 || seen[0] != "e1" || seen[1] != "e1" {
		t.Errorf("caught up from %v, want two catch-ups from e1", seen)
	}
	if conns := ws.connections(); len(conns) != 1 {
		t.Errorf("subscribed %d times, want once", len(conns))
	}
}

// TestWatchAsksAgainWhileThereIsMore covers a server that answers with part of
// what changed and says so: the state threads through the calls, and the watch
// keeps asking until the server stops saying there is more.
func TestWatchAsksAgainWhileThereIsMore(t *testing.T) {
	ws := newWatchServer(t)
	seen, err := watch(t, ws, "e1",
		step{newState: "e2", more: true},
		step{newState: "e3", more: true},
		step{newState: "e4"},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch: %v", err)
	}
	want := []string{"e1", "e2", "e3"}
	if len(seen) != len(want) {
		t.Fatalf("caught up from %v, want %v", seen, want)
	}
	for i, state := range want {
		if seen[i] != state {
			t.Errorf("catch-up %d asked from %q, want %q", i, seen[i], state)
		}
	}
}

// TestWatchIgnoresWhatIsNotItsOwn checks the events a watch has no use for: one
// about another account, one about another type, and one about a state it has
// already reached, which is what its own catch-up made the server push.
func TestWatchIgnoresWhatIsNotItsOwn(t *testing.T) {
	ws := newWatchServer(t,
		event("s1", "a2", "Email", "e9")+
			event("s2", "a1", "Mailbox", "m2")+
			event("s3", "a1", "Email", "e2")+
			event("s4", "a1", "Email", "e3"),
	)
	seen, err := watch(t, ws, "e1",
		step{newState: "e2"}, // on connecting, which is the state s3 announces
		step{newState: "e3"}, // so only s4 is worth another look
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch: %v", err)
	}
	if len(seen) != 2 {
		t.Errorf("caught up %d times from %v, want twice", len(seen), seen)
	}
}

// TestWatchReconnects covers the fact a stream is a connection rather than a
// subscription: when it drops, the watch opens another, says where it got to,
// and asks what it missed.
func TestWatchReconnects(t *testing.T) {
	ws := newWatchServer(t, event("s1", "a1", "Email", "e2"), "")
	seen, err := watch(t, ws, "e1",
		step{newState: "e1"}, // on connecting
		step{newState: "e2"}, // on the event
		step{newState: "e3"}, // on connecting again, after the stream dropped
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch: %v", err)
	}
	if len(seen) != 3 {
		t.Errorf("caught up %d times from %v, want three times", len(seen), seen)
	}
	conns := ws.connections()
	if len(conns) != 2 {
		t.Fatalf("subscribed %d times, want twice", len(conns))
	}
	if conns[0] != "" {
		t.Errorf("the first connection resumed from %q, want nothing", conns[0])
	}
	if conns[1] != "s1" {
		t.Errorf("the second connection resumed from %q, want s1", conns[1])
	}
}

// TestWatchStopsOnTheCallersError checks that the caller decides: an error from
// the catch-up ends the watch and comes back as it was.
func TestWatchStopsOnTheCallersError(t *testing.T) {
	ws := newWatchServer(t)
	refused := errors.New("the cache would not take it")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := New(ws.URL+"/session").Watch(ctx, "a1", "Email", "e1",
		func(context.Context, string) (string, bool, error) { return "", false, refused })
	if !errors.Is(err, refused) {
		t.Fatalf("Watch: %v, want the caller's error", err)
	}
}

// TestWatchStopsOnARefusal checks that a watch tells apart a server that is
// unreachable from one that is refusing: waiting fixes the first and not the
// second.
func TestWatchStopsOnARefusal(t *testing.T) {
	ws := newWatchServer(t)
	ws.status = http.StatusForbidden
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := New(ws.URL+"/session").Watch(ctx, "a1", "Email", "e1",
		func(context.Context, string) (string, bool, error) { return "", false, nil },
		WithReconnect(func(int) time.Duration { return 0 }))
	var reqErr *RequestError
	if !errors.As(err, &reqErr) || reqErr.Status != http.StatusForbidden {
		t.Fatalf("Watch: %v, want the refusal", err)
	}
	if conns := ws.connections(); len(conns) != 1 {
		t.Errorf("subscribed %d times, want the one refusal to be enough", len(conns))
	}
}

// TestBackoffDoubles checks the wait between attempts, which doubles so that a
// server that is down is not asked every second for as long as it is down.
func TestBackoffDoubles(t *testing.T) {
	for attempt, want := range map[int]time.Duration{
		1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second,
		6: 30 * time.Second, 20: 30 * time.Second,
	} {
		if got := backoff(attempt); got != want {
			t.Errorf("backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}
