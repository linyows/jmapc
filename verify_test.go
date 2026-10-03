package jmapc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// verifyProblems returns the problems Verify reported, one per request and
// reason.
func verifyProblems(t *testing.T, err error) []*VerifyError {
	t.Helper()
	if err == nil {
		return nil
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("Verify returned %T (%v), want the problems joined", err, err)
	}
	var problems []*VerifyError
	for _, e := range joined.Unwrap() {
		var v *VerifyError
		if !errors.As(e, &v) {
			t.Fatalf("Verify reported %T (%v), want a *VerifyError", e, e)
		}
		problems = append(problems, v)
	}
	return problems
}

func TestVerifyAcceptsWhatTheSessionSupports(t *testing.T) {
	ts := newTestServer(t)
	err := ts.client().Verify(context.Background(), RequestNeeds{
		Name:            "ListInboxEmails",
		Using:           []string{CapabilityCore, CapabilityMail},
		Calls:           2,
		PrimaryAccounts: []string{CapabilityMail},
	})
	if err != nil {
		t.Errorf("Verify: %v", err)
	}
}

// TestVerifyReportsEveryProblem checks that Verify does not stop at the first
// request it finds wrong: a program that starts, fixes one problem and starts
// again should not have to do so once per problem.
func TestVerifyReportsEveryProblem(t *testing.T) {
	ts := newTestServer(t)
	err := ts.client().Verify(context.Background(),
		RequestNeeds{
			Name:            "FindPeople",
			Using:           []string{CapabilityCore, CapabilityContacts},
			Calls:           1,
			PrimaryAccounts: []string{CapabilityContacts},
		},
		RequestNeeds{
			Name:  "TooManyCalls",
			Using: []string{CapabilityCore, CapabilityMail},
			Calls: 3,
		},
		RequestNeeds{
			Name:            "NoPrimaryAccount",
			Using:           []string{CapabilityCore},
			Calls:           1,
			PrimaryAccounts: []string{CapabilityCore},
		},
		RequestNeeds{
			Name:  "Fine",
			Using: []string{CapabilityCore, CapabilityMail},
			Calls: 1,
		},
	)
	problems := verifyProblems(t, err)
	got := map[string][]string{}
	for _, p := range problems {
		got[p.Request] = append(got[p.Request], p.Err.Error())
	}
	if len(got["Fine"]) != 0 {
		t.Errorf("Verify reported a request the session supports: %v", got["Fine"])
	}
	// The contacts capability is missing, and the account for it is not
	// reported again on top of that.
	if len(got["FindPeople"]) != 1 || !strings.Contains(got["FindPeople"][0], CapabilityContacts) {
		t.Errorf("FindPeople: %v, want the missing contacts capability alone", got["FindPeople"])
	}
	if len(got["TooManyCalls"]) != 1 || !strings.Contains(got["TooManyCalls"][0], "maxCallsInRequest") && !strings.Contains(got["TooManyCalls"][0], "3 method calls") {
		t.Errorf("TooManyCalls: %v, want the call limit", got["TooManyCalls"])
	}
	if len(got["NoPrimaryAccount"]) != 1 || !strings.Contains(got["NoPrimaryAccount"][0], "no primary account") {
		t.Errorf("NoPrimaryAccount: %v, want the missing primary account", got["NoPrimaryAccount"])
	}
	if !strings.Contains(err.Error(), "FindPeople") || !strings.Contains(err.Error(), "TooManyCalls") {
		t.Errorf("the error does not name the requests: %v", err)
	}
	// The problems are of the kind sending the request would fail with.
	if !HasErrorType(err, ErrTypeUnknownCapability) || !HasErrorType(err, ErrTypeLimit) {
		t.Errorf("HasErrorType does not find the capability and the limit in %v", err)
	}
	if IsTemporary(err) {
		t.Errorf("IsTemporary(%v) = true, want false: the session says the same until the server changes", err)
	}
}

// TestVerifyReadsWhatEachAccountSupports covers a primary account that exists
// but does not offer the capability it is the primary account for, which the
// session says through accountCapabilities.
func TestVerifyReadsWhatEachAccountSupports(t *testing.T) {
	ts := newTestServer(t)
	ts.sessionHandler = `{
	  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}, "urn:ietf:params:jmap:submission": {}},
	  "accounts": {"a1": {"name": "someone", "isPersonal": true, "accountCapabilities": {"urn:ietf:params:jmap:mail": {}}}},
	  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1", "urn:ietf:params:jmap:submission": "a1"},
	  "username": "someone",
	  "apiUrl": "/api",
	  "state": "s1"
	}`
	err := ts.client().Verify(context.Background(),
		RequestNeeds{Name: "ReadMail", Using: []string{CapabilityCore, CapabilityMail}, Calls: 1, PrimaryAccounts: []string{CapabilityMail}},
		RequestNeeds{Name: "SendEmail", Using: []string{CapabilityCore, CapabilitySubmission}, Calls: 1, PrimaryAccounts: []string{CapabilitySubmission}},
	)
	problems := verifyProblems(t, err)
	if len(problems) != 1 || problems[0].Request != "SendEmail" || !strings.Contains(problems[0].Err.Error(), "does not support") {
		t.Errorf("Verify reported %v, want SendEmail's account alone", err)
	}
	// Sending it, the server would answer the call with this method error, and
	// the capability itself is there.
	if !HasErrorType(err, ErrAccountNotSupport) || HasErrorType(err, ErrTypeUnknownCapability) {
		t.Errorf("Verify reported %v, want accountNotSupportedByMethod and no unknownCapability", err)
	}
}

// TestVerifyReportsAMissingPrimaryAccountAsSendingWould checks that a primary
// account the session does not name is reported with the error the generated
// function fails with, not as a capability the server lacks: the server has
// the capability.
func TestVerifyReportsAMissingPrimaryAccountAsSendingWould(t *testing.T) {
	ts := newTestServer(t)
	c := ts.client()
	err := c.Verify(context.Background(), RequestNeeds{Name: "ReadCore", Using: []string{CapabilityCore}, Calls: 1, PrimaryAccounts: []string{CapabilityCore}})
	problems := verifyProblems(t, err)
	if len(problems) != 1 {
		t.Fatalf("Verify reported %v, want one problem", err)
	}
	s, _ := c.Session(context.Background())
	_, want := s.PrimaryAccountID(CapabilityCore)
	if want == nil || problems[0].Err.Error() != want.Error() {
		t.Errorf("Verify reported %v, want the error PrimaryAccountID returns: %v", problems[0].Err, want)
	}
	if HasErrorType(err, ErrTypeUnknownCapability) {
		t.Errorf("Verify reported %v as an unknown capability, which the server has", err)
	}
}

func TestVerifyReportsASessionItCannotFetch(t *testing.T) {
	ts := newTestServer(t)
	c := New(ts.URL + "/nowhere")
	err := c.Verify(context.Background(), RequestNeeds{Name: "Anything", Using: []string{CapabilityCore}, Calls: 1})
	if err == nil {
		t.Fatal("Verify passed without a session to check against")
	}
	var v *VerifyError
	if errors.As(err, &v) {
		t.Errorf("Verify reported %v against a request, want the session failure", err)
	}
}
