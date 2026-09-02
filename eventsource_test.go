package jmapc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// eventServer serves a session advertising a push endpoint, and a stream whose
// body the test supplies.
type eventServer struct {
	*testServer
	// stream is the event-stream body served to a subscriber.
	stream string
	// requestURI and lastEventID record what the subscriber asked for.
	requestURI  string
	lastEventID string
	// status is the status served, so that a failure can be exercised.
	status int
}

func newEventServer(t *testing.T) *eventServer {
	t.Helper()
	es := &eventServer{status: http.StatusOK}
	ts := newTestServer(t)
	es.testServer = ts

	mux := ts.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		ts.sessionHits.Add(1)
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone",
		  "apiUrl": %q,
		  "eventSourceUrl": %q,
		  "state": "sess1"
		}`, ts.URL+"/api", ts.URL+"/events?types={types}&closeafter={closeafter}&ping={ping}")
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		es.requestURI = r.URL.RequestURI()
		es.lastEventID = r.Header.Get("Last-Event-ID")
		if es.status != http.StatusOK {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(es.status)
			fmt.Fprint(w, `{"type":"urn:ietf:params:jmap:error:limit","limit":"maxConcurrentRequests"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, es.stream)
	})
	return es
}

func (es *eventServer) client() *Client { return New(es.URL + "/session") }

func TestEventSource(t *testing.T) {
	es := newEventServer(t)
	es.stream = "" +
		": a comment keeping the connection warm\n" +
		"\n" +
		"event: ping\n" +
		"data: {\"interval\": 300}\n" +
		"\n" +
		"id: s1\n" +
		"event: state\n" +
		"data: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"e2\",\"Mailbox\":\"m2\"}}}\n" +
		"\n" +
		"id: s2\n" +
		"event: state\n" +
		"data: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"e3\"}}}\n" +
		"\n"

	stream, err := es.client().EventSource(context.Background(), &EventSourceOptions{
		Types: []string{"Email", "Mailbox"},
		Ping:  30 * time.Second,
	})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	want := "/events?types=Email%2CMailbox&closeafter=no&ping=30"
	if es.requestURI != want {
		t.Errorf("subscribed to %q, want %q", es.requestURI, want)
	}

	// The ping and the comment are consumed without being handed back.
	change, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, ok := change.StateOf("a1", "Email"); !ok || state != "e2" {
		t.Errorf("Email state = %q (present %v), want e2", state, ok)
	}
	if state, ok := change.StateOf("a1", "Mailbox"); !ok || state != "m2" {
		t.Errorf("Mailbox state = %q, want m2", state)
	}
	if _, ok := change.StateOf("a2", "Email"); ok {
		t.Error("an account the event did not mention reported a state")
	}
	if stream.LastEventID() != "s1" {
		t.Errorf("LastEventID = %q, want s1", stream.LastEventID())
	}

	change, err = stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, _ := change.StateOf("a1", "Email"); state != "e3" {
		t.Errorf("second Email state = %q, want e3", state)
	}
	if stream.LastEventID() != "s2" {
		t.Errorf("LastEventID = %q, want s2", stream.LastEventID())
	}

	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("after the stream ended Next returned %v, want io.EOF", err)
	}
}

// TestEventSourceDefaults checks the request made when nothing is asked for in
// particular: every type, no pings, and a connection that stays open.
func TestEventSourceDefaults(t *testing.T) {
	es := newEventServer(t)
	stream, err := es.client().EventSource(context.Background(), nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	stream.Close()
	want := "/events?types=%2A&closeafter=no&ping=0"
	if es.requestURI != want {
		t.Errorf("subscribed to %q, want %q", es.requestURI, want)
	}
}

// TestEventSourceResumes checks that a reconnection tells the server where the
// last one left off, which is what keeps events from being missed in between.
func TestEventSourceResumes(t *testing.T) {
	es := newEventServer(t)
	stream, err := es.client().EventSource(context.Background(), &EventSourceOptions{
		LastEventID:     "s7",
		CloseAfterState: true,
	})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	stream.Close()
	if es.lastEventID != "s7" {
		t.Errorf("Last-Event-ID = %q, want s7", es.lastEventID)
	}
	if !strings.Contains(es.requestURI, "closeafter=state") {
		t.Errorf("subscribed to %q, want closeafter=state", es.requestURI)
	}
	if stream.LastEventID() != "s7" {
		t.Errorf("LastEventID = %q, want the id it resumed from", stream.LastEventID())
	}
}

// TestEventSourceMultilineData checks that data split across lines is joined,
// as the event-stream format calls for.
func TestEventSourceMultilineData(t *testing.T) {
	es := newEventServer(t)
	es.stream = "event: state\n" +
		"data: {\"@type\":\"StateChange\",\n" +
		"data:  \"changed\":{\"a1\":{\"Email\":\"e5\"}}}\n" +
		"\n"

	stream, err := es.client().EventSource(context.Background(), nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	change, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, _ := change.StateOf("a1", "Email"); state != "e5" {
		t.Errorf("Email state = %q, want e5", state)
	}
}

// TestEventSourceMalformedData checks that a body that is not a state change is
// reported rather than passed on as an empty event.
func TestEventSourceMalformedData(t *testing.T) {
	es := newEventServer(t)
	es.stream = "event: state\ndata: not json\n\n"
	stream, err := es.client().EventSource(context.Background(), nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(); err == nil {
		t.Error("expected an error for a malformed event")
	}
}

func TestEventSourceRejected(t *testing.T) {
	es := newEventServer(t)
	es.status = http.StatusTooManyRequests
	_, err := es.client().EventSource(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T (%v), want *RequestError", err, err)
	}
	if reqErr.Limit != "maxConcurrentRequests" {
		t.Errorf("limit = %q, want maxConcurrentRequests", reqErr.Limit)
	}
}

// TestEventSourceUnavailable checks the error when the server has no push
// endpoint at all, which is allowed: push is optional.
func TestEventSourceUnavailable(t *testing.T) {
	ts := newTestServer(t)
	_, err := ts.client().EventSource(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "eventSourceUrl") {
		t.Errorf("error = %v, want it to mention eventSourceUrl", err)
	}
}
