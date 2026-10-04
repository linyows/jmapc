package jmapc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Redacted is what RedactContent puts in place of a string it withholds.
const Redacted = "[redacted]"

// KeepBodies hands a body on as it is. It is for development, where seeing the
// whole of what was sent and received is the point: a JMAP body carries the
// subjects, addresses and text of the messages it reads.
func KeepBodies(body json.RawMessage) json.RawMessage { return body }

// RedactContent returns a function that keeps the shape of a JMAP request or
// response and withholds what it says about the user's data. What it keeps is
// what a log needs to follow the exchange:
//
//   - the method names and call ids of each invocation, the capabilities in
//     using, and the ids in createdIds
//   - numbers, booleans and nulls, which say how many and whether
//   - ids: the values of id, accountId, blobId and every other key ending in
//     Id or Ids, the ids a /get is for and a /query or /changes answers with, and
//     the filter conditions that name a container by id, such as inMailbox
//   - states: state, sessionState, and every other key ending in State
//   - the property names a /get selects, the property a sort is on, and the
//     back references written with "#"
//   - type and @type, and every key of every object, which is where ids and
//     patch paths sit
//
// using, createdIds and the invocations are kept as such at the top of a body,
// and the property lists and lists of ids as the arguments of a call, which is
// where JMAP puts them; the same name anywhere else, as the arguments of
// Core/echo may hold, is redacted like any other key. Every other string, which
// is where subjects, addresses, bodies and search terms sit, becomes Redacted. keep names more keys whose string values are
// kept.
//
// A body that is not JSON is withheld whole.
func RedactContent(keep ...string) func(json.RawMessage) json.RawMessage {
	kept := make(map[string]bool, len(keep))
	for _, k := range keep {
		kept[k] = true
	}
	return func(body json.RawMessage) json.RawMessage {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return json.RawMessage(`"` + Redacted + `"`)
		}
		// Decode stops after one value, and what follows it is not JSON a
		// body would carry: the whole body is withheld rather than the
		// value before it handed on.
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return json.RawMessage(`"` + Redacted + `"`)
		}
		r := redactor{keep: kept}
		out, err := json.Marshal(r.body(v))
		if err != nil {
			return json.RawMessage(`"` + Redacted + `"`)
		}
		return out
	}
}

type redactor struct {
	keep map[string]bool
}

// argumentLists are, by the kind of method a call makes, the arguments whose
// value is a list of names or of ids though the key does not say so: the
// properties a /get selects, and what a /changes, /query or /set answers with.
// The kind is what follows the slash in the method name, which JMAP gives the
// same meaning for every type. They are kept only as the arguments of a call
// of that kind, and not under the same name anywhere else: Core/echo takes
// anything as its arguments.
var argumentLists = map[string]map[string]bool{
	"get":          {"properties": true, "bodyProperties": true, "ids": true, "notFound": true},
	"parse":        {"properties": true, "bodyProperties": true, "notFound": true},
	"query":        {"ids": true},
	"changes":      {"created": true, "updated": true, "destroyed": true},
	"set":          {"created": true, "updated": true, "destroyed": true},
	"copy":         {"created": true},
	"import":       {"created": true},
	"queryChanges": {"removed": true},
}

// keptKeys are the keys whose string value names something rather than
// carrying data: a type, a property, and the filter conditions whose value is
// the id of a container, or a list of them, which their names do not say.
var keptKeys = map[string]bool{
	"type": true, "@type": true, "property": true, "collation": true,
	"inMailbox": true, "inMailboxOtherThan": true, "inAddressBook": true, "inCalendar": true,
}

// keepsString reports whether a string under key is kept, wherever key is.
func (r redactor) keepsString(key string) bool {
	return r.keep[key] || keptKeys[key] || key == "id" ||
		strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "Ids") ||
		key == "state" || strings.HasSuffix(key, "State")
}

// body walks a whole request or response. using, the invocations and
// createdIds are what they are at the top of one, and nowhere else.
func (r redactor) body(v any) any {
	top, ok := v.(map[string]any)
	if !ok {
		return r.walk(v, "")
	}
	out := make(map[string]any, len(top))
	for k, e := range top {
		switch {
		case k == "using":
			out[k] = r.strings(e, k)
		case k == "methodCalls" || k == "methodResponses":
			out[k] = r.invocations(e)
		case strings.HasPrefix(k, "#"):
			out[k] = r.backReference(e)
		default:
			out[k] = r.walk(e, k)
		}
	}
	return out
}

// invocations keeps the method name and call id of each invocation, and walks
// its arguments.
func (r redactor) invocations(v any) any {
	list, ok := v.([]any)
	if !ok {
		return r.walk(v, "")
	}
	out := make([]any, len(list))
	for i, e := range list {
		inv, ok := e.([]any)
		if !ok || len(inv) != 3 {
			out[i] = r.walk(e, "")
			continue
		}
		name, _ := inv[0].(string)
		out[i] = []any{inv[0], r.arguments(name, inv[1]), inv[2]}
	}
	return out
}

// arguments walks the arguments of one invocation of method, keeping the lists
// JMAP puts there for a method of its kind.
func (r redactor) arguments(method string, v any) any {
	args, ok := v.(map[string]any)
	if !ok {
		return r.walk(v, "")
	}
	var lists map[string]bool
	if i := strings.LastIndex(method, "/"); i >= 0 {
		lists = argumentLists[method[i+1:]]
	}
	out := make(map[string]any, len(args))
	for k, e := range args {
		switch {
		case strings.HasPrefix(k, "#"):
			out[k] = r.backReference(e)
		case lists[k]:
			out[k] = r.strings(e, k)
		default:
			out[k] = r.walk(e, k)
		}
	}
	return out
}

// strings keeps the strings directly in a list or an object, and walks
// anything else in it as it would be walked under key.
func (r redactor) strings(v any, key string) any {
	switch v := v.(type) {
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			if s, ok := e.(string); ok {
				out[i] = s
				continue
			}
			out[i] = r.walk(e, key)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			if s, ok := e.(string); ok {
				out[k] = s
				continue
			}
			out[k] = r.walk(e, k)
		}
		return out
	default:
		return r.walk(v, key)
	}
}

// walk returns v with what it withholds replaced, key being the key v was
// found under.
func (r redactor) walk(v any, key string) any {
	switch v := v.(type) {
	case string:
		if r.keepsString(key) {
			return v
		}
		return Redacted
	case []any, map[string]any:
		if r.keepsString(key) {
			// A list of ids, or a map from ids or creation ids to ids, as
			// createdIds is.
			return r.strings(v, key)
		}
		if list, ok := v.([]any); ok {
			out := make([]any, len(list))
			for i, e := range list {
				out[i] = r.walk(e, key)
			}
			return out
		}
		obj := v.(map[string]any)
		out := make(map[string]any, len(obj))
		for k, e := range obj {
			if strings.HasPrefix(k, "#") {
				out[k] = r.backReference(e)
				continue
			}
			out[k] = r.walk(e, k)
		}
		return out
	default:
		// json.Number, bool and nil.
		return v
	}
}

// backReferenceMembers are the members of a ResultReference, RFC 8620,
// Section 3.7: a call id, a method name and a path, none of them data.
var backReferenceMembers = map[string]bool{"resultOf": true, "name": true, "path": true}

// backReference keeps the three members of a back reference, and withholds
// anything else written under a "#" key as any other value would be.
func (r redactor) backReference(v any) any {
	ref, ok := v.(map[string]any)
	if !ok {
		return r.walk(v, "")
	}
	out := make(map[string]any, len(ref))
	for k, e := range ref {
		if s, isString := e.(string); isString && backReferenceMembers[k] {
			out[k] = s
			continue
		}
		out[k] = r.walk(e, k)
	}
	return out
}
