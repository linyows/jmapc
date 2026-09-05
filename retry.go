package jmapc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy determines when a client sends a request again, and how long it
// waits first.
//
// The default is to retry only where the server reported that it did nothing —
// a 429 or a 503 — because a request that may have been carried out is not
// safe to repeat. A JMAP request creates records, and a /set sent twice
// creates twice.
type RetryPolicy struct {
	// Attempts is how many times a request is sent in all, counting the first.
	// Zero or one sends it once.
	Attempts int
	// Worth reports whether a response is worth another attempt. A nil Worth
	// selects the two responses that report the server did nothing: 429 and
	// 503.
	//
	// It is called with the response, or with the error where no response was
	// received. Retrying an error means retrying a request that may have been
	// carried out, so the default does not.
	Worth func(*http.Response, error) bool
	// Wait returns how long to wait before the nth attempt, counting from two.
	// The server's Retry-After is passed as it was received, or zero where the
	// server sent none. A nil Wait uses that value, and falls back to a delay
	// that doubles from 0.2 seconds to 30 seconds where the server sent none.
	//
	// A Retry-After longer than a minute is not waited out. The client stops
	// and reports the refusal instead, because holding a request in memory for
	// that long is of no use to the caller, which should retry later.
	Wait func(attempt int, retryAfter time.Duration) time.Duration
}

// The delays a policy uses where it specifies none of its own.
const (
	minRetryWait = 200 * time.Millisecond
	maxRetryWait = 30 * time.Second
	// maxRetryAfter caps the Retry-After the client waits out. Beyond it the
	// client reports the refusal instead, because holding a request in memory
	// for an hour is of no use to the caller, which should retry later.
	maxRetryAfter = time.Minute
)

// WithRetry makes the client send a request again where the server answered
// 429 or 503, up to the given number of attempts counting the first. Two or
// fewer than two disables it.
//
// Those are the two responses that report the request was not carried out, and
// the delay is the server's Retry-After where it sent one. A request that
// failed in transit is not retried, since it may have been carried out;
// WithRetryPolicy overrides that for a client whose requests are safe to
// repeat.
func WithRetry(attempts int) Option {
	return func(c *Client) { c.retry = RetryPolicy{Attempts: attempts} }
}

// WithRetryPolicy replaces the whole policy: which responses are worth another
// attempt, and how long to wait before each.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Client) { c.retry = p }
}

// worthRetrying reports whether a response is worth another attempt under the
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

// retryAfter reads the delay the server requested in Retry-After, which RFC
// 9110 writes either as a number of seconds or as a date.
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

// sendWithRetry sends a request, and sends it again while the policy reports
// the response is worth another attempt.
//
// A request can only be sent again if its body can be read again, which the
// standard library provides for a body whose length it knows. An upload
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
			// A delay this long is not one to wait out; the caller is
			// expected to retry later.
			return resp, err
		}
		delay := c.retry.wait(attempt+1, after)
		// The response is discarded, so the connection is released rather than
		// left to the finaliser.
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

// sleep waits, or returns as soon as the context ends.
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
