package jmapc

import (
	"bytes"
	"encoding/json"
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
//   - the method names and call ids of each invocation, and the capabilities
//     in using
//   - numbers, booleans and nulls, which say how many and whether
//   - ids: the values of id, accountId, blobId and every other key ending in
//     Id, Ids or ids, the lists of ids a /changes or /set answers with, and
//     the filter conditions that name a container by id, such as inMailbox
//   - states: state, sessionState, and every other key ending in State
//   - the property names a /get selects, the property a sort is on, and the
//     back references written with "#"
//   - type and @type, and every key of every object, which is where ids and
//     patch paths sit
//
// Every other string, which is where subjects, addresses, bodies and search
// terms sit, becomes Redacted. keep names more keys whose string values are
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
		r := redactor{keep: kept}
		out, err := json.Marshal(r.walk(v, ""))
		if err != nil {
			return json.RawMessage(`"` + Redacted + `"`)
		}
		return out
	}
}

type redactor struct {
	keep map[string]bool
}

// idLists are the keys whose value is a list of ids, though the key does not
// say so: what a /changes, /query or /set answers with, and the filter
// condition that excludes mailboxes.
var idLists = map[string]bool{
	"created": true, "updated": true, "destroyed": true,
	"notFound": true, "removed": true, "ids": true,
	"inMailboxOtherThan": true,
}

// keptLists are the keys whose value is a list of names rather than of data.
var keptLists = map[string]bool{
	"using": true, "properties": true, "bodyProperties": true,
}

// keptKeys are the keys whose string value names something rather than
// carrying data: a type, a property, and the filter conditions whose value is
// the id of a container, which their names do not say.
var keptKeys = map[string]bool{
	"type": true, "@type": true, "property": true, "collation": true,
	"inMailbox": true, "inAddressBook": true, "inCalendar": true,
}

// keepsString reports whether a string under key is kept.
func (r redactor) keepsString(key string) bool {
	return r.keep[key] || keptKeys[key] || key == "id" ||
		strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "Ids") || strings.HasSuffix(key, "ids") ||
		key == "state" || strings.HasSuffix(key, "State")
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
	case []any:
		if key == "methodCalls" || key == "methodResponses" {
			return r.invocations(v)
		}
		names := keptLists[key] || idLists[key] || r.keepsString(key)
		out := make([]any, len(v))
		for i, e := range v {
			if s, ok := e.(string); ok && names {
				out[i] = s
				continue
			}
			out[i] = r.walk(e, key)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			if strings.HasPrefix(k, "#") {
				// A back reference: a call id, a method name and a path.
				out[k] = e
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

// invocations keeps the method name and call id of each invocation, and walks
// its arguments.
func (r redactor) invocations(list []any) []any {
	out := make([]any, len(list))
	for i, e := range list {
		inv, ok := e.([]any)
		if !ok || len(inv) != 3 {
			out[i] = r.walk(e, "")
			continue
		}
		out[i] = []any{inv[0], r.walk(inv[1], ""), inv[2]}
	}
	return out
}
