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
func (s *paramSet) use(c *checker, name string, t *spec.Type, where, doc string) *Param {
	if p, seen := s.byName[name]; seen {
		if !sameType(p.Type, t) {
			c.errorf(where, "use a different parameter name for the second value",
				"parameter %q is %s where it is first used, at %s, but %s here",
				name, p.Type, p.Where, t)
		}
		return p
	}
	p := &Param{Name: name, Type: t, Where: where, Doc: doc}
	s.byName[name] = p
	s.order = append(s.order, p)
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

// assignGoNames settles the Go identifiers the generated code will use, making
// each one unique within the query.
func assignGoNames(q *Query) {
	taken := make(map[string]bool)
	for _, p := range q.Params {
		p.GoName = unique(taken, spec.ExportedName(p.Name))
	}
	fields := make(map[string]bool)
	for _, call := range q.Calls {
		call.GoField = unique(fields, call.Method.GoName())
	}
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
