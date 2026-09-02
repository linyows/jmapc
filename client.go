package jmapc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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

	mu      sync.Mutex
	session *Session
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient makes the client issue its requests through hc, which is where
// timeouts, proxies, and transport-level instrumentation belong.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBearerToken authenticates with an OAuth 2.0 bearer token or an
// equivalent API token.
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

// New returns a client that discovers the server through the session resource
// at sessionURL. Use WellKnownURL to build that URL from a bare hostname.
func New(sessionURL string, opts ...Option) *Client {
	c := &Client{
		sessionURL: sessionURL,
		httpClient: http.DefaultClient,
		userAgent:  DefaultUserAgent,
		strict:     true,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Session returns the server's Session object, fetching it on first use and
// caching it afterwards.
func (c *Client) Session(ctx context.Context) (*Session, error) {
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s != nil {
		return s, nil
	}
	return c.RefreshSession(ctx)
}

// RefreshSession re-fetches the Session object and replaces the cached copy.
// Call it when a response reports a sessionState different from the one the
// cached session carries.
func (c *Client) RefreshSession(ctx context.Context) (*Session, error) {
	if c.sessionURL == "" {
		return nil, fmt.Errorf("jmapc: no session URL configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.sessionURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jmapc: building session request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.send(req)
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
	if s.APIURL == "" {
		return nil, fmt.Errorf("jmapc: session from %s has no apiUrl", c.sessionURL)
	}
	c.mu.Lock()
	c.session = &s
	c.mu.Unlock()
	return &s, nil
}

// Do sends one JMAP request and returns the decoded response. Method-level
// errors do not fail the call: the response is returned alongside a
// MethodErrors describing the calls the server could not execute, because the
// remaining calls may still have produced usable results.
func (c *Client) Do(ctx context.Context, r *Request) (*Response, error) {
	apiURL, err := c.resolveAPIURL(ctx, r)
	if err != nil {
		return nil, err
	}
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
	httpResp, err := c.send(httpReq)
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
	resp.req = r
	if errs := resp.Errors(); len(errs) > 0 {
		return &resp, errs
	}
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
			return &RequestError{
				Type:   ErrTypeUnknownCapability,
				Detail: fmt.Sprintf("server does not support %s", uri),
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
func (c *Client) send(req *http.Request) (*http.Response, error) {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	for _, edit := range c.editors {
		if err := edit(req); err != nil {
			return nil, fmt.Errorf("jmapc: preparing request: %w", err)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmapc: %s %s: %w", req.Method, req.URL, err)
	}
	return resp, nil
}

// requestError turns a non-200 response into a *RequestError, decoding the RFC
// 7807 problem details document when the server sent one.
func (c *Client) requestError(resp *http.Response) error {
	e := &RequestError{Status: resp.StatusCode}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil || len(body) == 0 {
		return e
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/problem+json") || strings.HasPrefix(ct, "application/json") {
		if err := json.Unmarshal(body, e); err == nil && e.Type != "" {
			e.Status = resp.StatusCode
			return e
		}
	}
	e.Detail = strings.TrimSpace(string(body))
	return e
}
