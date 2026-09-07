package jmapc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies this client to servers.
const DefaultUserAgent = "jmapc/0.1 (+https://github.com/linyows/jmapc)"

// maxErrorBody caps how much of an unparseable error response is kept for the
// error message.
const maxErrorBody = 8 << 10

// Client sends JMAP requests to one server. It is safe for concurrent use, and
// it caches the Session object so that repeated queries cost one round trip
// each.
type Client struct {
	sessionURL string
	apiURL     string
	httpClient *http.Client
	editors    []func(*http.Request) error
	userAgent  string
	strict     bool
	retry      RetryPolicy
	observer   *Observer
	tokens     *tokenHolder
	splitGets  bool
	refresh    bool

	// api and uploads limit the client to the number of each the server
	// accepts at once.
	api     limiter
	uploads limiter

	mu      sync.Mutex
	session *Session
	// stale records that a response reported a session other than the one
	// held, so that the next caller needing the session fetches it again.
	stale bool
	// fetching is closed when the fetch in progress returns, and is nil where
	// there is none. It is what makes requests arriving together share one
	// fetch of the session resource.
	fetching chan struct{}
	// fetchErr is what the last fetch returned, for the callers that waited on
	// it rather than making one of their own.
	fetchErr error
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient makes the client issue its requests through hc, which is where
// timeouts, proxies, and transport-level instrumentation belong.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBearerToken authenticates with an OAuth 2.0 bearer token or an
// equivalent API token. Use WithTokenSource for a token that expires.
func WithBearerToken(token string) Option {
	return WithRequestEditor(func(r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
}

// WithBasicAuth authenticates with HTTP Basic credentials.
func WithBasicAuth(username, password string) Option {
	return WithRequestEditor(func(r *http.Request) error {
		r.SetBasicAuth(username, password)
		return nil
	})
}

// WithHeader sets a header on every request the client makes.
func WithHeader(key, value string) Option {
	return WithRequestEditor(func(r *http.Request) error {
		r.Header.Set(key, value)
		return nil
	})
}

// WithRequestEditor runs f on every outgoing HTTP request before it is sent,
// which covers authentication schemes the options above do not.
func WithRequestEditor(f func(*http.Request) error) Option {
	return func(c *Client) { c.editors = append(c.editors, f) }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithAPIURL posts requests straight to apiURL instead of the apiUrl the
// session advertises. The session is still fetched when something needs it,
// such as resolving a primary account id.
func WithAPIURL(apiURL string) Option {
	return func(c *Client) { c.apiURL = apiURL }
}

// WithoutPreflightChecks stops the client from validating a request against the
// session's advertised capabilities and limits before sending it. The checks
// turn a wasted round trip into a local error, so leave them on unless a server
// under-reports what it supports.
func WithoutPreflightChecks() Option {
	return func(c *Client) { c.strict = false }
}

// WithoutSessionRefresh stops the client from fetching the session again when a
// response reports that the server's has changed. The session then stays as it
// was first fetched, which is what a client that never outlives a change wants,
// and a long-running one does not: an account added or removed, a limit
// changed, an endpoint moved, and a key rotated all reach a client only through
// the session.
func WithoutSessionRefresh() Option {
	return func(c *Client) { c.refresh = false }
}

// New returns a client that discovers the server through the session resource
// at sessionURL. Use WellKnownURL to build that URL from a bare hostname.
func New(sessionURL string, opts ...Option) *Client {
	c := &Client{
		sessionURL: sessionURL,
		httpClient: http.DefaultClient,
		userAgent:  DefaultUserAgent,
		strict:     true,
		refresh:    true,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Session returns the server's Session object. It is fetched on first use, and
// again where a response has reported a sessionState other than the one the
// cached session carries, unless the client was given WithoutSessionRefresh.
//
// A fetch that fails is not passed on to the caller once there is a session to
// fall back on: the one held is out of date, which is what it was a moment ago,
// and the fetch is made again the next time the session is needed. The failure
// is reported to an Observer as a request of KindSession that did not succeed.
func (c *Client) Session(ctx context.Context) (*Session, error) {
	c.mu.Lock()
	s, stale := c.session, c.stale
	c.mu.Unlock()
	if s != nil && !stale {
		return s, nil
	}
	fresh, err := c.fetchSession(ctx)
	if err != nil {
		if s != nil {
			return s, nil
		}
		return nil, err
	}
	return fresh, nil
}

// RefreshSession fetches the Session object and replaces the cached copy,
// whether or not a response has reported that it changed. A client that is
// told about a change by some other means calls it; one that learns of it from
// a response does not have to, since the next call to Session fetches it.
func (c *Client) RefreshSession(ctx context.Context) (*Session, error) {
	return c.fetchSession(ctx)
}

// fetchSession fetches the session, or waits for the fetch another caller
// started rather than making a second one. Requests arriving together after a
// change therefore cost one request to the session resource between them.
func (c *Client) fetchSession(ctx context.Context) (*Session, error) {
	c.mu.Lock()
	if wait := c.fetching; wait != nil {
		c.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		s, err := c.session, c.fetchErr
		c.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	wait := make(chan struct{})
	c.fetching = wait
	c.mu.Unlock()

	s, err := c.getSession(ctx)

	c.mu.Lock()
	c.fetching, c.fetchErr = nil, err
	if err == nil {
		c.session, c.stale = s, false
	}
	c.mu.Unlock()
	close(wait)
	return s, err
}

// noteSessionState records that a response reported a session other than the
// one held. RFC 8620 has the server return its sessionState on every response
// so that a client can tell, without asking, that the session it holds no
// longer describes the server.
func (c *Client) noteSessionState(state string) {
	if !c.refresh || state == "" {
		return
	}
	c.mu.Lock()
	// A session carrying no state of its own says nothing about whether it has
	// changed, and comparing against it would fetch the session on every
	// response.
	if c.session != nil && c.session.State != "" && c.session.State != state {
		c.stale = true
	}
	c.mu.Unlock()
}

// getSession fetches and decodes the session resource. It is fetchSession
// without the sharing, and the caller stores what it returns.
func (c *Client) getSession(ctx context.Context) (*Session, error) {
	if c.sessionURL == "" {
		return nil, fmt.Errorf("jmapc: no session URL configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.sessionURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building session request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.sendWithRetry(req, KindSession)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.requestError(resp)
	}
	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("jmapc: decoding session: %w", err)
	}
	if s.APIURL == "" && c.apiURL == "" {
		return nil, fmt.Errorf("jmapc: session from %s has no apiUrl", c.sessionURL)
	}
	sessionURL, err := url.Parse(c.sessionURL)
	if err != nil {
		return nil, fmt.Errorf("jmapc: parsing session URL: %w", err)
	}
	s.resolveURLs(sessionURL)
	return &s, nil
}

// Do sends one JMAP request and returns the decoded response. Method-level
// errors do not fail the call: the response is returned alongside a
// MethodErrors describing the calls the server could not execute, because the
// remaining calls may still have produced usable results.
func (c *Client) Do(ctx context.Context, r *Request) (*Response, error) {
	// The report starts before anything is sent, so that the session fetch a
	// first request triggers is included in the request's duration.
	ctx, answered := c.observeRequest(ctx, r)

	apiURL, err := c.resolveAPIURL(ctx, r)
	if err != nil {
		answered(err, nil)
		return nil, err
	}

	// A /get holding more ids than the server takes is sent in several
	// requests, where the client was told to send it that way at all.
	first, parts := c.planSplit(r)
	resp, err := c.post(ctx, apiURL, first)
	if err != nil {
		answered(err, nil)
		return nil, err
	}
	resp.req = r

	var split []error
	for _, part := range parts {
		answer, err := c.post(ctx, apiURL, part.request)
		if err != nil {
			answered(err, nil)
			return nil, err
		}
		if err := joinSplit(resp, answer, part.chunks); err != nil {
			split = append(split, err)
		}
	}

	errs := resp.Errors()
	if len(errs) > 0 {
		answered(nil, errs)
		return resp, errors.Join(append(split, errs)...)
	}
	answered(nil, nil)
	if len(split) > 0 {
		return resp, errors.Join(split...)
	}
	return resp, nil
}

// post sends one request and decodes what comes back. It is one round trip:
// the request a caller made is one, and each further request a split needs is
// another.
func (c *Client) post(ctx context.Context, apiURL string, r *Request) (*Response, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("jmapc: encoding request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jmapc: building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Accept", "application/json")

	// The server states how many requests it accepts at once, and this is one.
	release, err := c.api.hold(ctx, c.limit(func(core *CoreCapability) UnsignedInt {
		return core.MaxConcurrentRequests
	}), c.waiting(ctx, KindAPI))
	if err != nil {
		return nil, err
	}
	defer release()

	httpResp, err := c.sendWithRetry(httpReq, KindAPI)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return nil, c.requestError(httpResp)
	}
	var resp Response
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("jmapc: decoding response: %w", err)
	}
	c.noteSessionState(resp.SessionState)
	return &resp, nil
}

// resolveAPIURL determines where to post r, running the preflight checks along
// the way when a session is available.
func (c *Client) resolveAPIURL(ctx context.Context, r *Request) (string, error) {
	if c.apiURL != "" && !c.strict {
		return c.apiURL, nil
	}
	if c.apiURL != "" && c.sessionURL == "" {
		return c.apiURL, nil
	}
	s, err := c.Session(ctx)
	if err != nil {
		return "", err
	}
	if c.strict {
		if err := preflight(s, r); err != nil {
			return "", err
		}
	}
	if c.apiURL != "" {
		return c.apiURL, nil
	}
	return s.APIURL, nil
}

// preflight rejects a request the session already shows the server will not
// accept, so that a missing capability or an oversized batch surfaces as a
// local error instead of a round trip.
func preflight(s *Session, r *Request) error {
	for _, uri := range r.Using {
		if !s.HasCapability(uri) {
			detail := fmt.Sprintf("server does not support %s", uri)
			if len(s.Capabilities) == 0 {
				// An empty capabilities map is what a minimal server or a test
				// stub produces, and it does not mean the same as a server
				// that listed its capabilities and omitted this one.
				detail = fmt.Sprintf("session lists no capabilities at all, so it cannot say whether it supports %s", uri)
			}
			return &RequestError{
				Type:   ErrTypeUnknownCapability,
				Detail: detail,
			}
		}
	}
	core, err := s.Core()
	if err != nil {
		// A server that does not describe its core limits is unusual but not
		// fatal; the remaining checks simply cannot run.
		return nil
	}
	if max := core.MaxCallsInRequest; max > 0 && UnsignedInt(len(r.MethodCalls)) > max {
		return &RequestError{
			Type:   ErrTypeLimit,
			Limit:  "maxCallsInRequest",
			Detail: fmt.Sprintf("request has %d method calls, server allows %d", len(r.MethodCalls), max),
		}
	}
	return nil
}

// send applies the configured request editors and performs the HTTP request.
func (c *Client) send(req *http.Request, kind RequestKind, attempt int) (*http.Response, error) {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	for _, edit := range c.editors {
		if err := edit(req); err != nil {
			return nil, fmt.Errorf("jmapc: preparing request: %w", err)
		}
	}
	// After the editors, so that the token sent is the one the source last
	// returned rather than one an editor set from a value captured earlier.
	if err := c.authorize(req); err != nil {
		return nil, err
	}
	req, came := c.observeAttempt(req, kind, attempt)
	resp, err := c.httpClient.Do(req)
	came(resp, err)
	if err != nil {
		// Redacted, because a URL may carry credentials in its userinfo, and
		// this error may be written to a log.
		return nil, fmt.Errorf("jmapc: %s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	return resp, nil
}

// requestError turns a non-200 response into a *RequestError, decoding the RFC
// 7807 problem details document when the server sent one.
func (c *Client) requestError(resp *http.Response) error {
	e := &RequestError{Status: resp.StatusCode, RetryAfter: retryAfter(resp, time.Now())}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil || len(body) == 0 {
		return e
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/problem+json") || strings.HasPrefix(ct, "application/json") {
		// Decoding overwrites the fields the document names, so what was read
		// from the response itself is put back afterwards.
		delay := e.RetryAfter
		if err := json.Unmarshal(body, e); err == nil && e.Type != "" {
			e.Status, e.RetryAfter = resp.StatusCode, delay
			return e
		}
	}
	e.Detail = strings.TrimSpace(string(body))
	return e
}
