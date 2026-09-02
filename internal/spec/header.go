package spec

import (
	"sort"
	"strings"
)

// A client may ask for any header field of a message by name, in any of the
// parsed forms of RFC 8621, Section 4.1.2. The property is not a member of the
// Email type — it names a header field the message may or may not carry — but
// its type is decided entirely by the form asked for, so it can be resolved
// rather than left as raw JSON.
//
//	header:List-Id                      the raw value of the last such field
//	header:List-Id:asText               the same, decoded and unfolded
//	header:Resent-To:asAddresses:all    every instance, each parsed as addresses

// headerForms maps a parsed-form suffix to the type a single instance of the
// header field takes in that form. The raw form is the one with no suffix.
var headerForms = map[string]string{
	"asRaw":              "String",
	"asText":             "String",
	"asAddresses":        "EmailAddress[]",
	"asGroupedAddresses": "EmailAddressGroup[]",
	"asMessageIds":       "String[]|null",
	"asDate":             "Date|null",
	"asURLs":             "String[]|null",
}

// HeaderProperty is a property naming one header field of a message.
type HeaderProperty struct {
	// Name is the header field's name, as written in the property.
	Name string
	// Form is the parsed form asked for, empty for the raw form.
	Form string
	// All reports whether every instance of the field was asked for rather
	// than just the last.
	All bool
	// Type is the type the property's value takes.
	Type string
}

// ParseHeaderProperty reads a property of the form
// "header:{name}[:as{form}][:all]", as defined in RFC 8621, Section 4.1.3. It
// reports whether the property is one of these at all, and what is wrong with
// it when the name is right but the rest is not.
func ParseHeaderProperty(property string) (*HeaderProperty, error) {
	rest, ok := strings.CutPrefix(property, "header:")
	if !ok {
		return nil, nil
	}

	h := &HeaderProperty{}
	parts := strings.Split(rest, ":")
	h.Name = parts[0]
	if h.Name == "" {
		return nil, &HeaderPropertyError{Property: property, Problem: "names no header field"}
	}
	for _, c := range h.Name {
		// A field name is printable ASCII apart from the colon, which is what
		// separates the parts of this property in the first place.
		if c < 33 || c > 126 {
			return nil, &HeaderPropertyError{
				Property: property,
				Problem:  "has a header field name outside the printable ASCII range",
			}
		}
	}

	// The suffixes come in a fixed order: the form, then :all.
	suffixes := parts[1:]
	if len(suffixes) > 0 && suffixes[len(suffixes)-1] == "all" {
		h.All = true
		suffixes = suffixes[:len(suffixes)-1]
	}
	switch len(suffixes) {
	case 0:
	case 1:
		h.Form = suffixes[0]
		if _, known := headerForms[h.Form]; !known {
			return nil, &HeaderPropertyError{
				Property: property,
				Problem:  "asks for the " + h.Form + " form, which is not one this specification defines",
				Forms:    HeaderFormNames(),
			}
		}
	default:
		return nil, &HeaderPropertyError{
			Property: property,
			Problem:  "has more suffixes than the form and :all",
		}
	}

	h.Type = headerFormType(h.Form, h.All)
	return h, nil
}

// headerFormType returns the type a header property takes in the given form.
// Asking for every instance gives an array of what one instance would be, and
// asking for a single one gives null where the message has no such field.
func headerFormType(form string, all bool) string {
	single := headerForms["asRaw"]
	if form != "" {
		single = headerForms[form]
	}
	if all {
		// An array of whatever one instance would be. Every item corresponds
		// to a field that is there, so the array itself adds no null; a form
		// that is nullable in its own right keeps that null inside, and needs
		// parentheses to say so.
		if strings.Contains(single, "|") {
			return "(" + single + ")[]"
		}
		return single + "[]"
	}
	if strings.HasSuffix(single, "|null") {
		return single
	}
	return single + "|null"
}

// HeaderFormNames returns the parsed-form suffixes, sorted, for diagnostics.
func HeaderFormNames() []string {
	names := make([]string, 0, len(headerForms))
	for name := range headerForms {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HeaderPropertyError says what is wrong with a property naming a header
// field.
type HeaderPropertyError struct {
	// Property is the property as written.
	Property string
	// Problem describes what is wrong with it.
	Problem string
	// Forms lists the parsed forms, where naming an unknown one is the
	// problem.
	Forms []string
}

func (e *HeaderPropertyError) Error() string {
	return e.Property + " " + e.Problem
}
