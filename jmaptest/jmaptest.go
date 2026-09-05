// Package jmaptest is a JMAP server for a test to run a generated client
// against.
//
// Testing a JMAP client means answering a request that carries several method
// calls, some of which refer to the results of the others. A stub written by
// hand for one test either ignores that — and stops resembling a server — or
// grows into this. So this is it: a server that checks what it is given
// against the JMAP data model, resolves the back references the way a server
// does, answers each call from whatever the test says, and remembers what it
// was asked.
//
//	srv := jmaptest.New(t)
//	srv.Reply("Email/query", jmapc.EmailQueryResponse{
//		AccountID: jmaptest.AccountID,
//		IDs:       []jmapc.ID{"m1"},
//	})
//	srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
//		// The ids are the ones the query call answered with: the back
//		// reference has already been resolved.
//		return myEmails(c.IDs()), nil
//	})
//
//	res, err := jmapq.ListInboxEmails(ctx, srv.Client(), params)
//
// What it does not do is store anything. It is a server to test a client
// against, not an implementation of JMAP: nothing a /set creates comes back
// from a later /get unless the test says it does.
package jmaptest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// AccountID is the account the server holds, and the primary account for every
// capability it advertises.
const AccountID = jmapc.ID("account")

// Handler answers one method call. Returning an error from Refuse answers with
// a JMAP method-level error; returning any other error fails the test, since
// it is the test that is broken rather than the client.
type Handler func(*Call) (any, error)

// Server is a JMAP server backed by an httptest.Server. It is closed when the
// test that made it ends.
type Server struct {
	t    testing.TB
	http *httptest.Server
	mux  *http.ServeMux

	mu       sync.Mutex
	handlers map[string]Handler
	calls    []*Call
	session  *jmapc.Session
	fail     *jmapc.RequestError
	checks   bool
	watchers map[chan string]bool
	sent     int
}

// Option settles something about the server before it answers anything.
type Option func(*Server)

// WithoutChecks stops the server from holding the requests it receives to the
// data model. Use it for a server that has to answer a method jmapc has never
// heard of, or a request a test means to be wrong.
func WithoutChecks() Option {
	return func(s *Server) { s.checks = false }
}

// WithSession adjusts the session the server serves, after the parts that
// depend on its address have been filled in. It is where to take a capability
// away, add one of your own, or hold a second account.
func WithSession(f func(*jmapc.Session)) Option {
	return func(s *Server) { f(s.session) }
}

// New returns a running server, closed when the test ends.
func New(t testing.TB, opts ...Option) *Server {
	t.Helper()
	s := &Server{
		t:        t,
		handlers: map[string]Handler{},
		checks:   true,
		watchers: map[chan string]bool{},
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/.well-known/jmap", s.serveSession)
	s.mux.HandleFunc("/api", s.serveAPI)
	s.mux.HandleFunc("/events", s.serveEvents)
	s.http = httptest.NewServer(s.mux)
	t.Cleanup(s.http.Close)

	s.session = &jmapc.Session{
		Capabilities:    capabilities(),
		Accounts:        map[jmapc.ID]*jmapc.Account{AccountID: {Name: "someone@example.com", IsPersonal: true}},
		PrimaryAccounts: primaryAccounts(),
		Username:        "someone@example.com",
		APIURL:          s.http.URL + "/api",
		EventSourceURL:  s.http.URL + "/events?types={types}&closeafter={closeafter}&ping={ping}",
		State:           "session-1",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// URL is the session resource to point a client at.
func (s *Server) URL() string { return s.http.URL + "/.well-known/jmap" }

// BaseURL is the address the server answers on, without the session path. It
// is what a client that is not the generated one is pointed at, since such a
// client has its own idea of where the session and the API live.
func (s *Server) BaseURL() string { return s.http.URL }

// Mux is where the server's paths are registered, so that a test can add its
// own beside them.
//
// A migration is the reason this is here. A client half converted to jmapc
// has a generated half that reaches this server through Client, and a
// hand-written half still posting to paths of its own. Both halves have to
// answer in one test, and this is where the paths the other half wants are
// mounted:
//
//	srv := jmaptest.New(t)
//	srv.Mux().HandleFunc("/jmap", myOldAPIHandler)
//	srv.Mux().HandleFunc("/jmap/session", myOldSessionHandler)
//	old := myOldClient(srv.BaseURL())
//
// The paths jmaptest serves are taken already, and registering one of those
// again panics, as registering any pattern twice on a ServeMux does.
func (s *Server) Mux() *http.ServeMux { return s.mux }

// Client returns a client pointed at the server, carrying a token it does not
// check.
func (s *Server) Client(opts ...jmapc.Option) *jmapc.Client {
	return jmapc.New(s.URL(), append([]jmapc.Option{jmapc.WithBearerToken("token")}, opts...)...)
}

// Handle says how the server answers a method. It replaces whatever was
// registered for that method before.
func (s *Server) Handle(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Reply says what a method answers with, where the answer does not depend on
// the call. The value is marshalled as JSON, except a json.RawMessage or a
// []byte, which is taken as the JSON it already is.
func (s *Server) Reply(method string, response any) {
	s.Handle(method, func(*Call) (any, error) { return response, nil })
}

// Fail makes a method answer with a JMAP method-level error, as a server does
// for a call it will not run. The types are the ones RFC 8620, Section 3.6.2
// names, such as "invalidArguments" or "accountNotFound".
func (s *Server) Fail(method, errType string) {
	s.Handle(method, func(*Call) (any, error) { return nil, Refuse(errType) })
}

// FailRequest makes the whole request fail, as a server does when it will not
// look at the calls at all. Pass nil to stop.
func (s *Server) FailRequest(err *jmapc.RequestError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// Calls returns every method call the server has been sent, in order.
func (s *Server) Calls() []*Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*Call(nil), s.calls...)
}

// Call returns the last call to a method, and fails the test where there has
// been none.
func (s *Server) Call(method string) *Call {
	s.t.Helper()
	calls := s.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method {
			return calls[i]
		}
	}
	s.t.Fatalf("jmaptest: nothing called %s; the calls were %s", method, strings.Join(methodNames(calls), ", "))
	return nil
}

// Requests returns how many requests the server has answered, which is what a
// test asking whether several calls travelled together is really asking.
func (s *Server) Requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
}

// Push sends a state change to every client watching the push endpoint, as a
// server does when something in an account has moved on. It is what a watch
// waits for.
func (s *Server) Push(accountID jmapc.ID, states map[string]string) {
	change := jmapc.StateChange{
		Type:    "StateChange",
		Changed: map[jmapc.ID]map[string]string{accountID: states},
	}
	body, err := json.Marshal(change)
	if err != nil {
		s.t.Errorf("jmaptest: encoding the event: %v", err)
		return
	}
	s.mu.Lock()
	watchers := make([]chan string, 0, len(s.watchers))
	for w := range s.watchers {
		watchers = append(watchers, w)
	}
	s.mu.Unlock()
	if len(watchers) == 0 {
		s.t.Error("jmaptest: nothing is watching the push endpoint")
		return
	}
	for _, w := range watchers {
		w <- fmt.Sprintf("id: e%d\nevent: state\ndata: %s\n\n", s.Requests()+1, body)
	}
}

// serveSession answers the session resource.
func (s *Server) serveSession(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	session := *s.session
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(session); err != nil {
		s.t.Errorf("jmaptest: writing the session: %v", err)
	}
}

// serveEvents holds a connection open and writes what Push sends it.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	events := make(chan string, 8)
	s.mu.Lock()
	s.watchers[events] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.watchers, events)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flush, _ := w.(http.Flusher)
	if flush != nil {
		flush.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if _, err := fmt.Fprint(w, event); err != nil {
				return
			}
			if flush != nil {
				flush.Flush()
			}
		}
	}
}

// request is the JMAP Request object as it arrives, kept undecoded so that
// each call is checked as it was written.
type request struct {
	Using       []string              `json:"using"`
	MethodCalls []json.RawMessage     `json:"methodCalls"`
	CreatedIDs  map[jmapc.ID]jmapc.ID `json:"createdIds"`
}

// serveAPI answers one request.
func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fail := s.fail
	s.sent++
	s.mu.Unlock()
	if fail != nil {
		s.writeRequestError(w, fail)
		return
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.t.Errorf("jmaptest: the request is not JSON: %v", err)
		s.writeRequestError(w, &jmapc.RequestError{Status: http.StatusBadRequest, Type: jmapc.ErrTypeNotJSON})
		return
	}
	if err := s.checkUsing(req.Using); err != nil {
		s.writeRequestError(w, err)
		return
	}

	check := query.NewRequestCheck(spec.Standard(), req.Using)
	responses := make([]jmapc.Invocation, 0, len(req.MethodCalls))
	answered := map[string]json.RawMessage{}
	names := map[string]string{}

	for i, raw := range req.MethodCalls {
		call, err := decodeCall(raw)
		if err != nil {
			s.t.Errorf("jmaptest: %v", err)
			continue
		}
		if s.checks {
			if err := check.Call(raw, i); err != nil {
				s.t.Errorf("jmaptest: the client sent something the data model does not allow:\n%v", err)
				responses = append(responses, methodError(call.ID, "invalidArguments", err.Error()))
				continue
			}
		}
		if err := call.resolve(answered, names); err != nil {
			responses = append(responses, methodError(call.ID, "invalidResultReference", err.Error()))
			continue
		}

		s.mu.Lock()
		handler, known := s.handlers[call.Method]
		s.calls = append(s.calls, call)
		s.mu.Unlock()
		if !known {
			s.t.Errorf("jmaptest: nothing answers %s; add srv.Reply(%q, ...) or srv.Handle(%q, ...)",
				call.Method, call.Method, call.Method)
			responses = append(responses, methodError(call.ID, "serverFail", "the test registered no answer"))
			continue
		}

		value, err := handler(call)
		if err != nil {
			var refused *refusal
			if !as(err, &refused) {
				s.t.Errorf("jmaptest: the handler for %s failed: %v", call.Method, err)
				refused = &refusal{Type: "serverFail", Description: err.Error()}
			}
			responses = append(responses, methodError(call.ID, refused.Type, refused.Description))
			continue
		}
		body, err := marshal(value)
		if err != nil {
			s.t.Errorf("jmaptest: encoding the answer to %s: %v", call.Method, err)
			responses = append(responses, methodError(call.ID, "serverFail", err.Error()))
			continue
		}
		answered[call.ID] = body
		names[call.ID] = call.Method
		responses = append(responses, jmapc.Invocation{Name: call.Method, CallID: call.ID, Args: body})
	}

	w.Header().Set("Content-Type", "application/json")
	out := struct {
		MethodResponses []jmapc.Invocation    `json:"methodResponses"`
		CreatedIDs      map[jmapc.ID]jmapc.ID `json:"createdIds,omitempty"`
		SessionState    string                `json:"sessionState"`
	}{responses, req.CreatedIDs, s.session.State}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		s.t.Errorf("jmaptest: writing the response: %v", err)
	}
}

// checkUsing reports a capability the request declares and the session does
// not advertise, which a server answers with a request-level error rather than
// by running the calls.
func (s *Server) checkUsing(using []string) *jmapc.RequestError {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, uri := range using {
		if _, ok := s.session.Capabilities[uri]; !ok {
			return &jmapc.RequestError{
				Status: http.StatusBadRequest,
				Type:   jmapc.ErrTypeUnknownCapability,
				Detail: "the server does not support " + uri,
			}
		}
	}
	return nil
}

// writeRequestError answers with a problem document, as a server does when it
// will not look at the calls.
func (s *Server) writeRequestError(w http.ResponseWriter, err *jmapc.RequestError) {
	status := err.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if writeErr := json.NewEncoder(w).Encode(err); writeErr != nil {
		s.t.Errorf("jmaptest: writing the error: %v", writeErr)
	}
}

// methodError renders a method-level error as the invocation it travels in.
func methodError(callID, errType, description string) jmapc.Invocation {
	args := map[string]any{"type": errType}
	if description != "" {
		args["description"] = description
	}
	return jmapc.Invocation{Name: "error", CallID: callID, Args: args}
}

// marshal renders whatever a handler answered with. JSON that is already JSON
// is left as it is, so that a fixture read from a file arrives untouched.
func marshal(value any) (json.RawMessage, error) {
	switch v := value.(type) {
	case nil:
		return json.RawMessage("{}"), nil
	case json.RawMessage:
		return v, nil
	case []byte:
		return json.RawMessage(v), nil
	}
	return json.Marshal(value)
}

// refusal is a method-level error a handler asked for.
type refusal struct {
	Type        string
	Description string
}

func (r *refusal) Error() string {
	if r.Description == "" {
		return "jmaptest: " + r.Type
	}
	return "jmaptest: " + r.Type + ": " + r.Description
}

// Refuse returns an error that answers the call with a JMAP method-level
// error, as a server does for a call it will not run.
func Refuse(errType string, description ...string) error {
	r := &refusal{Type: errType}
	if len(description) > 0 {
		r.Description = strings.Join(description, " ")
	}
	return r
}

// as is errors.As for the one type this package unwraps to, kept here so that
// the reader does not have to look up what is being matched.
func as(err error, target **refusal) bool {
	for err != nil {
		if r, ok := err.(*refusal); ok {
			*target = r
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// methodNames lists what was called, for a message about what was not.
func methodNames(calls []*Call) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Method
	}
	if len(out) == 0 {
		return []string{"none"}
	}
	return out
}

// capabilities is what the session advertises: everything the catalogue knows
// about, since a server that supported less would only get in the way of a
// test about something else.
func capabilities() map[string]json.RawMessage {
	out := map[string]json.RawMessage{
		spec.CapabilityCore: json.RawMessage(`{
			"maxSizeUpload": 50000000, "maxConcurrentUpload": 4,
			"maxSizeRequest": 10000000, "maxConcurrentRequests": 4,
			"maxCallsInRequest": 16, "maxObjectsInGet": 500, "maxObjectsInSet": 500,
			"collationAlgorithms": ["i;ascii-numeric", "i;ascii-casemap", "i;unicode-casemap"]
		}`),
	}
	for _, uri := range capabilityURIs() {
		if _, done := out[uri]; !done {
			out[uri] = json.RawMessage(`{}`)
		}
	}
	return out
}

// primaryAccounts points every capability at the one account the server holds.
func primaryAccounts() map[string]jmapc.ID {
	out := map[string]jmapc.ID{}
	for _, uri := range capabilityURIs() {
		out[uri] = AccountID
	}
	out[spec.CapabilityCore] = AccountID
	return out
}

// capabilityURIs is every capability the catalogue's methods belong to.
func capabilityURIs() []string {
	seen := map[string]bool{}
	for _, m := range spec.Standard().Methods() {
		if m.Capability != "" {
			seen[m.Capability] = true
		}
	}
	out := make([]string, 0, len(seen))
	for uri := range seen {
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}
