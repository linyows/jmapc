package shared

import (
	"sort"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// CallPlan is what a generator writes for one method call of a request: the
// types its response decodes into, and the variable holding the account it
// runs against where the request leaves that to the session.
type CallPlan struct {
	// ResponseType is the type the call's response decodes into.
	ResponseType string
	// RecordType names the generated record type, empty where the call fetches
	// whole records and the runtime's type is used.
	RecordType string
	// NestedType names the generated type for the records' nested type, as
	// bodyProperties gives an Email's body parts, empty where that is not
	// narrowed either.
	NestedType string
	// AccountIDVar is the variable holding the accountId the request left out,
	// empty where the request gives one.
	AccountIDVar string
	// WritesTypes says this call is the one that writes the types it names. A
	// call reading the same records in the same shape as an earlier one shares
	// its types rather than declaring them again.
	WritesTypes bool
	// SharedRecord says the record type is one of the named sets, which is
	// written once for the package rather than by the request that asks for it.
	SharedRecord bool
	// SharedNested says the same of the nested type.
	SharedNested bool
}

// Namer is how a language writes the names PlanCalls makes. Every name a
// request's types take is the request's prefix followed by parts; the parts
// are written as the language writes a type's name, and the runtime's own
// types as the language refers to them.
type Namer struct {
	// Prefix begins every name generated for the request.
	Prefix string
	// Field writes a call's field name, which the request has already made
	// an exported name, as part of a generated type's name.
	Field func(string) string
	// Part writes a data type's name as part of a generated type's name.
	Part func(string) string
	// Set writes the name of a set of properties as the type it is.
	Set func(string) string
	// Runtime writes the name of a type of the runtime's, such as the
	// response of a method, as a reference to it.
	Runtime func(string) string
	// AccountID names the variable holding a capability's primary account.
	AccountID func(capability string) string
}

// PlanCalls settles the types each call of q decodes into, taking each name it
// makes from taken so that none is made twice, and returns them with the
// capabilities whose primary account the request looks up, in a stable order.
// The names are made in the order of the calls, which is what keeps them the
// same from one run to the next.
func PlanCalls(s *spec.Spec, q *request.Request, taken map[string]bool, n Namer) (map[*request.Call]*CallPlan, []string) {
	calls := make(map[*request.Call]*CallPlan, len(q.Calls))
	same := SameNarrowing(q.Calls)
	for _, c := range q.Calls {
		plan := &CallPlan{}
		// Narrowing the nested type means the record type has to be
		// generated too, since its own fields change to refer to it.
		switch {
		case same[c] != nil && same[c] != c:
			// Another call of this request reads the same records in the
			// same shape, and one shape is one type.
			*plan = *calls[same[c]]
			plan.WritesTypes = false
		case c.PropertySet != nil:
			// The set names the type, so every call asking for it answers
			// with the one the package declares.
			plan.RecordType = n.Set(c.PropertySet.Name)
			plan.ResponseType = Unique(taken, n.Prefix+n.Field(c.Field)+"Response")
			plan.WritesTypes = true
			plan.SharedRecord = true
		case c.Properties != nil || c.NestedProperties != nil || OpenRecord(c):
			plan.RecordType = Unique(taken, n.Prefix+n.Field(c.Field)+n.Part(c.Method.DataType))
			plan.ResponseType = Unique(taken, n.Prefix+n.Field(c.Field)+"Response")
			plan.WritesTypes = true
		default:
			plan.ResponseType = n.Runtime(c.Method.Response)
		}
		switch {
		case c.NestedPropertySet != nil:
			plan.NestedType = n.Set(c.NestedPropertySet.Name)
			plan.SharedNested = true
		case c.NestedProperties != nil && plan.WritesTypes:
			plan.NestedType = Unique(taken, n.Prefix+n.Field(c.Field)+n.Part(c.Method.NestedType))
		}
		calls[c] = plan
	}

	var capabilities []string
	seen := make(map[string]bool)
	for _, c := range q.Calls {
		capability, needed := c.AccountIDCapability(s)
		if !needed {
			continue
		}
		if !seen[capability] {
			seen[capability] = true
			capabilities = append(capabilities, capability)
		}
		calls[c].AccountIDVar = n.AccountID(capability)
	}
	sort.Strings(capabilities)
	return calls, capabilities
}
