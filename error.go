package jmapc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RequestError is a request-level error as defined in RFC 8620, Section 3.6.1.
// The server reports these as an RFC 7807 problem details document with a
// non-2xx status, and none of the method calls in the request were executed.
type RequestError struct {
	// Status is the HTTP status code the server responded with.
	Status int `json:"status"`
	// Type is the problem type URI, such as
	// "urn:ietf:params:jmap:error:unknownCapability".
	Type string `json:"type"`
	// Title is a short human-readable summary, if the server supplied one.
	Title string `json:"title,omitempty"`
	// Detail is a human-readable explanation specific to this occurrence.
	Detail string `json:"detail,omitempty"`
	// Limit names the exceeded limit when Type is
	// "urn:ietf:params:jmap:error:limit".
	Limit string `json:"limit,omitempty"`
}

func (e *RequestError) Error() string {
	var b strings.Builder
	b.WriteString("jmapc: request failed")
	if e.Status != 0 {
		fmt.Fprintf(&b, " with status %d %s", e.Status, http.StatusText(e.Status))
	}
	if e.Type != "" {
		fmt.Fprintf(&b, ": %s", e.Type)
	}
	if e.Limit != "" {
		fmt.Fprintf(&b, " (limit %s)", e.Limit)
	}
	if e.Detail != "" {
		fmt.Fprintf(&b, ": %s", e.Detail)
	} else if e.Title != "" {
		fmt.Fprintf(&b, ": %s", e.Title)
	}
	return b.String()
}

// Request-level error types defined by RFC 8620, Section 3.6.1.
const (
	ErrTypeUnknownCapability = "urn:ietf:params:jmap:error:unknownCapability"
	ErrTypeNotJSON           = "urn:ietf:params:jmap:error:notJSON"
	ErrTypeNotRequest        = "urn:ietf:params:jmap:error:notRequest"
	ErrTypeLimit             = "urn:ietf:params:jmap:error:limit"
)

// MethodError is a method-level error as defined in RFC 8620, Section 3.6.2.
// The server returns it in place of the response to a single method call; the
// other calls in the same request may still have succeeded.
type MethodError struct {
	// CallID is the client-assigned identifier of the failed method call.
	CallID string `json:"-"`
	// MethodName is the name of the method that was invoked.
	MethodName string `json:"-"`
	// Type is the error type, such as "invalidArguments".
	Type string `json:"type"`
	// Description is an optional human-readable explanation.
	Description string `json:"description,omitempty"`
	// Arguments names the invalid arguments when Type is "invalidArguments".
	Arguments []string `json:"arguments,omitempty"`
	// Raw holds every member of the error object, so that error types carrying
	// members beyond those above remain inspectable.
	Raw map[string]json.RawMessage `json:"-"`
}

func (e *MethodError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "jmapc: method %s (call %q) failed: %s", e.MethodName, e.CallID, e.Type)
	if len(e.Arguments) > 0 {
		fmt.Fprintf(&b, " %v", e.Arguments)
	}
	if e.Description != "" {
		fmt.Fprintf(&b, ": %s", e.Description)
	}
	return b.String()
}

// Method-level error types defined by RFC 8620, Section 3.6.2.
const (
	ErrServerUnavailable = "serverUnavailable"
	ErrServerFail        = "serverFail"
	ErrServerPartialFail = "serverPartialFail"
	ErrUnknownMethod     = "unknownMethod"
	ErrInvalidArguments  = "invalidArguments"
	ErrInvalidResultRef  = "invalidResultReference"
	ErrForbidden         = "forbidden"
	ErrAccountNotFound   = "accountNotFound"
	ErrAccountNotSupport = "accountNotSupportedByMethod"
	ErrAccountReadOnly   = "accountReadOnly"
	ErrRequestTooLarge   = "requestTooLarge"
	ErrCannotCalcChanges = "cannotCalculateChanges"
	ErrStateMismatch     = "stateMismatch"
	ErrUnsupportedFilter = "unsupportedFilter"
	ErrUnsupportedSort   = "unsupportedSort"
	ErrOverQuota         = "overQuota"
	ErrTooLarge          = "tooLarge"
	ErrRateLimit         = "rateLimit"
	ErrNotFound          = "notFound"
	ErrInvalidPatch      = "invalidPatch"
	ErrWillDestroy       = "willDestroy"
	ErrInvalidProperties = "invalidProperties"
	ErrSingleton         = "singleton"
)

// MethodErrors collects every method-level error in one response. A request
// whose calls partly succeeded returns both the decoded results and a
// MethodErrors describing the calls that did not.
type MethodErrors []*MethodError

func (e MethodErrors) Error() string {
	switch len(e) {
	case 0:
		return "jmapc: no method errors"
	case 1:
		return e[0].Error()
	}
	parts := make([]string, len(e))
	for i, me := range e {
		parts[i] = fmt.Sprintf("%s (call %q): %s", me.MethodName, me.CallID, me.Type)
	}
	return "jmapc: " + fmt.Sprint(len(e)) + " method calls failed: " + strings.Join(parts, "; ")
}

// Unwrap lets errors.As reach the individual method errors.
func (e MethodErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i, me := range e {
		errs[i] = me
	}
	return errs
}

// SetError is the error object reported per-record in the notCreated,
// notUpdated, and notDestroyed maps of a /set response. It is not returned as a
// Go error, because the surrounding method call itself succeeded.
type SetError struct {
	// Type is the error type, such as "invalidProperties" or "notFound".
	Type string `json:"type"`
	// Description is an optional human-readable explanation.
	Description string `json:"description,omitempty"`
	// Properties names the offending properties when Type is
	// "invalidProperties".
	Properties []string `json:"properties,omitempty"`
	// ExistingID is set when Type is "alreadyExists".
	ExistingID ID `json:"existingId,omitempty"`
}

func (e *SetError) Error() string {
	var b strings.Builder
	b.WriteString(e.Type)
	if len(e.Properties) > 0 {
		fmt.Fprintf(&b, " %v", e.Properties)
	}
	if e.Description != "" {
		fmt.Fprintf(&b, ": %s", e.Description)
	}
	return b.String()
}
