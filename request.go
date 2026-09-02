package jmapc

import (
	"encoding/json"
	"fmt"
)

// Invocation is a single method call or method response. On the wire it is the
// three-element array [name, arguments, callId] described in RFC 8620,
// Section 3.2.
type Invocation struct {
	// Name is the method name, such as "Email/get", or "error" for a
	// method-level error in a response.
	Name string
	// Args holds the arguments to marshal in a request, and a json.RawMessage
	// holding the unparsed arguments in a response.
	Args any
	// CallID is the client-assigned identifier that back references and
	// responses use to refer to this call.
	CallID string
}

func (in Invocation) MarshalJSON() ([]byte, error) {
	args := in.Args
	if args == nil {
		args = struct{}{}
	}
	return json.Marshal([3]any{in.Name, args, in.CallID})
}

func (in *Invocation) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("jmapc: invocation is not an array: %w", err)
	}
	if len(raw) != 3 {
		return fmt.Errorf("jmapc: invocation has %d elements, want 3", len(raw))
	}
	if err := json.Unmarshal(raw[0], &in.Name); err != nil {
		return fmt.Errorf("jmapc: invocation name: %w", err)
	}
	if err := json.Unmarshal(raw[2], &in.CallID); err != nil {
		return fmt.Errorf("jmapc: invocation call id: %w", err)
	}
	in.Args = raw[1]
	return nil
}

// RawArgs returns the undecoded arguments of an invocation that came from a
// response.
func (in Invocation) RawArgs() (json.RawMessage, bool) {
	raw, ok := in.Args.(json.RawMessage)
	return raw, ok
}

// ResultReference refers to a value in the response to an earlier method call
// in the same request, as defined in RFC 8620, Section 3.7. It is what makes a
// single JMAP request able to stand in for a chain of dependent calls.
type ResultReference struct {
	// ResultOf is the call id of the earlier method call.
	ResultOf string `json:"resultOf"`
	// Name is the method name of that call, which the server checks against
	// the call it finds.
	Name string `json:"name"`
	// Path is a JMAP JSON pointer into that call's response.
	Path string `json:"path"`
}

// Request is the JMAP Request object of RFC 8620, Section 3.3.
type Request struct {
	// Using lists the capability URIs the request depends on.
	Using []string `json:"using"`
	// MethodCalls are executed by the server in order.
	MethodCalls []Invocation `json:"methodCalls"`
	// CreatedIDs maps creation ids to record ids carried over from an earlier
	// request, and is optional.
	CreatedIDs map[ID]ID `json:"createdIds,omitempty"`
}

// Response is the JMAP Response object of RFC 8620, Section 3.4.
type Response struct {
	// MethodResponses holds one entry per executed method call, in the order
	// the server executed them.
	MethodResponses []Invocation `json:"methodResponses"`
	// CreatedIDs maps creation ids to the ids the server assigned.
	CreatedIDs map[ID]ID `json:"createdIds,omitempty"`
	// SessionState is the state string of the Session object at the time the
	// request was handled. A change means the session should be re-fetched.
	SessionState string `json:"sessionState"`

	// req is the request this response answers, when it is known. It lets an
	// error response report the method that failed, which the wire format
	// replaces with the literal name "error".
	req *Request
}

// requestedMethod returns the name of the method call the client sent under the
// given call id, falling back to fallback when the request is not known.
func (r *Response) requestedMethod(callID, fallback string) string {
	if r.req == nil {
		return fallback
	}
	for _, in := range r.req.MethodCalls {
		if in.CallID == callID {
			return in.Name
		}
	}
	return fallback
}

// find returns the response to the method call with the given call id.
func (r *Response) find(callID string) (*Invocation, bool) {
	for i := range r.MethodResponses {
		if r.MethodResponses[i].CallID == callID {
			return &r.MethodResponses[i], true
		}
	}
	return nil, false
}

// Decode unmarshals the response to the method call with the given call id into
// dest. It returns a *MethodError if the server reported an error for that
// call. Generated code uses this to turn one entry of methodResponses into a
// typed value.
func (r *Response) Decode(callID string, dest any) error {
	in, ok := r.find(callID)
	if !ok {
		return fmt.Errorf("jmapc: response has no result for call %q", callID)
	}
	raw, ok := in.RawArgs()
	if !ok {
		return fmt.Errorf("jmapc: result for call %q was not decoded from JSON", callID)
	}
	if in.Name == "error" {
		return methodErrorFrom(callID, r.requestedMethod(callID, in.Name), raw)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("jmapc: decoding result of call %q (%s): %w", callID, in.Name, err)
	}
	return nil
}

// Errors returns every method-level error in the response, or nil if there are
// none. A response may hold both errors and successful results, because the
// server executes the calls it can.
func (r *Response) Errors() MethodErrors {
	var errs MethodErrors
	for i := range r.MethodResponses {
		in := &r.MethodResponses[i]
		if in.Name != "error" {
			continue
		}
		raw, ok := in.RawArgs()
		if !ok {
			continue
		}
		var me *MethodError
		if err := methodErrorFrom(in.CallID, r.requestedMethod(in.CallID, in.Name), raw); err != nil {
			me, _ = err.(*MethodError)
		}
		if me != nil {
			errs = append(errs, me)
		}
	}
	return errs
}

// methodErrorFrom builds a *MethodError from the arguments of an error
// response. It never returns nil, so callers can return it as an error
// unconditionally.
func methodErrorFrom(callID, methodName string, raw json.RawMessage) error {
	me := &MethodError{CallID: callID, MethodName: methodName}
	if err := json.Unmarshal(raw, me); err != nil {
		me.Type = ErrServerFail
		me.Description = fmt.Sprintf("malformed error object: %v", err)
		return me
	}
	_ = json.Unmarshal(raw, &me.Raw)
	if me.Type == "" {
		me.Type = ErrServerFail
	}
	return me
}
