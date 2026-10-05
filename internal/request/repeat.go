package request

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"

	"github.com/linyows/jmapc/internal/spec"
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

// readOnly are the method names, after the slash, that only read where the
// specifications define them. RFC 8620 lets data change between the calls of
// one request, so a call is a repeat only where every call between it and the
// one it repeats is known to only read.
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

// standardReads are the methods of the specifications jmapc knows that only
// read. A vendor schema may define a method under any name, and nothing says
// that its Note/get or Note/echo only reads, so a method is taken as one that
// may change data unless it is one of these.
var standardReads = sync.OnceValue(func() map[string]bool {
	reads := map[string]bool{}
	for _, name := range spec.Standard().MethodNames() {
		if _, verb, ok := strings.Cut(name, "/"); ok && readOnly[verb] {
			reads[name] = true
		}
	}
	return reads
})

// readsOnly reports whether a method is one the specifications define as only
// reading.
func readsOnly(method string) bool {
	return standardReads()[method]
}

// repeatsOf finds the calls that repeat an earlier one. Two calls are the same
// where they call one method with the same checked arguments: compared without
// regard to the order of members, with parameters compared by name, since one
// name is one value, and with a back reference to a call found to repeat
// another read as one to the other, so that a query and the get reading it,
// written twice, are both found. What the request states outright is compared
// as it was written, whatever it looks like.
func repeatsOf(calls []*Call) []Repeat {
	// same maps a call to the earliest call asking the same thing.
	same := map[*Call]*Call{}
	// asked maps what a call asks to the call that asked it first, since the
	// last call that may have changed data.
	asked := map[string]*Call{}
	var repeats []Repeat
	for _, call := range calls {
		if !readsOnly(call.Method.Name) {
			// What was asked before may have a different answer after.
			clear(asked)
			continue
		}
		key, err := json.Marshal([]any{call.Method.Name, comparable(call.Args, same)})
		if err != nil {
			continue
		}
		if earlier, ok := asked[string(key)]; ok {
			repeats = append(repeats, Repeat{Call: call, Same: earlier})
			same[call] = earlier
			continue
		}
		asked[string(key)] = call
	}
	return repeats
}

// comparable returns a checked value as something to compare, tagged by kind
// so that a parameter or a reference never equals a literal that happens to
// look like one. json.Marshal writes the members of a map in order of their
// names, which is what makes the order they were written in not count.
func comparable(n Node, same map[*Call]*Call) any {
	switch node := n.(type) {
	case nil:
		return nil
	case *Literal:
		return []any{"literal", literal(node.JSON)}
	case *ParamRef:
		return []any{"param", node.Param.Name}
	case *ResultRef:
		from := node.From
		if earlier, ok := same[from]; ok {
			from = earlier
		}
		id := ""
		if from != nil {
			id = from.ID
		}
		return []any{"ref", node.Argument, id, node.Ref.Name, node.Ref.Path}
	case *Array:
		items := make([]any, len(node.Items))
		for i, item := range node.Items {
			items[i] = comparable(item, same)
		}
		return []any{"array", items}
	case *Object:
		fields := make(map[string]any, len(node.Fields))
		for _, f := range node.Fields {
			fields[memberName(f)] = comparable(f.Value, same)
		}
		return []any{"object", fields}
	}
	return nil
}

// memberName writes a member name with each parameter it is built from marked
// as one, so that a parameter never equals literal text.
func memberName(f ObjectField) string {
	if f.KeySegments == nil {
		return "literal:" + f.Key
	}
	var b strings.Builder
	b.WriteString("segments:")
	for _, s := range f.KeySegments {
		if s.Param != nil {
			b.WriteString("\x00" + s.Param.Name + "\x00")
			continue
		}
		b.WriteString(s.Text)
	}
	return b.String()
}

// literal decodes a value the request states outright, keeping each number as
// the digits it was written with: two integers too large for a float64 to tell
// apart are still two values.
func literal(raw json.RawMessage) any {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return string(raw)
	}
	return v
}
