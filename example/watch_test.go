package example

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/example/jmapq"
)

// watchStub is a JMAP server with a push endpoint: it answers each request
// according to the state it was asked from, pushes one event, and then holds
// the connection open, as a server with nothing to say does.
type watchStub struct {
	t *testing.T
	// asked records the sinceState of every Email/changes it answered.
	asked []string
}

func (s *watchStub) start() *httptest.Server {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	s.t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
		  "accounts": {"acct1": {"name": "someone@example.com", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "acct1"},
		  "username": "someone@example.com",
		  "apiUrl": %q,
		  "eventSourceUrl": %q,
		  "state": "session-1"
		}`, srv.URL+"/jmap/api/", srv.URL+"/events?types={types}&closeafter={closeafter}&ping={ping}")
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "id: e1\nevent: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"acct1\":{\"Email\":\"s2\"}}}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Nothing else is going to happen, and a stream that ended would only
		// make the watch reconnect.
		<-r.Context().Done()
	})

	mux.HandleFunc("/jmap/api/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Errorf("reading the request: %v", err)
			return
		}
		var req struct {
			MethodCalls []json.RawMessage `json:"methodCalls"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.t.Errorf("the request is not JSON: %v", err)
			return
		}
		var changes []any
		if err := json.Unmarshal(req.MethodCalls[0], &changes); err != nil {
			s.t.Errorf("the first call is not an invocation: %v", err)
			return
		}
		since, _ := changes[1].(map[string]any)["sinceState"].(string)
		s.asked = append(s.asked, since)

		// Nothing has changed until the event says so, and then one message
		// has been created.
		created, list, newState := `[]`, `[]`, "s1"
		if since != "s1" {
			s.t.Errorf("asked from %q, want s1 both times", since)
		}
		if len(s.asked) > 1 {
			created, newState = `["m1"]`, "s2"
			list = `[{"id": "m1", "threadId": "t1", "mailboxIds": {"mbx1": true},
			          "keywords": {}, "subject": "The first one",
			          "receivedAt": "2026-09-04T09:00:00Z"}]`
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"sessionState": "session-1", "methodResponses": [
		  ["Email/changes", {"accountId": "acct1", "oldState": "s1", "newState": %q,
		                     "hasMoreChanges": false, "created": %s, "updated": [], "destroyed": []}, "changes"],
		  ["Email/get", {"accountId": "acct1", "state": %q, "list": %s, "notFound": []}, "created"],
		  ["Email/get", {"accountId": "acct1", "state": %q, "list": [], "notFound": []}, "updated"]
		]}`, newState, created, newState, list, newState)
	})
	return srv
}

// TestSyncEmailsWatch exercises the loop a watching query generates: it catches
// up on connecting, waits to be told the type has moved on, and catches up
// again, threading the state through as it goes.
func TestSyncEmailsWatch(t *testing.T) {
	s := &watchStub{t: t}
	srv := s.start()
	c := jmapc.New(srv.URL+"/.well-known/jmap", jmapc.WithBearerToken("token"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var subjects []string
	err := jmapq.SyncEmailsWatch(ctx, c, jmapq.SyncEmailsParams{SinceState: "s1"},
		func(ctx context.Context, res *jmapq.SyncEmailsResult) error {
			for _, email := range res.Created.List {
				subjects = append(subjects, *email.Subject)
			}
			// The second catch-up is the one the event caused, and there is
			// nothing further to wait for.
			if res.Changes.NewState == "s2" {
				cancel()
			}
			return nil
		})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncEmailsWatch: %v", err)
	}
	if len(s.asked) != 2 {
		t.Errorf("caught up %d times, want twice: on connecting, and on the event", len(s.asked))
	}
	if strings.Join(subjects, ",") != "The first one" {
		t.Errorf("the watch reported %v, want the one created message", subjects)
	}
}

// TestSyncEmailsWatchStopsOnTheCallersError checks that the caller decides when
// to stop: an error from the callback ends the loop and comes back as it was.
func TestSyncEmailsWatchStopsOnTheCallersError(t *testing.T) {
	s := &watchStub{t: t}
	srv := s.start()
	c := jmapc.New(srv.URL+"/.well-known/jmap", jmapc.WithBearerToken("token"))

	full := errors.New("the cache is full")
	err := jmapq.SyncEmailsWatch(context.Background(), c, jmapq.SyncEmailsParams{SinceState: "s1"},
		func(context.Context, *jmapq.SyncEmailsResult) error { return full })
	if !errors.Is(err, full) {
		t.Fatalf("SyncEmailsWatch: %v, want the caller's error", err)
	}
}
