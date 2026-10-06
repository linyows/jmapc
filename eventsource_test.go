package jmapc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
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

// TestEventSourceTemplateThatDoesNotExpand checks a push endpoint the session
// advertises in a form that cannot be used. Connecting again reads the same
// session, so the failure is as permanent as an endpoint that is missing.
func TestEventSourceTemplateThatDoesNotExpand(t *testing.T) {
	for _, template := range []string{
		"/events?types={types&ping={ping}",
		"/events?types={types}&since={since}",
	} {
		ts := newTestServer(t)
		ts.sessionHandler = fmt.Sprintf(`{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {}, "primaryAccounts": {}, "username": "someone",
		  "apiUrl": %q, "eventSourceUrl": %q, "state": "sess1"
		}`, ts.URL+"/api", ts.URL+template)
		_, err := ts.client().EventSource(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "expanding eventSourceUrl") {
			t.Errorf("%s: EventSource: %v, want the template reported", template, err)
			continue
		}
		if IsTemporary(err) {
			t.Errorf("%s: IsTemporary(%v) = true, want a template that does not expand to be permanent", template, err)
		}
	}
}

// TestEventSourceURLThatCannotBeRequested checks a push endpoint that expands
// into something that is not a URL. Like a template that does not expand, it is
// what the session says, and connecting again does not change it.
func TestEventSourceURLThatCannotBeRequested(t *testing.T) {
	ts := newTestServer(t)
	ts.sessionHandler = fmt.Sprintf(`{
	  "capabilities": {"urn:ietf:params:jmap:core": {}},
	  "accounts": {}, "primaryAccounts": {}, "username": "someone",
	  "apiUrl": %q, "eventSourceUrl": %q, "state": "sess1"
	}`, ts.URL+"/api", ts.URL+"/ev%zz?types={types}")
	_, err := ts.client().EventSource(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "building event source request") {
		t.Fatalf("EventSource: %v, want the URL reported", err)
	}
	if IsTemporary(err) {
		t.Errorf("IsTemporary(%v) = true, want a URL that cannot be requested to be permanent", err)
	}
}

// slowEventServer serves a session advertising a push endpoint, and hands the
// push endpoint to events, so that a test controls when each part of the
// stream is written.
func slowEventServer(t *testing.T, events http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {},
		  "username": "someone",
		  "apiUrl": %q,
		  "eventSourceUrl": %q,
		  "state": "sess1"
		}`, srv.URL+"/api", srv.URL+"/events")
	})
	mux.HandleFunc("/events", events)
	return srv
}

// TestEventSourceOutlivesTheClientTimeout checks that a stream stays open past
// the Timeout of the http.Client it was opened through. The http.Client counts
// reading the body against its Timeout, and an event stream's body does not
// end, so a client given a timeout, as WithHTTPClient suggests, had its stream
// cut each time the timeout ran out.
func TestEventSourceOutlivesTheClientTimeout(t *testing.T) {
	const timeout = 100 * time.Millisecond
	release := make(chan struct{})
	srv := slowEventServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"e1\"}}}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"e2\"}}}\n\n")
		w.(http.Flusher).Flush()
	})
	hc := &http.Client{Timeout: timeout}
	c := New(srv.URL+"/session", WithHTTPClient(hc))

	stream, err := c.EventSource(context.Background(), nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(); err != nil {
		t.Fatalf("first event: %v", err)
	}
	time.Sleep(3 * timeout)
	close(release)
	change, err := stream.Next()
	if err != nil {
		t.Fatalf("the stream was cut after the client's timeout: %v", err)
	}
	if state, _ := change.StateOf("a1", "Email"); state != "e2" {
		t.Errorf("second event state = %q, want e2", state)
	}
	if hc.Timeout != timeout {
		t.Errorf("the http.Client passed in has its Timeout changed to %v", hc.Timeout)
	}
}

// TestEventSourceGivesUpOnASilentServer checks that the Timeout still bounds the
// wait for the response, so that a server that accepts the connection and never
// answers is given up on, and the failure reads as one worth reconnecting
// after.
func TestEventSourceGivesUpOnASilentServer(t *testing.T) {
	const timeout = 100 * time.Millisecond
	srv := slowEventServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	c := New(srv.URL+"/session", WithHTTPClient(&http.Client{Timeout: timeout}))

	start := time.Now()
	_, err := c.EventSource(context.Background(), nil)
	if err == nil {
		t.Fatal("EventSource waited on a server that never answered and returned no error")
	}
	if elapsed := time.Since(start); elapsed > 20*timeout {
		t.Errorf("EventSource gave up after %v, want about %v", elapsed, timeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if !IsTemporary(err) {
		t.Errorf("IsTemporary(%v) = false, want true so that a watch reconnects", err)
	}
}

// TestPushVerificationDecodes covers the object a server posts to a push
// subscription's URL before it will send anything else. It arrives at the URL
// rather than through the API, so a client decodes it from a request body it
// received rather than from a method response.
func TestPushVerificationDecodes(t *testing.T) {
	body := `{
	  "@type": "PushVerification",
	  "pushSubscriptionId": "sub1",
	  "verificationCode": "b7cb4a4c8d1e"
	}`
	var v PushVerification
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if v.Type != "PushVerification" {
		t.Errorf("@type = %q", v.Type)
	}
	if v.PushSubscriptionID != "sub1" {
		t.Errorf("pushSubscriptionId = %q", v.PushSubscriptionID)
	}
	if v.VerificationCode != "b7cb4a4c8d1e" {
		t.Errorf("verificationCode = %q", v.VerificationCode)
	}
}

// pingingServer serves a stream that writes what script says, a step at a
// time, and then holds the connection open without writing anything, as a
// connection dropped somewhere between the two looks from here.
func pingingServer(t *testing.T, script func(w http.ResponseWriter, flush func())) *httptest.Server {
	t.Helper()
	return slowEventServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		flusher.Flush()
		script(w, flusher.Flush)
		<-r.Context().Done()
	})
}

// TestEventSourceNoticesAStreamThatStopsPinging checks that a stream that
// carries nothing for twice the ping interval fails Next, where it used to
// wait for ever on a connection nothing would arrive on.
func TestEventSourceNoticesAStreamThatStopsPinging(t *testing.T) {
	srv := pingingServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "event: ping\ndata: {\"interval\": 1}\n\n")
		flush()
	})
	c := New(srv.URL + "/session")
	stream, err := c.EventSource(context.Background(), &EventSourceOptions{Ping: time.Second})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()

	start := time.Now()
	_, err = stream.Next()
	if err == nil || !strings.Contains(err.Error(), "taken as lost") {
		t.Fatalf("Next returned %v, want the silence reported", err)
	}
	if took := time.Since(start); took < 1500*time.Millisecond || took > 5*time.Second {
		t.Errorf("the silence was noticed after %v, want about twice the second", took)
	}
	if !IsTemporary(err) {
		t.Errorf("IsTemporary(%v) = false, want true so that a watch reconnects", err)
	}
}

// TestEventSourceKeepsToTheIntervalTheServerPingsAt checks a server that pings
// less often than it was asked to, as one that clamps the interval does: the
// interval its pings report is the one the stream waits by.
func TestEventSourceKeepsToTheIntervalTheServerPingsAt(t *testing.T) {
	srv := pingingServer(t, func(w http.ResponseWriter, flush func()) {
		// Asked for a second, the server pings every two; two seconds and a
		// half between pings would be taken as lost at the second asked for.
		for range 2 {
			fmt.Fprint(w, "event: ping\ndata: {\"interval\": 2}\n\n")
			flush()
			time.Sleep(2500 * time.Millisecond)
		}
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"s2\"}}}\n\n")
		flush()
	})
	c := New(srv.URL + "/session")
	stream, err := c.EventSource(context.Background(), &EventSourceOptions{Ping: time.Second})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	change, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if state, _ := change.StateOf("a1", "Email"); state != "s2" {
		t.Errorf("state = %q, want s2", state)
	}
}

// TestEventSourceWithoutPingsWaits checks that a stream asked for no pings is
// not taken as lost for being quiet: nothing was promised to arrive.
func TestEventSourceWithoutPingsWaits(t *testing.T) {
	srv := pingingServer(t, func(w http.ResponseWriter, flush func()) {
		time.Sleep(2500 * time.Millisecond)
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"s2\"}}}\n\n")
		flush()
	})
	c := New(srv.URL + "/session")
	stream, err := c.EventSource(context.Background(), nil)
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
}

// TestEventSourceAsksForWholeSeconds checks that a ping interval of less than
// a second is asked for as a second rather than as zero, which asks for none.
func TestEventSourceAsksForWholeSeconds(t *testing.T) {
	es := newEventServer(t)
	es.stream = "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{}}\n\n"
	stream, err := es.client().EventSource(context.Background(), &EventSourceOptions{Ping: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	stream.Close()
	if !strings.Contains(es.requestURI, "ping=1") {
		t.Errorf("the stream was asked for with %s, want ping=1", es.requestURI)
	}
}

// TestEventSourceIsNotLostWhileTheCallerIsBusy checks a caller that takes
// longer than twice the ping interval over what Next returned, as a watch
// catching up may, while the server goes on pinging: the pings wait in the
// stream for the next call, and the stream is not taken as lost for them not
// being read.
func TestEventSourceIsNotLostWhileTheCallerIsBusy(t *testing.T) {
	srv := pingingServer(t, func(w http.ResponseWriter, flush func()) {
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"s1\"}}}\n\n")
		flush()
		for range 7 {
			time.Sleep(500 * time.Millisecond)
			fmt.Fprint(w, "event: ping\ndata: {\"interval\": 1}\n\n")
			flush()
		}
		fmt.Fprint(w, "event: state\ndata: {\"@type\":\"StateChange\",\"changed\":{\"a1\":{\"Email\":\"s2\"}}}\n\n")
		flush()
	})
	c := New(srv.URL + "/session")
	stream, err := c.EventSource(context.Background(), &EventSourceOptions{Ping: time.Second})
	if err != nil {
		t.Fatalf("EventSource: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(); err != nil {
		t.Fatalf("the first Next: %v", err)
	}
	// Busy for longer than twice the interval, reading nothing.
	time.Sleep(3 * time.Second)
	change, err := stream.Next()
	if err != nil {
		t.Fatalf("the stream was taken as lost while the caller was busy: %v", err)
	}
	if state, _ := change.StateOf("a1", "Email"); state != "s2" {
		t.Errorf("state = %q, want s2", state)
	}
}

func TestPingInterval(t *testing.T) {
	for _, tt := range []struct {
		in, want time.Duration
	}{
		{0, 0},
		{-time.Second, 0},
		{300 * time.Millisecond, time.Second},
		{time.Second, time.Second},
		{1500 * time.Millisecond, 2 * time.Second},
		{30 * time.Second, 30 * time.Second},
		// Rounding up by adding would overflow, and come out negative.
		{time.Duration(math.MaxInt64), maxPing},
	} {
		if got := pingInterval(tt.in); got != tt.want {
			t.Errorf("pingInterval(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
