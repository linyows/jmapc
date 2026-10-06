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
	// Each call is refused, as the server refuses each, rather than the first
	// alone: a stub answering the second would let the client think it went.
	if errs[1] == nil {
		t.Error("the second call passed, want it refused as the first was")
	}
	declared := append(append([]string{}, mailUsing...), "urn:ietf:params:jmap:smimeverify")
	if errs := sent(t, declared, `["Email/get", {"ids": ["e1"], "properties": ["smimeStatus"]}, "c0"]`); errs[0] != nil {
		t.Errorf("with the capability declared: %v", errs[0])
	}
}

// TestASentPatchIsHeldToWhatItReaches checks a patch in a request that has
// been sent: a property it reaches needs its capability as one selected by
// name does, and an id it takes as a key has to be one.
func TestASentPatchIsHeldToWhatItReaches(t *testing.T) {
	errs := sent(t, mailUsing,
		`["Email/set", {"update": {"e1": {"smimeStatus": "signed"}}}, "c0"]`,
		`["Email/set", {"update": {"e1": {"mailboxIds/{{mailboxId}}": true}}}, "c1"]`,
		`["Email/set", {"update": {"e1": {"mailboxIds/m1": true}}}, "c2"]`,
	)
	if errs[0] == nil || !strings.Contains(errs[0].Error(), "smimeverify") {
		t.Errorf("a patch to smimeStatus: %v, want the undeclared capability reported", errs[0])
	}
	if errs[1] == nil || !strings.Contains(errs[1].Error(), "is not a valid id") {
		t.Errorf("a patch keyed by braces: %v, want the id refused", errs[1])
	}
	if errs[2] != nil {
		t.Errorf("a patch keyed by an id: %v", errs[2])
	}
}

// TestASentPatchNeedsTheCapabilityOfWhatItPassesThrough checks a patch reaching
// into a property another specification adds: the property's capability is
// needed as it would be to replace the property whole, though the property
// inside it needs none of its own.
func TestASentPatchNeedsTheCapabilityOfWhatItPassesThrough(t *testing.T) {
	s := spec.Standard()
	if err := s.Extend(&spec.Schema{
		Capability: "urn:example:notes",
		Types: []*spec.SchemaType{
			{Name: "Extra", Capability: "urn:example:notes", Properties: []*spec.SchemaField{{Name: "label", Type: "String"}}},
			{Name: "Note", Methods: []string{"set"}, Properties: []*spec.SchemaField{
				{Name: "id", Type: "Id", ServerSet: true},
				{Name: "extra", Type: "Extra", Capability: "urn:example:extras"},
			}},
		},
	}); err != nil {
		t.Fatalf("Extend: %v", err)
	}
	check := NewRequestCheck(s, []string{"urn:ietf:params:jmap:core", "urn:example:notes"})
	err := check.Call(json.RawMessage(`["Note/set", {"update": {"n1": {"extra/label": "x"}}}, "c0"]`), 0)
	if err == nil || !strings.Contains(err.Error(), "urn:example:extras") {
		t.Errorf("a patch through extra: %v, want urn:example:extras reported", err)
	}
}

// TestAUnionAlternativeThatFailedNeedsNoCapability checks a value of a union
// whose first alternative uses a property another capability adds and does not
// fit, and whose second fits and needs nothing: the capability of the
// alternative the value is not is not one the request needs.
func TestAUnionAlternativeThatFailedNeedsNoCapability(t *testing.T) {
	s := spec.Standard()
	if err := s.Extend(&spec.Schema{
		Capability: "urn:example:picks",
		Types: []*spec.SchemaType{
			{Name: "TextChoice", Properties: []*spec.SchemaField{
				{Name: "value", Type: "String", Required: true, Capability: "urn:example:text"},
			}},
			{Name: "NumberChoice", Properties: []*spec.SchemaField{
				{Name: "value", Type: "UnsignedInt", Required: true},
			}},
		},
		Methods: []*spec.SchemaMethod{{
			Name:      "Choice/pick",
			Arguments: []*spec.SchemaField{{Name: "choice", Type: "TextChoice|NumberChoice"}},
			Response:  []*spec.SchemaField{{Name: "ok", Type: "Boolean"}},
		}},
	}); err != nil {
		t.Fatalf("Extend: %v", err)
	}
	check := NewRequestCheck(s, []string{"urn:ietf:params:jmap:core", "urn:example:picks"})
	if err := check.Call(json.RawMessage(`["Choice/pick", {"choice": {"value": 1}}, "c0"]`), 0); err != nil {
		t.Errorf("a number, which the alternative needing nothing takes: %v", err)
	}
	if err := check.Call(json.RawMessage(`["Choice/pick", {"choice": {"value": "a"}}, "c1"]`), 1); err == nil ||
		!strings.Contains(err.Error(), "urn:example:text") {
		t.Errorf("a string, which the alternative needing urn:example:text takes: %v, want it reported", err)
	}
}
