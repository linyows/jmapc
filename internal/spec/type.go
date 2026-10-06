// Package spec holds the JMAP data model that jmapc generates code against:
// the object types, their properties, and the methods that operate on them.
package spec

import (
	"fmt"
	"strings"
)

// Primitive type names, spelled as the JMAP specifications spell them.
const (
	String      = "String"
	Boolean     = "Boolean"
	Number      = "Number"
	Int         = "Int"
	UnsignedInt = "UnsignedInt"
	IdType      = "Id"
	DateType    = "Date"
	UTCDateType = "UTCDate"
	Any         = "Any"

	// The primitive types JSCalendar adds, RFC 8984, Section 1.4.
	LocalDateTimeType  = "LocalDateTime"
	DurationType       = "Duration"
	SignedDurationType = "SignedDuration"
	TimeZoneIDType     = "TimeZoneId"
)

// primitive records how a primitive JMAP type is spelled in each language a
// client is generated in. In Go, named types live in the jmapc package and so
// need qualifying when the generated code lives elsewhere; the builtins never
// do. Rust and TypeScript declare a type of their own, an alias of a string,
// for each primitive that is a string with a form of its own, and alias says
// what that form is.
type primitive struct {
	goName    string
	qualified bool
	rust      string
	ts        string
	alias     string
}

// primitives maps every primitive type name to the types it becomes.
var primitives = map[string]primitive{
	String:      {goName: "string", rust: "String", ts: "string"},
	Boolean:     {goName: "bool", rust: "bool", ts: "boolean"},
	Number:      {goName: "float64", rust: "f64", ts: "number"},
	Int:         {goName: "Int", qualified: true, rust: "i64", ts: "number"},
	UnsignedInt: {goName: "UnsignedInt", qualified: true, rust: "u64", ts: "number"},
	IdType: {goName: "ID", qualified: true, rust: "Id", ts: "Id",
		alias: "An id assigned by the server: 1 to 255 characters from A-Z, a-z, 0-9, _ and -, not beginning with - or #."},
	DateType: {goName: "Date", qualified: true, rust: "Date", ts: "Date",
		alias: "A date and time with an offset, written as 2006-01-02T15:04:05Z07:00."},
	UTCDateType: {goName: "UTCDate", qualified: true, rust: "UtcDate", ts: "UTCDate",
		alias: "A date and time in UTC, written as 2006-01-02T15:04:05Z."},
	Any: {goName: "any", rust: "serde_json::Value", ts: "unknown"},

	LocalDateTimeType: {goName: "LocalDateTime", qualified: true, rust: "LocalDateTime", ts: "LocalDateTime",
		alias: "A date and time with no zone at all, written as 2006-01-02T15:04:05. What it means depends on the time zone the enclosing object gives."},
	DurationType: {goName: "Duration", qualified: true, rust: "Duration", ts: "Duration",
		alias: "A length of time in the ISO 8601 form, such as PT1H30M or P1D. Not a number of milliseconds: a day is not always 24 hours."},
	SignedDurationType: {goName: "SignedDuration", qualified: true, rust: "SignedDuration", ts: "SignedDuration",
		alias: "A Duration that may be negative, which is how an alert says it fires before the event it belongs to."},
	TimeZoneIDType: {goName: "TimeZoneID", qualified: true, rust: "TimeZoneId", ts: "TimeZoneId",
		alias: "A time zone from the IANA database, such as Europe/London, or a name beginning with / that refers to a zone the event itself defines."},
}

// aliased lists the primitives Rust and TypeScript declare an alias for, in the
// order the aliases are written.
var aliased = []string{IdType, UTCDateType, DateType, LocalDateTimeType, DurationType, SignedDurationType, TimeZoneIDType}

// primitiveAliases returns the aliases a language declares, named as name says
// it names them, each with what it is an alias for.
func primitiveAliases(name func(primitive) string) []struct{ Name, Doc string } {
	out := make([]struct{ Name, Doc string }, len(aliased))
	for i, t := range aliased {
		p := primitives[t]
		out[i] = struct{ Name, Doc string }{name(p), p.alias}
	}
	return out
}

// Type is a parsed JMAP type expression. The surface syntax follows the
// specifications: "Foo[]" is an array, "String[Foo]" is a map, "Foo|null" is
// nullable, and "Foo|Bar" is a union.
type Type struct {
	// Name is the primitive or object type name, empty for arrays, maps, and
	// unions.
	Name string
	// Elem is the element type of an array.
	Elem *Type
	// Key and Value are the key and value types of a map.
	Key, Value *Type
	// Union holds the alternatives of a union type, which JMAP uses where a
	// property may hold either of two shapes.
	Union []*Type
	// Nullable reports whether null is an accepted value.
	Nullable bool
}

// ParseType parses a JMAP type expression.
func ParseType(s string) (*Type, error) {
	t, err := parseUnion(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("parsing type %q: %w", s, err)
	}
	return t, nil
}

// MustParseType is ParseType for type expressions written into the catalogue
// itself, where a syntax error is a bug rather than bad input.
func MustParseType(s string) *Type {
	t, err := ParseType(s)
	if err != nil {
		panic(err)
	}
	return t
}

// parseUnion splits on the top-level "|", peeling off a trailing "null" as a
// nullability marker rather than a union member.
func parseUnion(s string) (*Type, error) {
	parts := splitTopLevel(s, '|')
	var nullable bool
	var members []*Type
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "null" {
			nullable = true
			continue
		}
		t, err := parseSuffix(part)
		if err != nil {
			return nil, err
		}
		members = append(members, t)
	}
	switch len(members) {
	case 0:
		return nil, fmt.Errorf("type expression has no non-null member")
	case 1:
		members[0].Nullable = members[0].Nullable || nullable
		return members[0], nil
	default:
		return &Type{Union: members, Nullable: nullable}, nil
	}
}

// parseSuffix handles the array and map forms, which are written as suffixes on
// a base type name.
func parseSuffix(s string) (*Type, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty type expression")
	}
	if strings.HasSuffix(s, "[]") {
		elem, err := parseUnion(s[:len(s)-2])
		if err != nil {
			return nil, err
		}
		return &Type{Elem: elem}, nil
	}
	if strings.HasSuffix(s, "]") {
		open := matchingOpen(s)
		if open <= 0 {
			return nil, fmt.Errorf("unbalanced brackets in %q", s)
		}
		key, err := parseUnion(s[:open])
		if err != nil {
			return nil, err
		}
		value, err := parseUnion(s[open+1 : len(s)-1])
		if err != nil {
			return nil, err
		}
		return &Type{Key: key, Value: value}, nil
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		// Parentheses group a union or a nullable type so that a suffix can
		// apply to the whole of it: "(Date|null)[]" is an array whose items
		// may each be null, which "Date|null[]" could not say.
		return parseUnion(s[1 : len(s)-1])
	}
	if strings.ContainsAny(s, "[]|()") {
		return nil, fmt.Errorf("malformed type expression %q", s)
	}
	return &Type{Name: s}, nil
}

// matchingOpen returns the index of the "[" that closes at the end of s.
func matchingOpen(s string) int {
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ']', ')':
			depth++
		case '(':
			depth--
		case '[':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits s on sep, ignoring separators nested inside brackets.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// IsArray reports whether t is an array type.
func (t *Type) IsArray() bool { return t != nil && t.Elem != nil }

// IsMap reports whether t is a map type.
func (t *Type) IsMap() bool { return t != nil && t.Key != nil }

// IsUnion reports whether t is a union of more than one shape.
func (t *Type) IsUnion() bool { return t != nil && len(t.Union) > 0 }

// IsPrimitive reports whether t names one of the primitive JMAP types.
func (t *Type) IsPrimitive() bool {
	if t == nil || t.Name == "" {
		return false
	}
	_, ok := primitives[t.Name]
	return ok
}

// IsObject reports whether t names an object type from the catalogue.
func (t *Type) IsObject() bool {
	return t != nil && t.Name != "" && !t.IsPrimitive()
}

// String renders t back into the type expression syntax.
func (t *Type) String() string {
	if t == nil {
		return "<nil>"
	}
	var s string
	switch {
	case t.IsArray():
		elem := t.Elem.String()
		if t.Elem.IsUnion() || t.Elem.Nullable {
			// Without the parentheses the suffix would bind to the last
			// member of the union rather than to the whole of it.
			elem = "(" + elem + ")"
		}
		s = elem + "[]"
	case t.IsMap():
		s = t.Key.String() + "[" + t.Value.String() + "]"
	case t.IsUnion():
		parts := make([]string, len(t.Union))
		for i, m := range t.Union {
			parts[i] = m.String()
		}
		s = strings.Join(parts, "|")
	default:
		s = t.Name
	}
	if t.Nullable {
		s += "|null"
	}
	return s
}

// GoType renders t as a Go type expression. Types defined by the jmapc runtime
// package are prefixed with qualifier, which should be "jmapc." for generated
// code outside that package and empty for code inside it.
//
// Nullability is carried by a pointer, except where the Go representation
// already has a nil form: a null array or map is simply the nil slice or map.
// A union becomes the struct GoUnionName names, one field per shape.
func (t *Type) GoType(qualifier string) string {
	if t == nil {
		return "any"
	}
	var base string
	switch {
	case t.IsArray():
		base = "[]" + t.Elem.GoType(qualifier)
	case t.IsMap():
		base = "map[" + t.Key.GoType(qualifier) + "]" + t.Value.GoType(qualifier)
	case t.IsUnion():
		base = qualifier + GoUnionName(t)
	default:
		if p, ok := primitives[t.Name]; ok {
			base = p.goName
			if p.qualified {
				base = qualifier + base
			}
		} else {
			base = qualifier + ExportedName(t.Name)
		}
	}
	if t.Nullable && t.needsPointerForNull() {
		return "*" + base
	}
	return base
}

// needsPointerForNull reports whether a null value would otherwise be
// indistinguishable from the Go zero value.
func (t *Type) needsPointerForNull() bool {
	switch {
	case t.IsArray(), t.IsMap():
		return false
	case t.Name == Any:
		return false
	}
	return true
}

// IsNullableSlice reports whether t is a nullable array or map, whose null form
// the Go zero value already covers.
func (t *Type) IsNullableSlice() bool {
	return t.Nullable && (t.IsArray() || t.IsMap())
}

// GoUnionName names the struct standing for a union of shapes. Like the Rust
// enum, it is built from the members, so that the same union written in two
// places names one type: a filter that may be an operator or a condition is a
// FilterOperatorOrEmailFilterCondition wherever it appears.
func GoUnionName(t *Type) string { return unionName(t, ExportedName) }

// GoUnionMemberName names one alternative of a union, both as the field
// holding it and as its part of the struct's name.
func GoUnionMemberName(t *Type) string { return unionMemberName(t, ExportedName) }

// unionName names a union after its members, joined by Or, each named by
// unionMemberName with leaf naming a type that is neither a list, a map nor a
// union, as the language names it.
func unionName(t *Type, leaf func(string) string) string {
	parts := make([]string, len(t.Union))
	for i, m := range t.Union {
		parts[i] = unionMemberName(m, leaf)
	}
	return strings.Join(parts, "Or")
}

// unionMemberName names one member of a union: a list or a map after what it
// holds, and anything else by leaf. A primitive is named after the JMAP type
// rather than after the language's, so that a member is Number rather than
// Float64 or F64.
func unionMemberName(t *Type, leaf func(string) string) string {
	switch {
	case t.IsArray():
		return unionMemberName(t.Elem, leaf) + "List"
	case t.IsMap():
		return unionMemberName(t.Value, leaf) + "Map"
	case t.IsUnion():
		return unionName(t, leaf)
	}
	return leaf(t.Name)
}
