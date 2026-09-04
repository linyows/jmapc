package jmapc

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestInvocationMarshal(t *testing.T) {
	in := Invocation{
		Name:   "Email/get",
		CallID: "c0",
		Args:   map[string]any{"accountId": ID("a1")},
	}
	got, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	want := `["Email/get",{"accountId":"a1"},"c0"]`
	if string(got) != want {
		t.Errorf("marshalled to %s, want %s", got, want)
	}
}

// TestInvocationMarshalNoArgs checks that a call with no arguments still sends
// an object, because the wire format has no room for a missing one.
func TestInvocationMarshalNoArgs(t *testing.T) {
	got, err := json.Marshal(Invocation{Name: "Core/echo", CallID: "c0"})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if want := `["Core/echo",{},"c0"]`; string(got) != want {
		t.Errorf("marshalled to %s, want %s", got, want)
	}
}

func TestInvocationUnmarshalErrors(t *testing.T) {
	for _, in := range []string{`{}`, `[]`, `["a",{}]`, `["a",{},"c","extra"]`, `[1,{},"c"]`} {
		var out Invocation
		if err := json.Unmarshal([]byte(in), &out); err == nil {
			t.Errorf("unmarshalling %s succeeded, want an error", in)
		}
	}
}

// response builds a Response from a JSON body, as the client would.
func response(t *testing.T, req *Request, body string) *Response {
	t.Helper()
	var r Response
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshalling response: %v", err)
	}
	r.req = req
	return &r
}

func TestResponseDecode(t *testing.T) {
	r := response(t, nil, `{"sessionState":"s","methodResponses":[
	  ["Email/query",{"accountId":"a1","ids":["e1"]},"c0"]
	]}`)

	var out EmailQueryResponse
	if err := r.Decode("c0", &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out.IDs) != 1 || out.IDs[0] != "e1" {
		t.Errorf("ids = %v, want [e1]", out.IDs)
	}
	err := r.Decode("c9", &out)
	if err == nil {
		t.Fatal("decoding an absent call succeeded, want an error")
	}
	if want := `jmapc: response has no result for call "c9" (response has "c0")`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestResponseDecodeNamesNoResults checks the diagnostic for a response with
// no method responses at all, which is the empty-list edge of callIDs.
func TestResponseDecodeNamesNoResults(t *testing.T) {
	r := response(t, nil, `{"sessionState":"s","methodResponses":[]}`)
	var out EmailQueryResponse
	err := r.Decode("c0", &out)
	if want := `jmapc: response has no result for call "c0" (response has no results)`; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

// TestResponseDecodeMethodError checks that decoding an error response reports
// the method the client asked for, not the "error" the server sends in its
// place.
func TestResponseDecodeMethodError(t *testing.T) {
	req := &Request{MethodCalls: []Invocation{{Name: "Email/query", CallID: "c0"}}}
	r := response(t, req, `{"sessionState":"s","methodResponses":[
	  ["error",{"type":"invalidArguments","arguments":["filter"],"description":"bad filter"},"c0"]
	]}`)

	var out EmailQueryResponse
	err := r.Decode("c0", &out)
	if err == nil {
		t.Fatal("expected an error")
	}
	var me *MethodError
	if !errors.As(err, &me) {
		t.Fatalf("error is %T, want *MethodError", err)
	}
	if me.Type != ErrInvalidArguments {
		t.Errorf("type = %q, want %q", me.Type, ErrInvalidArguments)
	}
	if me.MethodName != "Email/query" {
		t.Errorf("method = %q, want Email/query", me.MethodName)
	}
	if len(me.Arguments) != 1 || me.Arguments[0] != "filter" {
		t.Errorf("arguments = %v, want [filter]", me.Arguments)
	}
	if _, kept := me.Raw["description"]; !kept {
		t.Error("Raw does not hold the error object's members")
	}
}

// TestResponseErrorsPartial checks that the successful calls of a partly failed
// request are still readable, because JMAP runs the calls it can.
func TestResponseErrorsPartial(t *testing.T) {
	req := &Request{MethodCalls: []Invocation{
		{Name: "Email/query", CallID: "c0"},
		{Name: "Email/get", CallID: "c1"},
	}}
	r := response(t, req, `{"sessionState":"s","methodResponses":[
	  ["error",{"type":"unsupportedSort"},"c0"],
	  ["Email/get",{"accountId":"a1","state":"s1","list":[],"notFound":[]},"c1"]
	]}`)

	errs := r.Errors()
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if errs[0].MethodName != "Email/query" {
		t.Errorf("failed method = %q, want Email/query", errs[0].MethodName)
	}
	var out EmailGetResponse
	if err := r.Decode("c1", &out); err != nil {
		t.Errorf("decoding the call that succeeded: %v", err)
	}
	if out.State != "s1" {
		t.Errorf("state = %q, want s1", out.State)
	}
}

func TestMethodErrorsUnwrap(t *testing.T) {
	errs := MethodErrors{
		{CallID: "c0", MethodName: "Email/query", Type: ErrUnsupportedSort},
		{CallID: "c1", MethodName: "Email/get", Type: ErrRequestTooLarge},
	}
	var me *MethodError
	if !errors.As(error(errs), &me) {
		t.Fatal("errors.As did not reach a *MethodError")
	}
	if me.CallID != "c0" {
		t.Errorf("reached call %q, want c0", me.CallID)
	}
}
