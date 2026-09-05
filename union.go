package jmapc

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// A union is a value a specification lets take either of two shapes: the
// filter of a /query call is a boolean operator or a condition on the type
// being queried, and a contact's anniversary date is a partial date or a
// timestamp. Go cannot express "one of these shapes" directly, so each union is
// generated as a struct with one field per shape, of which exactly one is set.
// The functions here are what those structs marshal and unmarshal through.

// maxUnionValueInError caps how much of a value that fits no shape is quoted
// back in the error, since the value may be a whole filter tree.
const maxUnionValueInError = 96

// marshalUnion writes the one alternative that is set. A union that holds
// nothing, or more than one alternative, is an error in the calling code rather
// than a value a server could interpret.
func marshalUnion(name string, set []any) ([]byte, error) {
	switch len(set) {
	case 1:
		return json.Marshal(set[0])
	case 0:
		return nil, fmt.Errorf("jmapc: %s: no alternative is set, and a union carries one", name)
	default:
		return nil, fmt.Errorf("jmapc: %s: %d alternatives are set, and a union carries one", name, len(set))
	}
}

// unionAlt is one alternative of a union: where to decode it, which properties
// an object of that shape must have, and how to record it once it fits.
type unionAlt struct {
	// Required names the properties the shape must have, which is most of what
	// distinguishes one object shape from another.
	Required []string
	// Into points at a value of the alternative's own type.
	Into any
	// Set records on the union the value Into now holds.
	Set func()
}

// unmarshalUnion fills the first alternative the value fits.
//
// It decodes twice. On the first pass, an alternative has to account for every
// property the value carries, which is what distinguishes two object shapes
// that share no property: a filter operator has an "operator" and a condition
// does not. On the second pass, unknown properties are allowed, so that a
// server which has added a property to a shape jmapc knows does not make the
// value unreadable, which is how every other type in this package treats an
// unknown property.
func unmarshalUnion(name string, data []byte, alts []unionAlt) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	keys, isObject := unionKeys(data)
	for _, strict := range []bool{true, false} {
		for _, alt := range alts {
			if isObject && !hasAll(keys, alt.Required) {
				continue
			}
			if err := decodeUnionAlt(data, alt.Into, strict); err != nil {
				continue
			}
			alt.Set()
			return nil
		}
	}
	return fmt.Errorf("jmapc: %s: the value fits none of the shapes it may take: %s",
		name, abbreviateJSON(data))
}

// unionKeys returns the properties of a JSON object, and reports whether the
// value was one at all.
func unionKeys(data []byte) (map[string]json.RawMessage, bool) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil || keys == nil {
		return nil, false
	}
	return keys, true
}

// hasAll reports whether every named property is present.
func hasAll(keys map[string]json.RawMessage, required []string) bool {
	for _, name := range required {
		if _, ok := keys[name]; !ok {
			return false
		}
	}
	return true
}

// decodeUnionAlt decodes one alternative, refusing a property the shape does
// not have where strict.
func decodeUnionAlt(data []byte, into any, strict bool) error {
	if !strict {
		return json.Unmarshal(data, into)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data after the value")
	}
	return nil
}

// abbreviateJSON shortens a value for an error message.
func abbreviateJSON(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) <= maxUnionValueInError {
		return string(data)
	}
	return string(data[:maxUnionValueInError]) + "..."
}
