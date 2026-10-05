package request

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Repeat is a call that asks what an earlier call in the same request already
// asked, with nothing between the two that could change the answer.
type Repeat struct {
	// Call is the later call, which the request could do without.
	Call *Call
	// Same is the earlier call it repeats, whose result Call's readers could
	// refer to instead.
	Same *Call
}

// readOnly are the method names, after the slash, that only read. RFC 8620
// lets data change between the calls of one request, so a call is a repeat only
// where every call between it and the one it repeats is one of these. A method
// not named here, a vendor's own included, is taken as one that may change
// data, so that nothing is reported where it is not known to be a repeat.
var readOnly = map[string]bool{
	"get":             true,
	"query":           true,
	"queryChanges":    true,
	"changes":         true,
	"parse":           true,
	"validate":        true,
	"echo":            true,
	"lookup":          true,
	"getAvailability": true,
}

// readsOnly reports whether a method only reads, by the name after its slash.
func readsOnly(method string) bool {
	_, verb, ok := strings.Cut(method, "/")
	return ok && readOnly[verb]
}

// repeatsOf finds the calls that repeat an earlier one, as pairs of call ids:
// the later call, then the one it repeats. Two calls are the same where they
// call one method with the same arguments, compared as JSON so that the order
// of members does not count, without _comment, which never reaches the server,
// and with parameters compared by name, since one name is one value. A back
// reference to a call found to repeat another is read as one to the other, so
// that a query and the get reading it, written twice, are both found.
//
// It works from the method calls as written, which the parser has already
// checked, and returns nothing for a call it cannot read.
func repeatsOf(calls []json.RawMessage) [][2]string {
	// same maps a call id to the earliest call id asking the same thing.
	same := map[string]string{}
	// asked maps what a call asks to the call that asked it first, since the
	// last call that may have changed data.
	asked := map[string]string{}
	var repeats [][2]string
	for _, raw := range calls {
		var call []any
		if err := json.Unmarshal(raw, &call); err != nil || len(call) != 3 {
			continue
		}
		method, _ := call[0].(string)
		args, _ := call[1].(map[string]any)
		id, _ := call[2].(string)
		if !readsOnly(method) {
			// What was asked before may have a different answer after.
			clear(asked)
			continue
		}
		delete(args, CommentArgument)
		key, err := json.Marshal([]any{method, sameReferences(args, same)})
		if err != nil {
			continue
		}
		if earlier, ok := asked[string(key)]; ok {
			repeats = append(repeats, [2]string{id, earlier})
			same[id] = earlier
			continue
		}
		asked[string(key)] = id
	}
	return repeats
}

// sameReferences rewrites a value so that a back reference to a repeated call
// names the call it repeats, and a parameter is written one way however it was
// spaced. json.Marshal writes the members of a map in order of their names, so
// the order they were written in is lost on the way.
func sameReferences(v any, same map[string]string) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, member := range value {
			if id, ok := member.(string); ok && key == "resultOf" {
				if earlier, ok := same[id]; ok {
					id = earlier
				}
				out[key] = id
				continue
			}
			out[sameParameters(key)] = sameReferences(member, same)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = sameReferences(item, same)
		}
		return out
	case string:
		return sameParameters(value)
	}
	return v
}

// sameParameters writes each parameter in a string without the spaces its
// braces may hold, so that {{ id }} and {{id}} are one parameter.
func sameParameters(text string) string {
	return shapeParamPattern.ReplaceAllStringFunc(text, func(match string) string {
		m := shapeParamPattern.FindStringSubmatch(match)
		return fmt.Sprintf("{{%s%s}}", m[1], m[2])
	})
}
