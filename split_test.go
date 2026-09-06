package jmapc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// splitServer answers Email/get with the ids it was asked for, and records
// what each request carried.
type splitServer struct {
	*testServer
	mu sync.Mutex
	// asked holds the ids of each Email/get call, in the order they arrived.
	asked [][]string
	// requests counts the requests the server answered.
	requests int
	// state is the state each answer reports, by the request it answers.
	state func(request int) string
	// fail names a call id the server answers with a method-level error.
	fail func(ids []string) bool
}

func newSplitServer(t *testing.T, maxObjectsInGet, maxCallsInRequest int) *splitServer {
	t.Helper()
	ts := &splitServer{testServer: newTestServer(t)}
	ts.state = func(int) string { return "s1" }
	ts.sessionHandler = fmt.Sprintf(`{
	  "capabilities": {
	    "urn:ietf:params:jmap:core": {"maxObjectsInGet": %d, "maxCallsInRequest": %d},
	    "urn:ietf:params:jmap:mail": {}
	  },
	  "accounts": {"a1": {"name": "someone", "isPersonal": true, "isReadOnly": false}},
	  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
	  "username": "someone",
	  "apiUrl": %q,
	  "state": "sess1"
	}`, maxObjectsInGet, maxCallsInRequest, ts.URL+"/api")

	ts.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MethodCalls []Invocation `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("the server could not read the request: %v", err)
			return
		}
		ts.mu.Lock()
		ts.requests++
		which := ts.requests
		ts.mu.Unlock()

		var answers []string
		for _, call := range req.MethodCalls {
			raw, _ := call.RawArgs()
			var args struct {
				IDs []string `json:"ids"`
			}
			_ = json.Unmarshal(raw, &args)
			ts.mu.Lock()
			ts.asked = append(ts.asked, args.IDs)
			ts.mu.Unlock()

			if ts.fail != nil && ts.fail(args.IDs) {
				answers = append(answers, fmt.Sprintf(`["error",{"type":"serverFail"},%q]`, call.CallID))
				continue
			}
			var list, notFound []string
			for _, id := range args.IDs {
				if strings.HasPrefix(id, "x") {
					notFound = append(notFound, fmt.Sprintf("%q", id))
					continue
				}
				list = append(list, fmt.Sprintf(`{"id":%q}`, id))
			}
			answers = append(answers, fmt.Sprintf(`["Email/get",{"accountId":"a1","state":%q,"list":[%s],"notFound":[%s]},%q]`,
				ts.state(which), strings.Join(list, ","), strings.Join(notFound, ","), call.CallID))
		}
		fmt.Fprintf(w, `{"sessionState":"sess1","methodResponses":[%s]}`, strings.Join(answers, ","))
	}
	return ts
}

func (ts *splitServer) sent() [][]string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.asked
}

// get builds an Email/get for the given ids.
func get(ids ...string) *Request {
	values := make([]ID, len(ids))
	for i, id := range ids {
		values[i] = ID(id)
	}
	return &Request{
		Using: []string{CapabilityCore, CapabilityMail},
		MethodCalls: []Invocation{
			{Name: "Email/get", CallID: "fetch", Args: map[string]any{"accountId": "a1", "ids": values}},
		},
	}
}

type emailGetResponse struct {
	State    string            `json:"state"`
	List     []struct{ ID ID } `json:"list"`
	NotFound []ID              `json:"notFound"`
}

func TestASplitGetIsSentInSeveralRequestsAndJoined(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	c := ts.client(WithSplitGets())

	resp, err := c.Do(context.Background(), get("m1", "m2", "m3", "m4", "m5"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	want := [][]string{{"m1", "m2"}, {"m3", "m4"}, {"m5"}}
	got := ts.sent()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the server was asked for %v, want %v", got, want)
	}

	var out emailGetResponse
	if err := resp.Decode("fetch", &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out.List) != 5 {
		t.Fatalf("the joined answer holds %d records, want 5", len(out.List))
	}
	for i, want := range []ID{"m1", "m2", "m3", "m4", "m5"} {
		if out.List[i].ID != want {
			t.Errorf("record %d is %q, want %q", i, out.List[i].ID, want)
		}
	}
	if out.State != "s1" {
		t.Errorf("state = %q, want s1", out.State)
	}
}

func TestNothingIsSplitWithoutTheOption(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	c := ts.client()
	if _, err := c.Do(context.Background(), get("m1", "m2", "m3")); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := ts.sent(); len(got) != 1 || len(got[0]) != 3 {
		t.Errorf("the server was asked for %v, want the three ids in one call", got)
	}
}

// The parts that did not fit travel together where the server takes several
// calls in one request, and separately where it does not.
func TestThePartsOfASplitAreGroupedByWhatTheServerTakes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		maxCalls int
		requests int
	}{
		{"several calls in one request", 16, 2},
		{"one call in a request", 1, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := newSplitServer(t, 2, tt.maxCalls)
			c := ts.client(WithSplitGets())
			if _, err := c.Do(context.Background(), get("m1", "m2", "m3", "m4", "m5")); err != nil {
				t.Fatalf("Do: %v", err)
			}
			ts.mu.Lock()
			defer ts.mu.Unlock()
			if ts.requests != tt.requests {
				t.Errorf("the server answered %d requests, want %d", ts.requests, tt.requests)
			}
		})
	}
}

// A call another call refers to cannot be split: a result reference resolves
// within one request, and there would be nothing left to resolve against.
func TestACallThatIsReferredToIsNotSplit(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	c := ts.client(WithSplitGets())

	r := get("m1", "m2", "m3")
	r.MethodCalls = append(r.MethodCalls, Invocation{
		Name:   "Email/get",
		CallID: "again",
		Args: map[string]any{
			"accountId": "a1",
			"#ids":      ResultReference{ResultOf: "fetch", Name: "Email/get", Path: "/list/*/id"},
		},
	})
	if _, err := c.Do(context.Background(), r); err != nil {
		t.Fatalf("Do: %v", err)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.requests != 1 {
		t.Errorf("the server answered %d requests, want 1", ts.requests)
	}
	if len(ts.asked) == 0 || len(ts.asked[0]) != 3 {
		t.Errorf("the referenced call was asked for %v, want its three ids in one call", ts.asked)
	}
}

// How many ids a back reference resolves to is known to the server alone.
func TestACallWhoseIDsComeFromAReferenceIsNotSplit(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	c := ts.client(WithSplitGets())
	r := &Request{
		Using: []string{CapabilityCore, CapabilityMail},
		MethodCalls: []Invocation{
			{Name: "Email/get", CallID: "fetch", Args: map[string]any{
				"accountId": "a1",
				"#ids":      ResultReference{ResultOf: "search", Name: "Email/query", Path: "/ids"},
			}},
		},
	}
	if _, err := c.Do(context.Background(), r); err != nil {
		t.Fatalf("Do: %v", err)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.requests != 1 {
		t.Errorf("the server answered %d requests, want 1", ts.requests)
	}
}

// Each request is answered separately, so the account may change between them,
// and a caller that needs one snapshot has to be told.
func TestAStateThatChangedBetweenThePartsIsReported(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	ts.state = func(request int) string { return fmt.Sprintf("s%d", request) }
	c := ts.client(WithSplitGets())

	resp, err := c.Do(context.Background(), get("m1", "m2", "m3"))
	if resp == nil {
		t.Fatalf("Do returned no response: %v", err)
	}
	var changed *StateChanged
	if !errors.As(err, &changed) {
		t.Fatalf("Do returned %v, want a *StateChanged", err)
	}
	if changed.From != "s1" || changed.To != "s2" {
		t.Errorf("the state changed from %q to %q, want s1 to s2", changed.From, changed.To)
	}
	if changed.CallID != "fetch" {
		t.Errorf("the call id is %q, want fetch", changed.CallID)
	}
	var out emailGetResponse
	if err := resp.Decode("fetch", &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out.List) != 3 {
		t.Errorf("the joined answer holds %d records, want the three that were fetched", len(out.List))
	}
}

func TestNotFoundIsJoinedTogetherWithTheRecords(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	c := ts.client(WithSplitGets())
	resp, err := c.Do(context.Background(), get("m1", "x2", "m3", "x4"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	var out emailGetResponse
	if err := resp.Decode("fetch", &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out.List) != 2 || len(out.NotFound) != 2 {
		t.Fatalf("the joined answer holds %d records and %d not found, want 2 and 2", len(out.List), len(out.NotFound))
	}
	if out.NotFound[0] != "x2" || out.NotFound[1] != "x4" {
		t.Errorf("notFound = %v, want x2 and x4", out.NotFound)
	}
}

// A part the server refused means the call did not return every record the
// caller asked for, so the call reports the refusal rather than part of an
// answer.
func TestAPartTheServerRefusedFailsTheCall(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	ts.fail = func(ids []string) bool { return len(ids) > 0 && ids[0] == "m3" }
	c := ts.client(WithSplitGets())

	_, err := c.Do(context.Background(), get("m1", "m2", "m3", "m4"))
	var errs MethodErrors
	if !errors.As(err, &errs) {
		t.Fatalf("Do returned %v, want a MethodErrors", err)
	}
	if len(errs) != 1 || errs[0].Type != "serverFail" {
		t.Fatalf("the errors are %v, want the one serverFail", errs)
	}
	if errs[0].CallID != "fetch" {
		t.Errorf("the error is reported under %q, want fetch", errs[0].CallID)
	}
}
