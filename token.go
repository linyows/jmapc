package jmapc

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Token is a bearer token, and the time it stops being accepted where the
// source knows it. A zero Expiry means the source does not know, and the token
// is then used until a server refuses it.
type Token struct {
	// Value is the token itself, sent as "Authorization: Bearer <value>".
	Value string
	// Expiry is when the token stops being accepted, or the zero time where
	// that is not known.
	Expiry time.Time
}

// TokenSource returns the token to authenticate with. The client calls it when
// it has no token, when the one it holds has expired, and when a server has
// refused the one it holds.
type TokenSource func(ctx context.Context) (Token, error)

// tokenMargin is how long before its expiry a token is treated as expired, so
// that one which expires while the request is in flight is replaced before it
// is sent rather than after it is refused.
const tokenMargin = 10 * time.Second

// WithTokenSource authenticates with a bearer token fetched when one is
// needed, which is what an OAuth 2.0 access token requires: it expires, and a
// client built around a fixed string has to be rebuilt to replace it, losing
// the cached session and the count of the requests in flight with it.
//
// The token is held until it expires. A source that reports an expiry is
// called again shortly before it, and one that reports none is called again
// only when a server answers 401. Whichever it is, requests arriving together
// share one call: a source that exchanges a refresh token is not asked to do
// so several times at once.
//
// A 401 also causes the one request that received it to be sent again, once,
// with a newly fetched token, since a token that has just been refused is
// worth replacing before the caller is told the request failed. That is
// separate from WithRetry, which retries what a server said it did not carry
// out.
func WithTokenSource(src TokenSource) Option {
	return func(c *Client) { c.tokens = &tokenHolder{src: src} }
}

// tokenHolder keeps the token a source last returned, and calls the source
// again when it is needed.
type tokenHolder struct {
	src TokenSource

	mu sync.Mutex
	// held is the token in use, and is zero where there is none.
	held Token
	// fetching is closed when the call in progress returns, and is nil where
	// there is none. It is what makes several requests arriving at once share
	// one call to the source.
	fetching chan struct{}
	// err is what the last call to the source returned.
	err error
}

// valid reports whether the token held may still be used. The caller holds mu.
func (t *tokenHolder) valid() bool {
	if t.held.Value == "" {
		return false
	}
	return t.held.Expiry.IsZero() || time.Now().Before(t.held.Expiry.Add(-tokenMargin))
}

// token returns the token to send, calling the source where there is none to
// send or the one held has expired.
func (t *tokenHolder) token(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.valid() {
		value := t.held.Value
		t.mu.Unlock()
		return value, nil
	}
	if wait := t.fetching; wait != nil {
		t.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return t.fetched()
	}

	wait := make(chan struct{})
	t.fetching = wait
	t.mu.Unlock()

	held, err := t.src(ctx)
	t.mu.Lock()
	t.fetching, t.err = nil, err
	if err == nil {
		t.held = held
	}
	t.mu.Unlock()
	close(wait)

	if err != nil {
		return "", fmt.Errorf("jmapc: fetching a token: %w", err)
	}
	if held.Value == "" {
		return "", fmt.Errorf("jmapc: the token source returned an empty token")
	}
	return held.Value, nil
}

// fetched reports the result of the call another request made, rather than
// making one of its own.
func (t *tokenHolder) fetched() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.valid() {
		return t.held.Value, nil
	}
	if t.err != nil {
		return "", fmt.Errorf("jmapc: fetching a token: %w", t.err)
	}
	return "", fmt.Errorf("jmapc: the token source returned a token that is already expired")
}

// discard drops the token held, so that the next request fetches another. It
// is called where a server has refused the one that was sent.
func (t *tokenHolder) discard() {
	t.mu.Lock()
	t.held = Token{}
	t.mu.Unlock()
}

// authorize puts the token on a request, fetching one where it has to.
func (c *Client) authorize(req *http.Request) error {
	if c.tokens == nil {
		return nil
	}
	value, err := c.tokens.token(req.Context())
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+value)
	return nil
}

// refusedToken reports whether a response is a server refusing the token that
// was sent, which is worth replacing before the caller is told the request
// failed.
func (c *Client) refusedToken(resp *http.Response) bool {
	return c.tokens != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized
}
