package query

import (
	"strconv"

	"github.com/linyows/jmapc/internal/spec"
)

// paramSet collects the parameters a query leaves open, in the order they first
// appear, and holds each one to a single type.
type paramSet struct {
	order  []*Param
	byName map[string]*Param
}

func newParamSet() *paramSet {
	return &paramSet{byName: make(map[string]*Param)}
}

// mark records the current size of the set, for rollback.
func (s *paramSet) mark() int { return len(s.order) }

// rollback discards every parameter added since the given mark, so that an
// alternative a union tried and rejected leaves nothing behind.
func (s *paramSet) rollback(mark int) {
	for _, p := range s.order[mark:] {
		delete(s.byName, p.Name)
	}
	s.order = s.order[:mark]
}

// use records a use of a parameter and returns it, creating it on first sight.
// A parameter used in two places must mean the same type in both, because the
// caller supplies it once.
//
// A weak use is one whose surroundings do not say what the parameter is, such as
// a name embedded in a JSON pointer. It claims a type only until a use that does
// know comes along.
func (s *paramSet) use(c *checker, name string, t *spec.Type, where, doc string, weak bool) *Param {
	p, seen := s.byName[name]
	if !seen {
		p = &Param{Name: name, Type: t, Where: where, Doc: doc, Weak: weak}
		p.addPlace(where)
		s.byName[name] = p
		s.order = append(s.order, p)
		return p
	}
	p.addPlace(where)
	switch {
	case weak:
		// The use says nothing the parameter does not already know.
	case p.Weak:
		p.Type, p.Where, p.Doc, p.Weak = t, where, doc, false
	case !sameType(p.Type, t):
		c.errorf(where, "use a different parameter name for the second value",
			"parameter %q is %s where it is first used, at %s, but %s here",
			name, p.Type, p.Where, t)
	}
	return p
}

// all returns the parameters in the order they first appeared.
func (s *paramSet) all() []*Param {
	return s.order
}

// sameType reports whether two uses of a parameter agree on its type. Whether
// the argument accepts null does not matter, because a parameter always carries
// a value.
func sameType(a, b *spec.Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	x.Nullable, y.Nullable = false, false
	return x.String() == y.String()
}

// assignFieldNames settles the name each parameter and each call's result goes
// by, making them unique within the query. They are given here in the shape Go
// uses, since that is a shape every generator can spell: a language that writes
// its identifiers differently lowers the first letter or splits on the case,
// and either way the uniqueness holds.
func assignFieldNames(q *Query) {
	taken := make(map[string]bool)
	for _, p := range q.Params {
		p.Field = unique(taken, spec.ExportedName(p.Name))
	}
	fields := make(map[string]bool)
	for _, call := range q.Calls {
		call.Field = unique(fields, callFieldName(call))
	}
}

// callFieldName is the name a call goes by in the generated code: the id the
// query gave it, which is unique within a request by definition and stays put
// when a call is inserted ahead of it. Numbering by position instead would
// mean a name that pointed at one call yesterday pointing at another today.
//
// A call id is any string RFC 8620 allows, so one that does not make an
// identifier falls back to the method it invokes, which is what every call
// went by before.
func callFieldName(call *Call) string {
	if name := spec.ExportedName(call.ID); isGoIdentifier(name) {
		return name
	}
	return call.Method.TypeNamePrefix()
}

// unique returns name, or name with a number appended, so that it has not been
// used before.
func unique(taken map[string]bool, name string) string {
	candidate := name
	for i := 2; taken[candidate]; i++ {
		candidate = name + strconv.Itoa(i)
	}
	taken[candidate] = true
	return candidate
}
