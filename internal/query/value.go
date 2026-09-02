package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/spec"
)

// paramPattern matches a value the query leaves open. The braces are used
// rather than a "$" prefix because JMAP keywords are themselves written with a
// leading "$", as in "$seen" and "$flagged".
var paramPattern = regexp.MustCompile(`^\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}$`)

// value checks one JSON value against the type the JMAP data model says it must
// have, and returns the node the generator will emit for it.
func (c *checker) value(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	raw = json.RawMessage(bytes.TrimSpace(raw))

	if s, isString := stringValue(raw); isString {
		if m := paramPattern.FindStringSubmatch(s); m != nil {
			return &ParamRef{Param: c.params.use(c, m[1], t, where, doc)}
		}
		if strings.Contains(s, "{{") {
			c.errorf(where, "write the whole value as {{name}}",
				"a parameter cannot be embedded in a larger string")
			return &Literal{JSON: raw}
		}
	}

	if isNull(raw) {
		if !t.Nullable && t.Name != spec.Any {
			c.errorf(where, "", "%s may not be null", t)
		}
		return &Literal{JSON: raw}
	}

	switch {
	case t.Name == spec.Any:
		return c.anyValue(raw)
	case t.IsUnion():
		return c.union(t, raw, where, doc)
	case t.IsArray():
		return c.array(t, raw, where, doc)
	case t.IsMap():
		return c.mapValue(t, raw, where, doc)
	case t.IsObject():
		return c.object(t, raw, where)
	default:
		return c.primitive(t, raw, where)
	}
}

// union checks a value that may take either of several shapes, which is how
// JMAP spells a /query filter. It reports the failure of the closest-fitting
// alternative rather than of all of them, because a filter that is nearly a
// valid condition is more usefully described as that condition with one thing
// wrong.
func (c *checker) union(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	saved := c.filterUnion
	if unionMentions(t, "FilterOperator") {
		c.filterUnion = t
	}
	defer func() { c.filterUnion = saved }()

	var best Node
	var bestErrs ErrorList
	for _, member := range c.rankUnion(t, raw) {
		node, errs := c.try(func() Node { return c.value(member, raw, where, doc) })
		if len(errs) == 0 {
			return node
		}
		if best == nil || len(errs) < len(bestErrs) {
			best, bestErrs = node, errs
		}
	}
	c.errs = append(c.errs, bestErrs...)
	return best
}

// rankUnion orders the alternatives of a union by how well each one fits the
// value at hand, so that a value that is nearly one shape is reported as that
// shape with something wrong, rather than as the wrong shape entirely. Without
// this, a filter condition with one bad property would be reported as a
// FilterOperator missing its operator.
func (c *checker) rankUnion(t *spec.Type, raw json.RawMessage) []*spec.Type {
	members := append([]*spec.Type(nil), t.Union...)
	keys, err := objectKeys(raw)
	if err != nil || len(keys) == 0 {
		return members
	}
	score := make(map[*spec.Type]int, len(members))
	for _, m := range members {
		o, ok := c.spec.Object(m.Name)
		if !ok {
			continue
		}
		for _, key := range keys {
			if _, known := o.Field(key); known {
				score[m]++
			}
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		return score[members[i]] > score[members[j]]
	})
	return members
}

// try runs f and, if it reports any problem, undoes everything f recorded and
// returns the problems instead. It lets a union try each alternative without
// leaving the parameters or errors of a failed attempt behind.
func (c *checker) try(f func() Node) (Node, ErrorList) {
	errMark, paramMark := len(c.errs), c.params.mark()
	node := f()
	if len(c.errs) > errMark {
		errs := append(ErrorList(nil), c.errs[errMark:]...)
		c.errs = c.errs[:errMark]
		c.params.rollback(paramMark)
		return node, errs
	}
	return node, nil
}

// array checks a JSON array against an array type.
func (c *checker) array(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return &Literal{JSON: raw}
	}
	out := &Array{Items: make([]Node, 0, len(items)), Raw: raw}
	for i, item := range items {
		out.Items = append(out.Items, c.value(t.Elem, item, fmt.Sprintf("%s[%d]", where, i), doc))
	}
	return out
}

// mapValue checks a JSON object against a map type, such as the mailboxIds of
// an Email or the create map of a /set call.
func (c *checker) mapValue(t *spec.Type, raw json.RawMessage, where, doc string) Node {
	members, keys, ok := c.objectMembers(t, raw, where)
	if !ok {
		return &Literal{JSON: raw}
	}
	out := &Object{Raw: raw}
	for _, key := range keys {
		field := ObjectField{Key: key}
		if m := paramPattern.FindStringSubmatch(key); m != nil {
			// The member name itself is left open, which is how a /set call
			// updates or destroys whichever record the caller names.
			keyType := t.Key
			if keyType == nil {
				keyType = &spec.Type{Name: spec.String}
			}
			field.KeyParam = c.params.use(c, m[1], keyType, where+"."+key, keyDoc(keyType))
		} else if t.Key != nil && t.Key.Name == spec.IdType && !isCreationID(key) && !jmapc.ID(key).Valid() {
			c.errorf(where+"."+key, "", "%q is not a valid id", key)
		}
		field.Value = c.value(t.Value, members[key], where+"."+key, doc)
		out.Fields = append(out.Fields, field)
	}
	return out
}

// object checks a JSON object against a named object type.
func (c *checker) object(t *spec.Type, raw json.RawMessage, where string) Node {
	o, known := c.spec.Object(t.Name)
	if !known {
		c.errorf(where, "", "unknown type %q in the data model", t.Name)
		return &Literal{JSON: raw}
	}
	members, keys, ok := c.objectMembers(t, raw, where)
	if !ok {
		return &Literal{JSON: raw}
	}

	// A PatchObject is keyed by JSON pointer rather than by property name, so
	// its members cannot be checked against a fixed set.
	if len(o.Fields) == 0 {
		out := &Object{Raw: raw}
		for _, key := range keys {
			out.Fields = append(out.Fields, ObjectField{
				Key:   key,
				Value: c.anyValue(members[key]),
			})
		}
		return out
	}

	out := &Object{Raw: raw}
	for _, key := range keys {
		field, isKnown := o.Field(key)
		if !isKnown {
			c.errorf(where+"."+key, hintFor(key, o.PropertyNames()), "%s has no property %q", o.Name, key)
			continue
		}
		elemType := field.ParsedType()
		if o.Name == "FilterOperator" && key == "conditions" && c.filterUnion != nil {
			// The conditions of a filter operator are filters of the same kind
			// as the one being built, which the data model can only describe
			// as Any.
			elemType = &spec.Type{Elem: c.filterUnion}
		}
		out.Fields = append(out.Fields, ObjectField{
			Key:   key,
			Value: c.value(elemType, members[key], where+"."+key, field.Doc),
		})
	}
	return out
}

// objectMembers decodes a JSON object, preserving the order its members were
// written in.
func (c *checker) objectMembers(t *spec.Type, raw json.RawMessage, where string) (map[string]json.RawMessage, []string, bool) {
	keys, err := objectKeys(raw)
	if err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return nil, nil, false
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		c.errorf(where, "", "expected %s, found %s", t, jsonKind(raw))
		return nil, nil, false
	}
	return members, keys, true
}

// anyValue accepts a value the data model does not constrain, keeping it as it
// was written.
func (c *checker) anyValue(raw json.RawMessage) Node {
	return &Literal{JSON: raw}
}

// primitive checks a value against one of the primitive JMAP types.
func (c *checker) primitive(t *spec.Type, raw json.RawMessage, where string) Node {
	kind := jsonKind(raw)
	fail := func() Node {
		c.errorf(where, "", "expected %s, found %s", t, kind)
		return &Literal{JSON: raw}
	}

	switch t.Name {
	case spec.String:
		if kind != "a string" {
			return fail()
		}
	case spec.Boolean:
		if kind != "a boolean" {
			return fail()
		}
	case spec.Number:
		if kind != "a number" {
			return fail()
		}
	case spec.Int, spec.UnsignedInt:
		if kind != "a number" {
			return fail()
		}
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return fail()
		}
		i, err := n.Int64()
		if err != nil {
			c.errorf(where, "", "expected %s, found the fractional number %s", t, n)
			return &Literal{JSON: raw}
		}
		if t.Name == spec.UnsignedInt && i < 0 {
			c.errorf(where, "", "expected %s, found the negative number %s", t, n)
			return &Literal{JSON: raw}
		}
		if i > int64(jmapc.MaxInt) || i < int64(jmapc.MinInt) {
			c.errorf(where, "JMAP integers are limited to the range a double can hold exactly",
				"%s is outside the range of %s", n, t)
			return &Literal{JSON: raw}
		}
	case spec.IdType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if !isCreationID(s) && !jmapc.ID(s).Valid() {
			c.errorf(where, "an id is 1 to 255 characters from A-Z, a-z, 0-9, _ and -, and a creation id is written as \"#\" followed by such a name",
				"%q is not a valid id", s)
		}
	case spec.UTCDateType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if _, err := time.Parse("2006-01-02T15:04:05Z", s); err != nil {
			c.errorf(where, "a UTCDate is written as 2006-01-02T15:04:05Z", "%q is not a UTCDate", s)
		}
	case spec.DateType:
		s, ok := stringValue(raw)
		if !ok {
			return fail()
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			c.errorf(where, "a Date is written as 2006-01-02T15:04:05Z07:00", "%q is not a Date", s)
		}
	default:
		c.errorf(where, "", "unknown type %q in the data model", t.Name)
	}
	return &Literal{JSON: raw}
}

// keyDoc describes what a parameter standing in for a map key selects.
func keyDoc(keyType *spec.Type) string {
	if keyType.Name == spec.IdType {
		return "The id of the record this entry applies to."
	}
	return "The key this entry is stored under."
}

// isCreationID reports whether s is a reference to a record created earlier in
// the same request, which RFC 8620, Section 5.3, writes as "#" followed by the
// creation id.
func isCreationID(s string) bool {
	return strings.HasPrefix(s, "#") && jmapc.ID(s[1:]).Valid()
}

// unionMentions reports whether a union type has a member with the given name.
func unionMentions(t *spec.Type, name string) bool {
	for _, m := range t.Union {
		if m.Name == name {
			return true
		}
	}
	return false
}

// assignable reports whether a value of type got may stand in where want is
// expected. It is what decides whether a back reference feeds the argument it
// points at.
func assignable(got, want *spec.Type) bool {
	if got == nil || want == nil {
		return false
	}
	if want.Name == spec.Any || got.Name == spec.Any {
		return true
	}
	if want.IsUnion() {
		for _, m := range want.Union {
			if assignable(got, m) {
				return true
			}
		}
		return false
	}
	if got.IsUnion() {
		for _, m := range got.Union {
			if !assignable(m, want) {
				return false
			}
		}
		return true
	}
	switch {
	case want.IsArray():
		return got.IsArray() && assignable(got.Elem, want.Elem)
	case want.IsMap():
		return got.IsMap() && assignable(got.Key, want.Key) && assignable(got.Value, want.Value)
	default:
		return got.Name == want.Name
	}
}

// objectKeys returns the member names of a JSON object in the order they were
// written, which decoding into a map would lose.
func objectKeys(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("object key is not a string")
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// jsonKind names the kind of a JSON value, for use in error messages.
func jsonKind(raw json.RawMessage) string {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 {
		return "nothing"
	}
	switch raw[0] {
	case '{':
		return "an object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}

// isNull reports whether raw is the JSON null literal.
func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// stringValue decodes raw as a JSON string.
func stringValue(raw json.RawMessage) (string, bool) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) { sort.Strings(s) }
