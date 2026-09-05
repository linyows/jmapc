package spec

import (
	"sort"
	"strings"
)

// rustPrimitives maps every primitive JMAP type to the Rust type it becomes.
// The types that carry a format rather than a shape — an id, a date, a
// duration — become named aliases of String, so that the format is visible in
// a signature and a reader can see what a bare string is standing for.
//
// A date stays a string rather than becoming a chrono type: the generated code
// takes on serde and nothing else, and a caller who wants a calendar type can
// parse the string with whichever crate they already use.
var rustPrimitives = map[string]string{
	String:      "String",
	Boolean:     "bool",
	Number:      "f64",
	Int:         "i64",
	UnsignedInt: "u64",
	IdType:      "Id",
	DateType:    "Date",
	UTCDateType: "UtcDate",
	Any:         "serde_json::Value",

	LocalDateTimeType:  "LocalDateTime",
	DurationType:       "Duration",
	SignedDurationType: "SignedDuration",
	TimeZoneIDType:     "TimeZoneId",
}

// RustPrimitiveAliases returns the named string aliases the generated types
// need, in a stable order, each with what it is an alias for.
func RustPrimitiveAliases() []struct{ Name, Doc string } {
	return []struct{ Name, Doc string }{
		{"Id", "An id assigned by the server: 1 to 255 characters from A-Z, a-z, 0-9, _ and -, not beginning with - or #."},
		{"UtcDate", "A date and time in UTC, written as 2006-01-02T15:04:05Z."},
		{"Date", "A date and time with an offset, written as 2006-01-02T15:04:05Z07:00."},
		{"LocalDateTime", "A date and time with no zone at all, written as 2006-01-02T15:04:05. What it means depends on the time zone the enclosing object gives."},
		{"Duration", "A length of time in the ISO 8601 form, such as PT1H30M or P1D. Not a number of milliseconds: a day is not always 24 hours."},
		{"SignedDuration", "A Duration that may be negative, which is how an alert says it fires before the event it belongs to."},
		{"TimeZoneId", "A time zone from the IANA database, such as Europe/London, or a name beginning with / that refers to a zone the event itself defines."},
	}
}

// RustType renders t as a Rust type expression. Nullability is Option, a map
// is a BTreeMap so that what goes on the wire is ordered, and a union of
// shapes is the untagged enum RustUnionName gives that union — the shapes stay
// apart, rather than collapsing into the Value that Go has to fall back on.
func (t *Type) RustType() string {
	if t == nil {
		return "serde_json::Value"
	}
	var base string
	switch {
	case t.IsArray():
		base = "Vec<" + t.Elem.RustType() + ">"
	case t.IsMap():
		base = "BTreeMap<" + t.Key.RustType() + ", " + t.Value.RustType() + ">"
	case t.IsUnion():
		base = RustUnionName(t)
	default:
		if p, ok := rustPrimitives[t.Name]; ok {
			base = p
		} else {
			base = RustTypeName(t.Name)
		}
	}
	if t.Nullable {
		return "Option<" + base + ">"
	}
	return base
}

// RustUnionName names the enum standing for a union of shapes. It is built
// from the members, so that the same union written in two places names one
// type: a filter that may be an operator or a condition is a
// FilterOperatorOrEmailFilterCondition wherever it appears.
func RustUnionName(t *Type) string {
	parts := make([]string, len(t.Union))
	for i, m := range t.Union {
		parts[i] = rustVariantName(m)
	}
	return strings.Join(parts, "Or")
}

// rustVariantName names one alternative of a union, both as the variant itself
// and as its part of the enum's name.
func rustVariantName(t *Type) string {
	switch {
	case t.IsArray():
		return rustVariantName(t.Elem) + "List"
	case t.IsMap():
		return rustVariantName(t.Value) + "Map"
	case t.IsUnion():
		return RustUnionName(t)
	}
	// A primitive names its variant after the JMAP type rather than after the
	// Rust one, so that a variant is Number rather than F64.
	return RustTypeName(t.Name)
}

// RustTypeName converts a JMAP name to a Rust type name. Rust writes a type in
// UpperCamelCase with no run of capitals, so an initialism becomes one word:
// UTCDate is a UtcDate, and an MDN is an Mdn.
func RustTypeName(name string) string {
	var b strings.Builder
	for _, w := range rustWords(name) {
		if w == "" {
			continue
		}
		r := []rune(strings.ToLower(w))
		b.WriteString(strings.ToUpper(string(r[0])))
		b.WriteString(string(r[1:]))
	}
	s := b.String()
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "N" + s
	}
	return s
}

// RustFieldName converts a JMAP property name to a Rust field name, which is
// snake_case. A name that collides with a keyword is written as a raw
// identifier, so that "type" stays type rather than becoming something else.
func RustFieldName(name string) string {
	s := RustName(name)
	switch {
	case rustNonRaw[s]:
		// These four cannot be written as raw identifiers at all, so the
		// collision is settled with an underscore instead.
		return s + "_"
	case rustKeywords[s]:
		return "r#" + s
	}
	return s
}

// RustName converts a JMAP name to snake_case, without the raw-identifier
// prefix a keyword would need. It is what a serde rename is compared against,
// and what names a module or a function.
func RustName(name string) string {
	words := rustWords(name)
	lowered := make([]string, 0, len(words))
	for _, w := range words {
		if w != "" {
			lowered = append(lowered, strings.ToLower(w))
		}
	}
	s := strings.Join(lowered, "_")
	if s == "" {
		return "_"
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "_" + s
	}
	return s
}

// RustConstName converts a JMAP name to the SCREAMING_SNAKE_CASE Rust spells a
// constant with.
func RustConstName(name string) string {
	return strings.ToUpper(RustName(name))
}

// rustWords splits a JMAP name into the words a Rust identifier is built from.
//
// It is the shared splitting, with one repair: a run of capitals made plural by
// a trailing "s" comes back as two words, because the last capital of a run
// usually begins the next one. Go reassembles those by accident when it joins
// the words back up; snake_case does not, and would write asURLs as as_ur_ls.
func rustWords(name string) []string {
	words := splitWords(name)
	out := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) == 2 && w[1] == 's' && isUpperASCII(w[0]) && len(out) > 0 && allUpperASCII(out[len(out)-1]) {
			out[len(out)-1] += w
			continue
		}
		out = append(out, w)
	}
	return out
}

func isUpperASCII(c byte) bool { return c >= 'A' && c <= 'Z' }

func allUpperASCII(w string) bool {
	for i := 0; i < len(w); i++ {
		if !isUpperASCII(w[i]) {
			return false
		}
	}
	return w != ""
}

// SerdeRename returns the name to put in a serde rename attribute for a
// property, or the empty string where serde's own camelCase rule already
// arrives at the name on the wire.
func SerdeRename(wireName, ident string) string {
	if serdeCamelCase(strings.TrimPrefix(ident, "r#")) == wireName {
		return ""
	}
	return wireName
}

// serdeCamelCase applies the rule serde's rename_all = "camelCase" applies: the
// first word stays lowercase and the rest are capitalised.
func serdeCamelCase(ident string) string {
	parts := strings.Split(ident, "_")
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			b.WriteString(p)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// CollectUnions walks a type and records every union under it, keyed by the
// name its enum will take, so that one enum is written per distinct union
// however many properties use it.
func CollectUnions(t *Type, into map[string]*Type) {
	CollectUnionsBy(t, RustUnionName, into)
}

// CollectUnionsBy is CollectUnions under a naming of the caller's own, since
// the same union is a Rust enum in one generator and a Go struct in another.
func CollectUnionsBy(t *Type, name func(*Type) string, into map[string]*Type) {
	switch {
	case t == nil:
	case t.IsArray():
		CollectUnionsBy(t.Elem, name, into)
	case t.IsMap():
		CollectUnionsBy(t.Key, name, into)
		CollectUnionsBy(t.Value, name, into)
	case t.IsUnion():
		into[name(t)] = t
		for _, m := range t.Union {
			CollectUnionsBy(m, name, into)
		}
	}
}

// SortedUnionNames returns the names of the collected unions, in order.
func SortedUnionNames(unions map[string]*Type) []string {
	names := make([]string, 0, len(unions))
	for name := range unions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// rustKeywords are the reserved words a name derived from a JMAP property may
// collide with. The list is every keyword Rust reserves, including the ones it
// reserves for later use, since a name may not be written bare either way.
var rustKeywords = map[string]bool{
	"as": true, "break": true, "const": true, "continue": true, "crate": true,
	"dyn": true, "else": true, "enum": true, "extern": true, "false": true,
	"fn": true, "for": true, "if": true, "impl": true, "in": true,
	"let": true, "loop": true, "match": true, "mod": true, "move": true,
	"mut": true, "pub": true, "ref": true, "return": true, "self": true,
	"static": true, "struct": true, "super": true, "trait": true, "true": true,
	"type": true, "unsafe": true, "use": true, "where": true, "while": true,
	"async": true, "await": true, "union": true,
	"abstract": true, "become": true, "box": true, "do": true, "final": true,
	"macro": true, "override": true, "priv": true, "try": true, "typeof": true,
	"unsized": true, "virtual": true, "yield": true,
}

// rustNonRaw are the keywords that may not be written as raw identifiers.
var rustNonRaw = map[string]bool{"crate": true, "self": true, "super": true, "_": true}

// RustKeyword reports whether a name is one Rust reserves.
func RustKeyword(s string) bool { return rustKeywords[s] }
