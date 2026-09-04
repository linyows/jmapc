package jmapc

import (
	"errors"
	"testing"
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
