package request

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// sent checks the calls of a request that has been sent, in order, and returns
// what the check reported about each.
func sent(t *testing.T, using []string, calls ...string) []error {
	t.Helper()
	check := NewRequestCheck(spec.Standard(), using)
	errs := make([]error, len(calls))
	for i, call := range calls {
		errs[i] = check.Call(json.RawMessage(call), i)
	}
	return errs
}

var mailUsing = []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"}

// TestASentRequestHoldsValuesRatherThanParameters checks that braces in a
// request that has been sent are a string like any other. A request file
// leaves values open with them; a sent request has nothing left open.
func TestASentRequestHoldsValuesRatherThanParameters(t *testing.T) {
	errs := sent(t, mailUsing,
		`["Mailbox/get", {"ids": "{{x}}"}, "c0"]`,
		`["Mailbox/set", {"create": {"m": {"name": "{{name}}"}}}, "c1"]`,
		`["Mailbox/get", {"ids": null, "properties": "@Summary"}, "c2"]`,
	)
	if errs[0] == nil || !strings.Contains(errs[0].Error(), "Id[]") {
		t.Errorf(`ids of "{{x}}": %v, want it refused as a string where ids belong`, errs[0])
	}
	if errs[1] != nil {
		t.Errorf(`a name of "{{name}}": %v, want it taken as the name it is`, errs[1])
	}
	if errs[2] == nil {
		t.Error(`properties of "@Summary" passed, want a string refused where a list belongs`)
	}
}

// TestASentRequestDeclaresTheCapabilitiesOfItsProperties checks a property a
// specification other than the method's own adds. The server refuses the
// request that uses one without declaring it, as it refuses a method.
func TestASentRequestDeclaresTheCapabilitiesOfItsProperties(t *testing.T) {
	errs := sent(t, mailUsing,
		`["Email/get", {"ids": ["e1"], "properties": ["smimeStatus"]}, "c0"]`,
		`["Email/get", {"ids": ["e2"], "properties": ["smimeStatus"]}, "c1"]`,
	)
	if errs[0] == nil || !strings.Contains(errs[0].Error(), "smimeverify") {
		t.Errorf("Email/get of smimeStatus: %v, want the undeclared capability reported", errs[0])
	}
	if errs[1] != nil {
		t.Errorf("the second call: %v, want the capability reported once", errs[1])
	}
	declared := append(append([]string{}, mailUsing...), "urn:ietf:params:jmap:smimeverify")
	if errs := sent(t, declared, `["Email/get", {"ids": ["e1"], "properties": ["smimeStatus"]}, "c0"]`); errs[0] != nil {
		t.Errorf("with the capability declared: %v", errs[0])
	}
}
