package spec

import "strings"

// tsPrimitives maps every primitive JMAP type to the TypeScript type it
// becomes. The types that carry a format rather than a shape — an id, a date, a
// duration — become named aliases of string, so that the format is visible in a
// signature and two of them cannot be swapped by accident.
var tsPrimitives = map[string]string{
	String:      "string",
	Boolean:     "boolean",
	Number:      "number",
	Int:         "number",
	UnsignedInt: "number",
	IdType:      "Id",
	DateType:    "Date",
	UTCDateType: "UTCDate",
	Any:         "unknown",

	LocalDateTimeType:  "LocalDateTime",
	DurationType:       "Duration",
	SignedDurationType: "SignedDuration",
	TimeZoneIDType:     "TimeZoneId",
}

// TSPrimitiveAliases returns the named string aliases the generated types need,
// in a stable order, each with what it is an alias for.
func TSPrimitiveAliases() []struct{ Name, Doc string } {
	return []struct{ Name, Doc string }{
		{"Id", "An id assigned by the server: 1 to 255 characters from A-Z, a-z, 0-9, _ and -, not beginning with - or #."},
		{"UTCDate", "A date and time in UTC, written as 2006-01-02T15:04:05Z."},
		{"Date", "A date and time with an offset, written as 2006-01-02T15:04:05Z07:00."},
		{"LocalDateTime", "A date and time with no zone at all, written as 2006-01-02T15:04:05. What it means depends on the time zone the enclosing object gives."},
		{"Duration", "A length of time in the ISO 8601 form, such as PT1H30M or P1D. Not a number of milliseconds: a day is not always 24 hours."},
		{"SignedDuration", "A Duration that may be negative, which is how an alert says it fires before the event it belongs to."},
		{"TimeZoneId", "A time zone from the IANA database, such as Europe/London, or a name beginning with / that refers to a zone the event itself defines."},
	}
}

// TSType renders t as a TypeScript type. Nullability is a union with null, as
// TypeScript writes it, and a union of shapes is a union of types rather than
// the unknown that Go has to fall back on.
func (t *Type) TSType() string {
	if t == nil {
		return "unknown"
	}
	var base string
	switch {
	case t.IsArray():
		base = tsElement(t.Elem) + "[]"
	case t.IsMap():
		// A map keyed by an id is written with the alias, which TypeScript
		// allows as an index signature key since it is a string underneath.
		base = "{ [key: " + t.Key.TSType() + "]: " + t.Value.TSType() + " }"
	case t.IsUnion():
		parts := make([]string, len(t.Union))
		for i, m := range t.Union {
			parts[i] = m.TSType()
		}
		base = strings.Join(parts, " | ")
	default:
		if p, ok := tsPrimitives[t.Name]; ok {
			base = p
		} else {
			base = ExportedName(t.Name)
		}
	}
	if t.Nullable {
		return base + " | null"
	}
	return base
}

// tsElement renders an array's element type, parenthesised where it is a union
// or nullable so that the brackets bind to the whole of it.
func tsElement(t *Type) string {
	s := t.TSType()
	if t.IsUnion() || t.Nullable {
		return "(" + s + ")"
	}
	return s
}

// TSName converts a JMAP name to a TypeScript identifier. TypeScript names its
// members as JMAP does, in lowerCamelCase, so a name that is already an
// identifier is left exactly as it is.
func TSName(name string) string {
	if isTSIdentifier(name) {
		return name
	}
	// A property such as "header:List-Id:asText" or "@type" is not an
	// identifier, and is written as a quoted key instead.
	return name
}

// TSNeedsQuoting reports whether a member name has to be quoted in a
// TypeScript type declaration.
func TSNeedsQuoting(name string) bool { return !isTSIdentifier(name) }

// isTSIdentifier reports whether a name can be written as a bare member name.
func isTSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '$':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
