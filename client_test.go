package jmapc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// testServer is a JMAP server that counts what it was asked for.
type testServer struct {
	*httptest.Server
	// sessionHits counts requests for the session resource, which shows
	// whether the client is caching it.
	sessionHits atomic.Int64
	// apiHits counts requests to the API endpoint.
	apiHits atomic.Int64
	// apiHandler answers API requests when set.
	apiHandler http.HandlerFunc
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{}
	mux := http.NewServeMux()
	ts.Server = httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		ts.sessionHits.Add(1)
		fmt.Fprintf(w, `{
		  "capabilities": {
		    "urn:ietf:params:jmap:core": {"maxCallsInRequest": 2},
		    "urn:ietf:params:jmap:mail": {}
		  },
		  "accounts": {"a1": {"name": "someone", "isPersonal": true, "isReadOnly": false}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone",
		  "apiUrl": %q,
		  "state": "sess1"
		}`, ts.URL+"/api")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		ts.apiHits.Add(1)
		if ts.apiHandler != nil {
			ts.apiHandler(w, r)
			return
		}
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[]}`)
	})
	return ts
}

func (ts *testServer) client(opts ...Option) *Client {
	return New(ts.URL+"/.well-known/jmap", opts...)
}

// TestSessionIsCached checks that the session is fetched once and reused, so
// that a request costs one round trip rather than two.
func TestSessionIsCached(t *testing.T) {
	ts := newTestServer(t)
	c := ts.client()
	ctx := context.Background()

	for range 3 {
		if _, err := c.Do(ctx, &Request{Using: []string{CapabilityCore}}); err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if got := ts.sessionHits.Load(); got != 1 {
		t.Errorf("the session was fetched %d times, want 1", got)
	}
	if got := ts.apiHits.Load(); got != 3 {
		t.Errorf("the API was called %d times, want 3", got)
	}

	if _, err := c.RefreshSession(ctx); err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}
	if got := ts.sessionHits.Load(); got != 2 {
		t.Errorf("after a refresh the session was fetched %d times, want 2", got)
	}
}

func TestPrimaryAccountID(t *testing.T) {
	ts := newTestServer(t)
	s, err := ts.client().Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	got, err := s.PrimaryAccountID(CapabilityMail)
	if err != nil {
		t.Fatalf("PrimaryAccountID: %v", err)
	}
	if got != "a1" {
		t.Errorf("primary account = %q, want a1", got)
	}
	if _, err := s.PrimaryAccountID("urn:ietf:params:jmap:calendars"); err == nil {
		t.Error("expected an error for a capability the server does not have")
	}
	core, err := s.Core()
	if err != nil {
		t.Fatalf("Core: %v", err)
	}
	if core.MaxCallsInRequest != 2 {
		t.Errorf("maxCallsInRequest = %d, want 2", core.MaxCallsInRequest)
	}
}

// TestPreflightRejectsTooManyCalls checks that a request the server has already
// said it will not accept fails locally.
// TestPreflightNamesAnEmptyCapabilitiesList checks that a server advertising
// no capabilities at all - a minimal server, or a test stub - gets a
// different message than a server that listed its capabilities and left this
// one out, so the message does not send the reader looking at the server for
// a decision the session never made.
func TestPreflightNamesAnEmptyCapabilitiesList(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"capabilities":{},"accounts":{},"primaryAccounts":{},"username":"u","apiUrl":"/api","state":"s"}`)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sessionState":"s","methodResponses":[]}`)
	})

	_, err := New(srv.URL+"/session").Do(context.Background(), &Request{Using: []string{CapabilityCore}})
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T, want *RequestError", err)
	}
	if !strings.Contains(reqErr.Detail, "no capabilities at all") {
		t.Errorf("detail = %q, want it to say the session lists no capabilities", reqErr.Detail)
	}
}

func TestPreflightRejectsTooManyCalls(t *testing.T) {
	ts := newTestServer(t)
	c := ts.client()

	_, err := c.Do(context.Background(), &Request{
		Using: []string{CapabilityCore},
		MethodCalls: []Invocation{
			{Name: "Email/get", CallID: "c0"},
			{Name: "Email/get", CallID: "c1"},
			{Name: "Email/get", CallID: "c2"},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T, want *RequestError", err)
	}
	if reqErr.Limit != "maxCallsInRequest" {
		t.Errorf("limit = %q, want maxCallsInRequest", reqErr.Limit)
	}
	if got := ts.apiHits.Load(); got != 0 {
		t.Errorf("the request was sent anyway (%d API calls)", got)
	}
}

// TestPreflightCanBeTurnedOff checks that a server under-reporting its limits
// does not block a request the caller knows is fine.
func TestPreflightCanBeTurnedOff(t *testing.T) {
	ts := newTestServer(t)
	c := ts.client(WithAPIURL(ts.URL+"/api"), WithoutPreflightChecks())

	_, err := c.Do(context.Background(), &Request{
		Using: []string{"urn:ietf:params:jmap:calendars"},
		MethodCalls: []Invocation{
			{Name: "A/get", CallID: "c0"}, {Name: "A/get", CallID: "c1"}, {Name: "A/get", CallID: "c2"},
		},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := ts.sessionHits.Load(); got != 0 {
		t.Errorf("the session was fetched %d times, want 0", got)
	}
}

func TestRequestErrorFromProblemDetails(t *testing.T) {
	ts := newTestServer(t)
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"urn:ietf:params:jmap:error:notRequest","status":400,"detail":"missing using"}`)
	}
	_, err := ts.client().Do(context.Background(), &Request{Using: []string{CapabilityCore}})
	if err == nil {
		t.Fatal("expected an error")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T, want *RequestError", err)
	}
	if reqErr.Type != ErrTypeNotRequest {
		t.Errorf("type = %q, want %q", reqErr.Type, ErrTypeNotRequest)
	}
	if reqErr.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", reqErr.Status)
	}
	if reqErr.Detail != "missing using" {
		t.Errorf("detail = %q", reqErr.Detail)
	}
}

// TestRequestErrorFromPlainBody checks that a server behind a proxy that
// answers with something other than problem details still yields a usable
// error.
func TestRequestErrorFromPlainBody(t *testing.T) {
	ts := newTestServer(t)
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream is down")
	}
	_, err := ts.client().Do(context.Background(), &Request{Using: []string{CapabilityCore}})
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error is %T (%v), want *RequestError", err, err)
	}
	if reqErr.Status != http.StatusBadGateway || reqErr.Detail != "upstream is down" {
		t.Errorf("error = %+v", reqErr)
	}
}

func TestAuthorizationHeaders(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{"bearer", WithBearerToken("tok"), "Bearer tok"},
		{"basic", WithBasicAuth("user", "pass"), "Basic dXNlcjpwYXNz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(t)
			var got string
			ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[]}`)
			}
			if _, err := ts.client(tt.opt).Do(context.Background(), &Request{Using: []string{CapabilityCore}}); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if got != tt.want {
				t.Errorf("Authorization = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDoSendsTheRequestBody checks the wire format of a request, which is what
// generated code depends on.
func TestDoSendsTheRequestBody(t *testing.T) {
	ts := newTestServer(t)
	var body map[string]any
	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		fmt.Fprint(w, `{"sessionState":"sess1","methodResponses":[]}`)
	}

	req := &Request{
		Using: []string{CapabilityCore, CapabilityMail},
		MethodCalls: []Invocation{{
			Name:   "Email/get",
			CallID: "c0",
			Args: map[string]any{
				"accountId": ID("a1"),
				"#ids":      ResultReference{ResultOf: "c-1", Name: "Email/query", Path: "/ids"},
			},
		}},
	}
	if _, err := ts.client().Do(context.Background(), req); err != nil {
		t.Fatalf("Do: %v", err)
	}

	calls := body["methodCalls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("got %d method calls, want 1", len(calls))
	}
	call := calls[0].([]any)
	if call[0] != "Email/get" || call[2] != "c0" {
		t.Errorf("call = %v, want Email/get as c0", call)
	}
	args := call[1].(map[string]any)
	ref := args["#ids"].(map[string]any)
	if ref["resultOf"] != "c-1" || ref["path"] != "/ids" {
		t.Errorf("#ids = %v", ref)
	}
}

func TestSessionWithoutAPIURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"capabilities":{},"accounts":{},"primaryAccounts":{},"username":"u","state":"s"}`)
	}))
	t.Cleanup(srv.Close)
	_, err := New(srv.URL).Session(context.Background())
	if err == nil {
		t.Fatal("expected an error for a session with no apiUrl")
	}
}

// TestCapabilityReading covers the capabilities that bring no types and no
// methods, only something to tell the client. VAPID is one: what RFC 9749 has
// to say is a key, carried in the session.
func TestCapabilityReading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
		  "capabilities": {
		    "urn:ietf:params:jmap:core": {"maxCallsInRequest": 16},
		    "urn:ietf:params:jmap:webpush-vapid": {"applicationServerKey": "BN1v_key"},
		    "urn:example:params:jmap:vendor": {"somethingElse": 7}
		  },
		  "accounts": {"a1": {"name": "someone", "isPersonal": true, "accountCapabilities": {
		    "urn:ietf:params:jmap:sieve": {"maxSizeScript": 65536, "maxNumberScripts": 20}
		  }}},
		  "primaryAccounts": {},
		  "username": "someone",
		  "apiUrl": "https://example.com/api",
		  "state": "s1"
		}`)
	}))
	t.Cleanup(srv.Close)

	s, err := New(srv.URL).Session(context.Background())
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	vapid, err := s.WebPushVAPID()
	if err != nil {
		t.Fatalf("WebPushVAPID: %v", err)
	}
	if vapid.ApplicationServerKey != "BN1v_key" {
		t.Errorf("applicationServerKey = %q", vapid.ApplicationServerKey)
	}

	// A capability jmapc has never heard of is read the same way.
	var vendor struct {
		SomethingElse int `json:"somethingElse"`
	}
	if err := s.Capability("urn:example:params:jmap:vendor", &vendor); err != nil {
		t.Fatalf("Capability: %v", err)
	}
	if vendor.SomethingElse != 7 {
		t.Errorf("somethingElse = %d, want 7", vendor.SomethingElse)
	}

	// Several capabilities state their limits per account rather than for the
	// server as a whole.
	var sieve struct {
		MaxSizeScript    int `json:"maxSizeScript"`
		MaxNumberScripts int `json:"maxNumberScripts"`
	}
	if err := s.Accounts["a1"].Capability(CapabilitySieve, &sieve); err != nil {
		t.Fatalf("account Capability: %v", err)
	}
	if sieve.MaxSizeScript != 65536 || sieve.MaxNumberScripts != 20 {
		t.Errorf("sieve limits = %+v", sieve)
	}

	// Asking for one the server does not advertise says so.
	if err := s.Capability(CapabilityMDN, &vendor); err == nil {
		t.Error("expected an error for a capability the server does not have")
	}
	if err := s.Accounts["a1"].Capability(CapabilityQuota, &vendor); err == nil {
		t.Error("expected an error for a capability the account does not support")
	}
}
