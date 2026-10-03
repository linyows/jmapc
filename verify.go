package jmapc

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RequestNeeds is what one generated request needs of the server, as far as it
// can be known before the request is sent. The generated Verify function holds
// one for every request in its package.
type RequestNeeds struct {
	// Name is the request's name, which a problem is reported under.
	Name string
	// Using is the capabilities the request declares.
	Using []string
	// Calls is how many method calls the request makes.
	Calls int
	// PrimaryAccounts is the capabilities whose primary account fills in the
	// accountId of a call that leaves it out.
	PrimaryAccounts []string
}

// Verify checks every request against the session, and reports everything
// that would stop one of them from being sent: a capability the server does not
// advertise, a primary account the session does not name, an account that
// does not support the capability it is used for, or more calls than the
// server takes in one request.
//
// The same checks run before each request is sent, and fail it there. Verify
// runs them for every request at once, so that a program can call it as it
// starts and learn that a request on a path it rarely takes will fail, rather
// than learn it when the path is taken. It reports every problem it finds,
// joined, each naming its request, so that fixing one does not merely reveal
// the next.
//
// It fetches the session if the client does not hold one yet, and returns the
// error if that fails.
func (c *Client) Verify(ctx context.Context, requests ...RequestNeeds) error {
	s, err := c.Session(ctx)
	if err != nil {
		return err
	}
	var problems []error
	for _, r := range requests {
		for _, err := range r.check(s) {
			problems = append(problems, &VerifyError{Request: r.Name, Err: err})
		}
	}
	return errors.Join(problems...)
}

// VerifyError is one problem Verify found with one request.
type VerifyError struct {
	// Request is the name of the request the problem is with.
	Request string
	// Err is the problem, as sending the request would have reported it: a
	// *RequestError for a capability or a limit the request would be refused
	// for, the error PrimaryAccountID returns for a primary account the
	// session does not name, and a *MethodError of type
	// accountNotSupportedByMethod for an account that does not support what
	// the request uses it for.
	Err error
}

func (e *VerifyError) Error() string {
	// The request was not sent, so "request failed", which is how a
	// RequestError reads, or "method failed", which is how a MethodError
	// does, would say something that did not happen.
	var refused *RequestError
	if errors.As(e.Err, &refused) && refused.Detail != "" {
		return fmt.Sprintf("jmapc: %s would be refused: %s", e.Request, refused.Detail)
	}
	var method *MethodError
	if errors.As(e.Err, &method) && method.Description != "" {
		return fmt.Sprintf("jmapc: %s would be refused: %s", e.Request, method.Description)
	}
	return fmt.Sprintf("jmapc: %s: %s", e.Request, strings.TrimPrefix(e.Err.Error(), "jmapc: "))
}

func (e *VerifyError) Unwrap() error { return e.Err }

// check returns every problem the session shows with the request.
func (n RequestNeeds) check(s *Session) []error {
	var problems []error
	missing := make(map[string]bool)
	for _, uri := range n.Using {
		if err := s.requireCapability(uri); err != nil {
			missing[uri] = true
			problems = append(problems, err)
		}
	}
	for _, capability := range n.PrimaryAccounts {
		if missing[capability] {
			// Already reported, and an account for a capability the server
			// lacks says nothing more.
			continue
		}
		// The generated function looks the account up the same way, and
		// fails with this error before sending anything.
		id, err := s.PrimaryAccountID(capability)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		// The server is what refuses this one, answering the call with a
		// method error.
		if !s.accountSupports(id, capability) {
			problems = append(problems, &MethodError{
				Type:        ErrAccountNotSupport,
				Description: fmt.Sprintf("the primary account %q for %s does not support it", id, capability),
			})
		}
	}
	if err := s.requireCalls(n.Calls); err != nil {
		problems = append(problems, err)
	}
	return problems
}

// accountSupports reports whether an account offers a capability. A session
// that lists nothing for an account is not saying the account supports
// nothing, so that is taken as support.
func (s *Session) accountSupports(id ID, capability string) bool {
	account, ok := s.Accounts[id]
	if !ok || len(account.AccountCapabilities) == 0 {
		return true
	}
	_, offered := account.AccountCapabilities[capability]
	return offered
}
