package jmapc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// An Observer receives a report of what the client does. It is the attachment
// point for logging, metrics and tracing, and it affects neither the request
// sent nor the response received. A nil field is not called.
//
// Each hook returns the function to call when the operation it covers has
// finished, which is where a duration is measured and where a span ends. A
// hook that returns a nil function receives no such call.
//
// Request and Attempt also return the context used for the operation, so a
// span started in one becomes the parent of the spans started under it.
// Returning the incoming context, or nil, leaves the context unchanged.
//
// The three hooks nest. A JMAP request has one attempt, or more if it is
// retried, and a wait may precede any attempt:
//
//	Request   Email/query, Email/get
//	  Wait    for a slot, where the server accepts two requests at once
//	  Attempt POST /jmap/api  429
//	  Wait    for the two seconds of the server's Retry-After
//	  Attempt POST /jmap/api  200
type Observer struct {
	// Request is called before a JMAP request is sent, ahead of the first
	// attempt. The function it returns is called once the response has been
	// decoded, or once the request has failed.
	Request func(ctx context.Context, info RequestInfo) (context.Context, func(ResponseInfo))

	// Attempt is called before each HTTP request the client sends. This
	// includes the requests that carry no JMAP calls — the session, an upload,
	// a download, the event stream — and each retry of any of them.
	Attempt func(ctx context.Context, info AttemptInfo) (context.Context, func(AttemptInfo, Answer))

	// Wait is called when the client delays a request instead of sending it:
	// either for one of the slots the server's maxConcurrentRequests allows,
	// or for the delay that follows a 429 or a 503. The function it returns is
	// called when the delay has elapsed.
	Wait func(ctx context.Context, info WaitInfo) func()

	// Redact, when set, makes the client put the body of each JMAP request
	// and response on RequestInfo and ResponseInfo, after passing it through
	// Redact. A body holds the subjects, addresses and text of the messages
	// a request reads and writes, which is why it is not reported unless
	// asked for, and why Redact is where it goes first: RedactContent keeps
	// the shape of the exchange and withholds the content, and KeepBodies
	// keeps everything.
	Redact func(json.RawMessage) json.RawMessage
}

// WithObserver makes the client report what it does to o.
func WithObserver(o *Observer) Option {
	return func(c *Client) { c.observer = o }
}

// RequestKind identifies the purpose of a request the client sends.
type RequestKind string

const (
	// KindAPI is a JMAP request posted to the API URL.
	KindAPI RequestKind = "api"
	// KindSession is a request for the session object.
	KindSession RequestKind = "session"
	// KindUpload is a blob upload to the upload URL.
	KindUpload RequestKind = "upload"
	// KindDownload is a blob download from the download URL.
	KindDownload RequestKind = "download"
	// KindEvents is a connection to the event source URL.
	KindEvents RequestKind = "events"
)

// RequestInfo describes a JMAP request the client is about to send.
type RequestInfo struct {
	// Calls lists the method calls of the request, in the order the request
	// holds them.
	Calls []CallInfo
	// Using holds the capability URIs the request declares.
	Using []string
	// Body is the request as JSON, after Observer.Redact, and nil where
	// Redact is not set.
	Body json.RawMessage
}

// CallInfo identifies one method call of a request: the method invoked, and
// the call id under which the response reports its result.
type CallInfo struct {
	Name   string
	CallID string
}

// ResponseInfo describes the outcome of a JMAP request.
type ResponseInfo struct {
	// Duration is the time the request took, including waits and retries.
	Duration time.Duration
	// Err is the error returned to the caller, and nil where the request was
	// answered. Method-level errors are not reported here; Errors holds
	// those.
	Err error
	// Errors holds the method-level errors of an answered request. The other
	// calls of the request may still have succeeded.
	Errors MethodErrors
	// Body is the response as JSON, after Observer.Redact, and nil where
	// Redact is not set or no response came. A request sent in several parts,
	// as WithSplitGets sends a /get, has its parts joined into one response
	// here, as the caller receives it.
	Body json.RawMessage
}

// AttemptInfo describes one HTTP request the client sends.
type AttemptInfo struct {
	// Kind says what the request is for.
	Kind RequestKind
	// Method and URL are the HTTP method and the address, with any credentials
	// removed from the URL.
	Method string
	URL    string
	// Attempt counts from one, and is higher only for a retry.
	Attempt int
}

// Answer is the outcome of one attempt.
type Answer struct {
	// Status is the HTTP status, and zero where no response was received.
	Status int
	// Duration is how long the attempt took.
	Duration time.Duration
	// Err is the transport error, and nil where a response was received. A
	// 4xx or 5xx status is not an error here.
	Err error
}

// WaitReason identifies why a request is delayed.
type WaitReason string

const (
	// WaitForSlot is a delay until one of the concurrent requests the server
	// allows has finished.
	WaitForSlot WaitReason = "slot"
	// WaitAfterRefusal is a delay before a retry, after a 429 or a 503.
	WaitAfterRefusal WaitReason = "refusal"
)

// WaitInfo describes a delay the client is about to apply.
type WaitInfo struct {
	// Reason identifies why the request is delayed.
	Reason WaitReason
	// Kind identifies the purpose of the delayed request.
	Kind RequestKind
	// Delay is the length of the delay where it is known in advance, which is
	// the case for WaitAfterRefusal and not for WaitForSlot.
	Delay time.Duration
}

// SlogObserver returns an Observer that writes to l at debug level, one
// record for each request, attempt and wait.
//
// It logs no failure at a higher level, because an error returned to the
// caller is logged by the caller. What it adds is the information the caller
// cannot obtain: which methods were sent in one request, how long the round
// trip took, and how much of that was spent waiting.
func SlogObserver(l *slog.Logger) *Observer {
	return &Observer{
		Request: func(ctx context.Context, info RequestInfo) (context.Context, func(ResponseInfo)) {
			return ctx, func(done ResponseInfo) {
				attrs := []any{
					slog.Any("calls", callNames(info.Calls)),
					slog.Duration("took", done.Duration),
				}
				if len(done.Errors) > 0 {
					attrs = append(attrs, slog.Int("method_errors", len(done.Errors)))
				}
				if done.Err != nil {
					attrs = append(attrs, slog.String("failed", done.Err.Error()))
				}
				if info.Body != nil {
					attrs = append(attrs, slog.String("request_body", string(info.Body)))
				}
				if done.Body != nil {
					attrs = append(attrs, slog.String("response_body", string(done.Body)))
				}
				l.DebugContext(ctx, "jmap request", attrs...)
			}
		},
		Attempt: func(ctx context.Context, info AttemptInfo) (context.Context, func(AttemptInfo, Answer)) {
			return ctx, func(info AttemptInfo, answer Answer) {
				attrs := []any{
					slog.String("kind", string(info.Kind)),
					slog.String("method", info.Method),
					slog.String("url", info.URL),
					slog.Int("attempt", info.Attempt),
					slog.Duration("took", answer.Duration),
				}
				if answer.Status != 0 {
					attrs = append(attrs, slog.Int("status", answer.Status))
				}
				if answer.Err != nil {
					attrs = append(attrs, slog.String("failed", answer.Err.Error()))
				}
				l.DebugContext(ctx, "http attempt", attrs...)
			}
		},
		Wait: func(ctx context.Context, info WaitInfo) func() {
			started := time.Now()
			return func() {
				l.DebugContext(ctx, "waited",
					slog.String("reason", string(info.Reason)),
					slog.String("kind", string(info.Kind)),
					slog.Duration("took", time.Since(started)))
			}
		},
	}
}

// callNames renders the calls of a request for a log record.
func callNames(calls []CallInfo) []string {
	names := make([]string, len(calls))
	for i, call := range calls {
		names[i] = call.Name
	}
	return names
}

// observeRequest reports a JMAP request to the observer, and returns the
// function that reports its outcome.
func (c *Client) observeRequest(ctx context.Context, r *Request, withBody bool) (context.Context, func(*Response, error, MethodErrors)) {
	if c.observer == nil || c.observer.Request == nil {
		return ctx, func(*Response, error, MethodErrors) {}
	}
	info := RequestInfo{Using: r.Using}
	if withBody {
		info.Body = c.observedBody(r)
	}
	for _, call := range r.MethodCalls {
		info.Calls = append(info.Calls, CallInfo{Name: call.Name, CallID: call.CallID})
	}
	started := time.Now()
	under, done := c.observer.Request(ctx, info)
	if under != nil {
		ctx = under
	}
	if done == nil {
		return ctx, func(*Response, error, MethodErrors) {}
	}
	return ctx, func(resp *Response, err error, errs MethodErrors) {
		answer := ResponseInfo{Duration: time.Since(started), Err: err, Errors: errs}
		if resp != nil {
			answer.Body = c.observedBody(resp)
		}
		done(answer)
	}
}

// bodiesObserved reports whether the observer is given the bodies of
// requests.
func (c *Client) bodiesObserved() bool {
	return c.observer != nil && c.observer.Request != nil && c.observer.Redact != nil
}

// encodedOnce returns a copy of r whose arguments are each JSON already. A
// request may be encoded more than once — to observe it, to split it, to send
// it — and an argument with a MarshalJSON of its own would otherwise be called
// each time, which an observer must not cause.
func encodedOnce(r *Request) (*Request, error) {
	copied := *r
	copied.MethodCalls = make([]Invocation, len(r.MethodCalls))
	for i, call := range r.MethodCalls {
		raw, err := json.Marshal(call.Args)
		if err != nil {
			return nil, fmt.Errorf("jmapc: encoding request: %w", err)
		}
		call.Args = json.RawMessage(raw)
		copied.MethodCalls[i] = call
	}
	return &copied, nil
}

// observedBody returns v as JSON through the observer's Redact, or nil where
// there is no Redact to pass it through.
func (c *Client) observedBody(v any) json.RawMessage {
	if c.observer.Redact == nil {
		return nil
	}
	body, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return c.observer.Redact(body)
}

// observeAttempt reports an HTTP request to the observer, and returns the
// function that reports the response.
func (c *Client) observeAttempt(req *http.Request, kind RequestKind, attempt int) (*http.Request, func(*http.Response, error)) {
	if c.observer == nil || c.observer.Attempt == nil {
		return req, func(*http.Response, error) {}
	}
	info := AttemptInfo{
		Kind:    kind,
		Method:  req.Method,
		URL:     req.URL.Redacted(),
		Attempt: attempt,
	}
	started := time.Now()
	under, done := c.observer.Attempt(req.Context(), info)
	if under != nil && under != req.Context() {
		req = req.WithContext(under)
	}
	if done == nil {
		return req, func(*http.Response, error) {}
	}
	return req, func(resp *http.Response, err error) {
		answer := Answer{Duration: time.Since(started), Err: err}
		if resp != nil {
			answer.Status = resp.StatusCode
		}
		done(info, answer)
	}
}

// observeWait reports a delay to the observer, and returns the function that
// reports its end.
func (c *Client) observeWait(ctx context.Context, info WaitInfo) func() {
	if c.observer == nil || c.observer.Wait == nil {
		return func() {}
	}
	done := c.observer.Wait(ctx, info)
	if done == nil {
		return func() {}
	}
	return done
}

// waiting returns the callback a limiter uses when a request has to wait for
// one of the slots the server allows.
func (c *Client) waiting(ctx context.Context, kind RequestKind) func() func() {
	return func() func() {
		return c.observeWait(ctx, WaitInfo{Reason: WaitForSlot, Kind: kind})
	}
}
