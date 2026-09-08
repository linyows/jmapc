package jmapc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestErrorsAsReachesSetError(t *testing.T) {
	setErrs := &SetErrors{}
	setErrs.Collect("Email/set", "s0", map[string]map[ID]SetError{
		"notDestroyed": {
			ID("m1"): {Type: "notFound"},
		},
	})
	var err error = setErrs.Err()
	if err == nil {
		t.Fatal("Err() = nil, want an error")
	}

	var se *SetError
	if !errors.As(err, &se) {
		t.Fatal("errors.As(err, &se) = false, want true")
	}
	if se.Type != "notFound" {
		t.Errorf("se.Type = %q, want %q", se.Type, "notFound")
	}
}

func TestSetFailureUnwrap(t *testing.T) {
	f := SetFailure{
		Method: "Email/set",
		CallID: "s0",
		Kind:   "notCreated",
		Key:    ID("c1"),
		Err:    SetError{Type: "invalidProperties", Properties: []string{"mailboxIds"}},
	}
	unwrapped := f.Unwrap()
	se, ok := unwrapped.(*SetError)
	if !ok {
		t.Fatalf("Unwrap() returned %T, want *SetError", unwrapped)
	}
	if se.Type != "invalidProperties" {
		t.Errorf("se.Type = %q, want %q", se.Type, "invalidProperties")
	}
}

func TestIsTemporary(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nothing failed", nil, false},
		{"the server was unavailable", &RequestError{Status: 503}, true},
		{"the server failed", &RequestError{Status: 500}, true},
		{"the server asked for fewer requests", &RequestError{Status: 429}, true},
		{"the request was refused", &RequestError{Status: 400}, false},
		{"the token was refused", &RequestError{Status: 401}, false},
		{"the request was refused before it was sent", &RequestError{Type: ErrTypeUnknownCapability}, false},
		{"a call met the server's own trouble", &MethodError{Type: ErrServerFail}, true},
		{"a call was rate limited", &MethodError{Type: ErrRateLimit}, true},
		{"a call was wrong", &MethodError{Type: ErrInvalidArguments}, false},
		{"a call named a type the server does not define", &MethodError{Type: "vendorSaysNo"}, false},
		{
			"one call of several was wrong",
			MethodErrors{{Type: ErrServerFail}, {Type: ErrInvalidArguments}},
			false,
		},
		{
			"every call met the server's own trouble",
			MethodErrors{{Type: ErrServerFail}, {Type: ErrServerUnavailable}},
			true,
		},
		{
			"a record was rate limited",
			&SetErrors{Failures: []SetFailure{{Err: SetError{Type: ErrRateLimit}}}},
			true,
		},
		{
			"a record was wrong",
			&SetErrors{Failures: []SetFailure{{Err: SetError{Type: ErrInvalidProperties}}}},
			false,
		},
		{"the request never reached the server", errors.New("dial tcp: connection refused"), true},
		{"a refusal wrapped in context", fmt.Errorf("reading the inbox: %w", &RequestError{Status: 400}), false},
		{"trouble wrapped in context", fmt.Errorf("reading the inbox: %w", &RequestError{Status: 503}), true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTemporary(tt.err); got != tt.want {
				t.Errorf("IsTemporary(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestHasErrorType(t *testing.T) {
	overQuotaRecord := &SetErrors{Failures: []SetFailure{
		{Key: "m1", Err: SetError{Type: ErrInvalidProperties}},
		{Key: "m2", Err: SetError{Type: ErrOverQuota}},
	}}

	cases := []struct {
		name string
		err  error
		typ  string
		want bool
	}{
		{"nothing failed", nil, ErrOverQuota, false},
		{"no type asked about", &MethodError{Type: ErrOverQuota}, "", false},
		{"the call was refused for that reason", &MethodError{Type: ErrOverQuota}, ErrOverQuota, true},
		{"the call was refused for another", &MethodError{Type: ErrInvalidArguments}, ErrOverQuota, false},
		{"one call of several", MethodErrors{{Type: ErrServerFail}, {Type: ErrOverQuota}}, ErrOverQuota, true},
		{"one record of several", overQuotaRecord, ErrOverQuota, true},
		{"a record refused for another reason", overQuotaRecord, ErrNotFound, false},
		{"one record on its own", &SetError{Type: ErrNotFound}, ErrNotFound, true},
		{"the whole request was refused", &RequestError{Status: 400, Type: ErrTypeLimit}, ErrTypeLimit, true},
		{"the request was refused for another", &RequestError{Status: 400, Type: ErrTypeNotJSON}, ErrTypeLimit, false},
		{"a status with no type", &RequestError{Status: 503}, ErrTypeLimit, false},
		{"a type the server defines itself", &MethodError{Type: "vendorSaysNo"}, "vendorSaysNo", true},
		{
			"a refusal wrapped in context",
			fmt.Errorf("filing the message: %w", &MethodError{Type: ErrOverQuota}),
			ErrOverQuota,
			true,
		},
		{"a failure that carries no type", errors.New("dial tcp: connection refused"), ErrOverQuota, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasErrorType(tt.err, tt.typ); got != tt.want {
				t.Errorf("HasErrorType(%v, %q) = %v, want %v", tt.err, tt.typ, got, tt.want)
			}
		})
	}
}

// TestHasErrorTypeReadsEveryLevel checks the case the function is for: one
// condition reported at the level the server happened to refuse at, which the
// caller answers the same way whichever level that was.
func TestHasErrorTypeReadsEveryLevel(t *testing.T) {
	atEveryLevel := []error{
		&RequestError{Status: 400, Type: ErrTypeLimit},
		&MethodError{MethodName: "Email/set", Type: ErrOverQuota},
		&SetErrors{Failures: []SetFailure{{Key: "m1", Err: SetError{Type: ErrOverQuota}}}},
	}
	full := func(err error) bool {
		return HasErrorType(err, ErrOverQuota) || HasErrorType(err, ErrTypeLimit)
	}
	for _, err := range atEveryLevel {
		if !full(err) {
			t.Errorf("%T was not recognised: %v", err, err)
		}
	}
}

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nothing failed", nil, false},
		{"too many requests", &RequestError{Status: 429}, true},
		{
			"one request too many at that moment",
			&RequestError{Status: 400, Type: ErrTypeLimit, Limit: "maxConcurrentRequests"},
			true,
		},
		{
			"one upload too many at that moment",
			&RequestError{Status: 400, Type: ErrTypeLimit, Limit: "maxConcurrentUpload"},
			true,
		},
		{
			"a request holding too many calls",
			&RequestError{Status: 400, Type: ErrTypeLimit, Limit: "maxCallsInRequest"},
			false,
		},
		{"the server was unavailable", &RequestError{Status: 503}, false},
		{"a call was rate limited", &MethodError{Type: ErrRateLimit}, true},
		{"a record was rate limited", &SetErrors{Failures: []SetFailure{{Err: SetError{Type: ErrRateLimit}}}}, true},
		{"a call was wrong", &MethodError{Type: ErrInvalidArguments}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRateLimited(tt.err); got != tt.want {
				t.Errorf("IsRateLimited(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryAfterOfAnError(t *testing.T) {
	if d, ok := RetryAfter(&RequestError{Status: 429, RetryAfter: 90 * time.Second}); !ok || d != 90*time.Second {
		t.Errorf("RetryAfter = %v, %v, want 1m30s and true", d, ok)
	}
	if d, ok := RetryAfter(&RequestError{Status: 429}); ok || d != 0 {
		t.Errorf("RetryAfter = %v, %v, want nothing where the server asked for no delay", d, ok)
	}
	if _, ok := RetryAfter(errors.New("dial tcp: connection refused")); ok {
		t.Error("RetryAfter reported a delay for an error that carries none")
	}
}

// TestAnErrorCarriesTheDelayItWasRefusedFor covers the delay the client will
// not wait out. It stops and reports the refusal, and the caller deciding when
// to come back needs the number the server gave, which used to be read for the
// retry and then dropped.
func TestAnErrorCarriesTheDelayItWasRefusedFor(t *testing.T) {
	c, _, _ := answering(t, answer{status: http.StatusTooManyRequests, after: "3600"})
	_, err := c.Do(context.Background(), echo())
	if err == nil {
		t.Fatal("the request answered nothing, want the refusal")
	}
	d, ok := RetryAfter(err)
	if !ok || d != time.Hour {
		t.Errorf("RetryAfter = %v, %v, want an hour", d, ok)
	}
	if !IsTemporary(err) {
		t.Error("a server asking for fewer requests was reported as a failure that time does not resolve")
	}
	if !IsRateLimited(err) {
		t.Error("a 429 was not reported as the server asking for fewer requests")
	}
}
