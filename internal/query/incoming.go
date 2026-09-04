package query

import (
	"encoding/json"
	"fmt"

	"github.com/linyows/jmapc/internal/spec"
)

// RequestCheck holds a request to the same standard a query file is held to,
// as its calls arrive.
//
// A query file is checked before anything is sent. A request that arrives at a
// server has already been sent, and a stub server that took whatever it was
// given would let a client send nonsense and call the test passed. The calls
// are checked in the order the server runs them, so that a back reference is
// held to the calls before it, exactly as one in a query file is.
type RequestCheck struct {
	c     *checker
	using map[string]bool
}

// NewRequestCheck returns a check over one request, against the given
// catalogue and the capabilities the request declared.
func NewRequestCheck(s *spec.Spec, using []string) *RequestCheck {
	declared := make(map[string]bool, len(using))
	for _, uri := range using {
		if full, aliased := capabilityAliases[uri]; aliased {
			uri = full
		}
		declared[uri] = true
	}
	return &RequestCheck{
		c: &checker{
			spec:   s,
			file:   "the request",
			params: newParamSet(),
			byID:   make(map[string]*Call),
			used:   make(map[string]bool),
		},
		using: declared,
	}
}

// Call checks one method call, given as the three-element array it arrives as,
// and reports what the data model says is wrong with it. The index is where the
// call sits in the request, which the diagnostics point at.
//
// A call that checks out is remembered, so that a back reference in a later
// call can be resolved against it.
func (r *RequestCheck) Call(raw json.RawMessage, index int) error {
	before := len(r.c.errs)
	call := r.c.methodCall(raw, fmt.Sprintf("methodCalls[%d]", index))
	if call != nil {
		r.capability(call, index)
	}
	return ErrorList(r.c.errs[before:]).Err()
}

// capability checks that the request declared what the call needs. RFC 8620,
// Section 3.2 has a server refuse a method whose capability the request did not
// declare, however well the server supports it.
func (r *RequestCheck) capability(call *Call, index int) {
	capability := call.Method.Capability
	if capability == "" || capability == spec.CapabilityCore || r.using[capability] {
		return
	}
	r.c.errorf(fmt.Sprintf("methodCalls[%d][0]", index),
		"a request has to declare the capabilities of the methods it calls",
		"%s needs %s, which the request does not declare", call.Method.Name, capability)
}
