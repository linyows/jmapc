package jmapc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
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
	// RetryAfter is how long the server asked the client to wait before
	// sending the request again, and zero where it asked for no particular
	// delay. It comes from the Retry-After header rather than from the
	// problem details document, which is why it carries no JSON name.
	RetryAfter time.Duration `json:"-"`
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

// SetFailure is one record a /set would not act on, and what the server said
// about it.
type SetFailure struct {
	// Method is the method call that refused the record, such as "Email/set".
	Method string
	// CallID is the id of that call within the request.
	CallID string
	// Kind is the response property the failure was reported in, such as
	// "notCreated".
	Kind string
	// Key is the creation id or record id the failure is filed under.
	Key ID
	// Err is the error the server reported.
	Err SetError
}

func (f SetFailure) Error() string {
	return fmt.Sprintf("%s could not %s %q: %s", f.Method, setVerb(f.Kind), f.Key, f.Err.Error())
}

// Unwrap exposes the SetError this failure carries, so that
// errors.As(err, &setError) with a *SetError target can reach it without the
// caller knowing SetFailure exists.
func (f SetFailure) Unwrap() error { return &f.Err }

// setVerb turns the name of a response property into the verb it refuses, so
// that the message reads as a sentence rather than as a field name.
func setVerb(kind string) string {
	switch kind {
	case "notCreated":
		return "create"
	case "notUpdated":
		return "update"
	case "notDestroyed":
		return "destroy"
	case "notCopied":
		return "copy"
	}
	return strings.TrimPrefix(kind, "not")
}

// SetErrors reports the records a request could not act on. A /set answers 200
// and lists what it refused, so a caller that reads only the transport error
// sees success where there was none; generated code collects those refusals
// and returns them here, alongside the part of the response that did succeed.
//
// Use errors.As to reach it, and Failures to see which records failed and why.
type SetErrors struct {
	Failures []SetFailure
}

// Collect records the failures a method call reported, keyed by the response
// property they arrived in. It is called by generated code.
func (e *SetErrors) Collect(method, callID string, groups map[string]map[ID]SetError) {
	kinds := make([]string, 0, len(groups))
	for kind := range groups {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		keys := make([]ID, 0, len(groups[kind]))
		for key := range groups[kind] {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, key := range keys {
			e.Failures = append(e.Failures, SetFailure{
				Method: method, CallID: callID, Kind: kind, Key: key, Err: groups[kind][key],
			})
		}
	}
}

// Err returns e where anything failed and nil where nothing did, so that
// generated code can collect first and decide afterwards.
func (e *SetErrors) Err() error {
	if len(e.Failures) == 0 {
		return nil
	}
	return e
}

func (e *SetErrors) Error() string {
	switch len(e.Failures) {
	case 0:
		return "no records failed"
	case 1:
		return e.Failures[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", e.Failures[0].Error(), len(e.Failures)-1)
}

// Unwrap reports the failures as errors, so that errors.Is and errors.As reach
// each of them.
func (e *SetErrors) Unwrap() []error {
	errs := make([]error, len(e.Failures))
	for i, f := range e.Failures {
		errs[i] = f
	}
	return errs
}

// temporaryTypes are the error types a server reports for its own trouble
// rather than for the request: it is unavailable, it failed, it failed part way
// through, or it is being asked too often. Every other type JMAP defines
// reports something about the request, and the answer to sending that request
// again is the same, so a type that is not here is taken to be one that time
// does not resolve. That applies to the types a vendor defines too, which are
// almost always refusals of the request.
var temporaryTypes = map[string]bool{
	ErrServerUnavailable: true,
	ErrServerFail:        true,
	ErrServerPartialFail: true,
	ErrRateLimit:         true,
}

// concurrencyLimits are the limits a request exceeds by being one request too
// many at that moment, rather than by being too large. The server answers 400
// for them, not 429, so a caller looking only at the status does not see that
// it was asked to send fewer.
var concurrencyLimits = map[string]bool{
	"maxConcurrentRequests": true,
	"maxConcurrentUpload":   true,
}

// IsTemporary reports whether err is a failure that time may resolve: the
// server was unavailable, it failed, it asked for fewer requests, or the
// request never reached it. It is false for a failure the server reported about
// the request itself, which is the same however long the caller waits.
//
// It answers what to do with a failure, not whether the request is safe to send
// again. A request that failed in transit may have been carried out, and a /set
// sent twice creates twice; RetryPolicy is where that judgement belongs.
//
// A failure jmapc cannot classify is temporary, since nothing about it says the
// next attempt will fail as well. A nil error is not a failure and is false.
func IsTemporary(err error) bool {
	if err == nil {
		return false
	}
	classified, temporary := false, true

	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		classified = true
		if !(reqErr.Status >= 500 || reqErr.Status == http.StatusTooManyRequests) {
			temporary = false
		}
	}
	for _, kind := range errorTypes(err) {
		classified = true
		if !temporaryTypes[kind] {
			temporary = false
		}
	}
	if !classified {
		return true
	}
	return temporary
}

// IsRateLimited reports whether err is the server asking for fewer requests: a
// 429, a rateLimit reported for a method call or for one record of a /set, or a
// request refused for exceeding one of the concurrency limits the session
// states.
//
// It is what tells a client that is being asked to slow down from one that is
// being told it is wrong, both of which arrive as a failed request.
func IsRateLimited(err error) bool {
	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		if reqErr.Status == http.StatusTooManyRequests {
			return true
		}
		if reqErr.Type == ErrTypeLimit && concurrencyLimits[reqErr.Limit] {
			return true
		}
	}
	for _, kind := range errorTypes(err) {
		if kind == ErrRateLimit {
			return true
		}
	}
	return false
}

// RetryAfter returns how long the server asked the client to wait before
// sending the request again, and whether it asked at all. It is the Retry-After
// header, which a server sends with a 429 or a 503, read as the seconds or the
// date RFC 9110 writes it as.
//
// The client waits out a short delay itself where the retry policy allows
// another attempt. This is how the caller learns of a long one: a server asking
// for an hour is not waited out, because holding a request in memory for that
// long is of no use to the caller, and the request is failed with the delay it
// asked for attached.
func RetryAfter(err error) (time.Duration, bool) {
	var reqErr *RequestError
	if errors.As(err, &reqErr) && reqErr.RetryAfter > 0 {
		return reqErr.RetryAfter, true
	}
	return 0, false
}

// errorTypes returns the error type of every method-level and record-level
// failure err carries. A request whose calls failed for different reasons is
// classified by all of them, since the server answers a caller that sends it
// again with every one of those reasons.
func errorTypes(err error) []string {
	var types []string
	var methods MethodErrors
	if errors.As(err, &methods) {
		for _, m := range methods {
			types = append(types, m.Type)
		}
	} else {
		var method *MethodError
		if errors.As(err, &method) {
			types = append(types, method.Type)
		}
	}
	var sets *SetErrors
	if errors.As(err, &sets) {
		for _, f := range sets.Failures {
			types = append(types, f.Err.Type)
		}
	} else {
		var set *SetError
		if errors.As(err, &set) {
			types = append(types, set.Type)
		}
	}
	return types
}
