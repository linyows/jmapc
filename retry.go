package jmapc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy says when a client sends a request again, and how long it waits
// first.
//
// The default is to try again only where the server has said it did nothing —
// a 429 or a 503 — because a request that may have been carried out is not
// safe to repeat. A JMAP request creates records, and a /set sent twice
// creates twice.
type RetryPolicy struct {
	// Attempts is how many times a request is sent in all, counting the first.
	// Zero or one sends it once.
	Attempts int
	// Worth reports whether an answer is worth another try. A nil Worth means
	// the two answers that say the server did nothing: 429 and 503.
	//
	// It is called with the response, or with the error where the request did
	// not come back at all. Retrying an error means retrying a request that
	// may have been carried out, so the default does not.
	Worth func(*http.Response, error) bool
	// Wait returns how long to wait before the nth attempt, counting from two.
	// The server's Retry-After is passed as it was given, or zero where it
	// said nothing. A nil Wait honours it, and falls back to a doubling wait
	// from a fifth of a second to half a minute where the server asked for
	// nothing in particular.
	//
	// However long it asks for, a client that has been told to wait longer
	// than a minute stops waiting and reports the refusal: a server asking for
	// an hour is telling the caller to come back later rather than asking a
	// request to sit in memory until then.
	Wait func(attempt int, retryAfter time.Duration) time.Duration
}

// The waits a policy uses where it says nothing itself.
const (
	minRetryWait = 200 * time.Millisecond
	maxRetryWait = 30 * time.Second
	// maxRetryAfter caps how long a server can ask the client to wait before
	// the client stops waiting and reports the refusal instead. A server that
	// asks for an hour is telling the caller to come back later, not asking a
	// request to sit in memory until then.
	maxRetryAfter = time.Minute
)

// WithRetry makes the client send a request again where the server said it
// could not take it now, up to the given number of attempts counting the
// first. Two or fewer than two turns it off.
//
// What is retried is a 429 or a 503, the two answers that say the request was
// not carried out, and the wait is the server's Retry-After where it gave one.
// A request that failed on the way there or on the way back is not retried,
// since it may have been carried out; WithRetryPolicy is where to say
// otherwise for a client whose requests are safe to repeat.
func WithRetry(attempts int) Option {
	return func(c *Client) { c.retry = RetryPolicy{Attempts: attempts} }
}

// WithRetryPolicy replaces the whole policy: which answers are worth another
// try, and how long to wait before each.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Client) { c.retry = p }
}

// worthRetrying reports whether an answer is worth another try under the
// policy.
func (p RetryPolicy) worthRetrying(resp *http.Response, err error) bool {
	if p.Worth != nil {
		return p.Worth(resp, err)
	}
	if err != nil || resp == nil {
		// The request may have been carried out, and there is no way to tell
		// from here.
		return false
	}
	return resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode == http.StatusServiceUnavailable
}

// wait returns how long to wait before the nth attempt.
func (p RetryPolicy) wait(attempt int, retryAfter time.Duration) time.Duration {
	if p.Wait != nil {
		return p.Wait(attempt, retryAfter)
	}
	if retryAfter > 0 {
		return retryAfter
	}
	d := minRetryWait
	for i := 2; i < attempt && d < maxRetryWait; i++ {
		d *= 2
	}
	if d > maxRetryWait {
		return maxRetryWait
	}
	return d
}

// retryAfter reads what the server asked the client to wait, which RFC 9110
// writes either as a number of seconds or as a date.
func retryAfter(resp *http.Response, now time.Time) time.Duration {
	if resp == nil {
		return 0
	}
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if d := when.Sub(now); d > 0 {
		return d
	}
	return 0
}

// sendWithRetry sends a request, and sends it again while the policy says the
// answer is worth another try.
//
// A request can only be sent again if its body can be read again, which the
// standard library arranges for a body it knows the length of. An upload
// reading from a file or a network stream has no such body, and is sent once.
func (c *Client) sendWithRetry(req *http.Request, kind RequestKind) (*http.Response, error) {
	attempts := c.retry.Attempts
	if attempts < 2 || (req.Body != nil && req.GetBody == nil) {
		return c.send(req, kind, 1)
	}
	for attempt := 1; ; attempt++ {
		resp, err := c.send(req, kind, attempt)
		if attempt >= attempts || !c.retry.worthRetrying(resp, err) {
			return resp, err
		}
		after := retryAfter(resp, time.Now())
		if after > maxRetryAfter {
			// The server is not asking the client to wait; it is telling the
			// caller to come back later.
			return resp, err
		}
		delay := c.retry.wait(attempt+1, after)
		// The answer is being thrown away, so the connection is given back
		// rather than left for the finaliser.
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
			resp.Body.Close()
		}
		if delay > 0 {
			waited := c.observeWait(req.Context(), WaitInfo{
				Reason: WaitAfterRefusal,
				Kind:   kind,
				Delay:  delay,
			})
			err := sleep(req.Context(), delay)
			waited()
			if err != nil {
				return nil, err
			}
		}
		next, err := again(req)
		if err != nil {
			return nil, err
		}
		req = next
	}
}

// again returns the request to send next time, with its body rewound.
func again(req *http.Request) (*http.Request, error) {
	next := req.Clone(req.Context())
	if req.GetBody == nil {
		return next, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("jmapc: rewinding the request to send it again: %w", err)
	}
	next.Body = body
	return next, nil
}

// sleep waits, or gives up as soon as the context ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
