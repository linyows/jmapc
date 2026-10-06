package shared

import (
	"fmt"
	"sort"

	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// SetProperties returns the properties the record type of a set holds: its own,
// with the id first where it is the set its type starts from and the type has
// an id, since a /get returns the id whatever it is asked for. A set extending
// another holds the id through the one it extends.
func SetProperties(set *request.PropertySet, dataType *spec.Object) []string {
	if set.Extends != nil {
		return set.Own
	}
	if _, hasID := dataType.Field("id"); !hasID {
		return set.Own
	}
	return RecordProperties(set.Own, true)
}

// SetDoc returns the documentation of a set's type, named as the language names
// it: what the project wrote of it, and what a call asking for it receives.
func SetDoc(set *request.PropertySet, typeName string) string {
	asked := fmt.Sprintf("%s holds the properties of %s that a call asking for @%s receives.",
		typeName, set.Type, set.Name)
	if set.Doc == "" {
		return asked
	}
	return set.Doc + "\n\n" + asked
}

// SetsUsed returns the names of the sets a request's calls ask for, written as
// the language writes a type's name, sorted and each once.
func SetsUsed(q *request.Request, typeName func(string) string) []string {
	found := map[string]bool{}
	for _, c := range q.Calls {
		if c.PropertySet != nil {
			found[typeName(c.PropertySet.Name)] = true
		}
		if c.NestedPropertySet != nil {
			found[typeName(c.NestedPropertySet.Name)] = true
		}
	}
	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
