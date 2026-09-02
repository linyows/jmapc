package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/spec"
)

// Extension is the suffix that marks a file as a JMAP query.
const Extension = ".jmap.json"

// Parser reads query files and checks them against a catalogue.
type Parser struct {
	// Spec is the catalogue the queries are checked against.
	Spec *spec.Spec
}

// NewParser returns a parser that checks queries against s.
func NewParser(s *spec.Spec) *Parser {
	return &Parser{Spec: s}
}

// fileSyntax is the shape of a query file: a JMAP Request object, plus the few
// members the generator needs and the server ignores.
type fileSyntax struct {
	// Doc documents the query, and becomes the generated function's comment.
	Doc string `json:"doc"`
	// Using lists the capabilities the request declares. It may be left out,
	// in which case it is derived from the methods called.
	Using []string `json:"using"`
	// Returns names the call whose result the generated function returns. It
	// may be left out, in which case every result is returned.
	Returns string `json:"returns"`
	// MethodCalls are the calls the request makes.
	MethodCalls []json.RawMessage `json:"methodCalls"`
}

// capabilityAliases lets a query name a capability by a short name instead of
// its full URI.
var capabilityAliases = map[string]string{
	"core":             spec.CapabilityCore,
	"mail":             spec.CapabilityMail,
	"submission":       "urn:ietf:params:jmap:submission",
	"vacationresponse": "urn:ietf:params:jmap:vacationresponse",
	"contacts":         spec.CapabilityContacts,
	"calendars":        spec.CapabilityCalendars,
	"calendars:parse":  spec.CapabilityCalendarsParse,
	"availability":     spec.CapabilityAvailability,
}

// QueryName returns the name a query file gives its query, which is the file
// name with its extension removed.
func QueryName(path string) string {
	base := filepath.Base(path)
	if strings.HasSuffix(base, Extension) {
		return strings.TrimSuffix(base, Extension)
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ParseFile reads and checks the query in the file at path.
func (p *Parser) ParseFile(path string) (*Query, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return p.Parse(path, src)
}

// Parse checks the query in src, which came from the file at path. It reports
// every problem it finds rather than stopping at the first, so that a query can
// be corrected in one pass.
func (p *Parser) Parse(path string, src []byte) (*Query, error) {
	name := QueryName(path)
	if !isGoIdentifier(name) {
		return nil, ErrorList{{
			File: path,
			Msg:  fmt.Sprintf("query name %q, taken from the file name, is not a Go identifier", name),
			Hint: "name the file after the function to generate, as in ListInboxEmails" + Extension,
		}}
	}

	var f fileSyntax
	dec := json.NewDecoder(bytes.NewReader(stripComments(src)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, ErrorList{{File: path, Msg: syntaxMessage(err)}}
	}

	c := &checker{
		spec:   p.Spec,
		file:   path,
		params: newParamSet(),
		byID:   make(map[string]*Call),
	}
	q := &Query{Name: name, Path: path, Doc: f.Doc}

	if len(f.MethodCalls) == 0 {
		c.errorf("methodCalls", "", "the query makes no method calls")
		return nil, c.errs
	}
	for i, raw := range f.MethodCalls {
		if call := c.methodCall(raw, fmt.Sprintf("methodCalls[%d]", i)); call != nil {
			q.Calls = append(q.Calls, call)
		}
	}

	q.Using = c.resolveUsing(f.Using, q.Calls)
	q.Params = c.params.all()
	if f.Returns != "" {
		call, ok := c.byID[f.Returns]
		if !ok {
			c.errorf("returns", "", "no method call has the id %q", f.Returns).
				hint(hintFor(f.Returns, c.callIDs()))
		}
		q.Returns = call
	}
	assignGoNames(q)

	if len(c.errs) > 0 {
		return nil, c.errs
	}
	return q, nil
}

// checker accumulates the problems found while checking one query file.
type checker struct {
	spec   *spec.Spec
	file   string
	errs   ErrorList
	params *paramSet
	byID   map[string]*Call
	order  []string

	// filterUnion is the type a /query filter may take, carried down so that
	// the conditions nested inside a FilterOperator can be checked against the
	// data type being queried instead of being waved through as Any.
	filterUnion *spec.Type

	// patchTarget names the data type that the PatchObjects being checked
	// apply to, carried down from the argument that holds them.
	patchTarget string

	// sortTarget names the data type whose sortable properties a Comparator
	// being checked may name, carried down the same way.
	sortTarget string
}

// errorf records a problem and returns it, so that a hint can be chained on.
func (c *checker) errorf(where, hint, format string, args ...any) *Error {
	e := &Error{File: c.file, Where: where, Msg: fmt.Sprintf(format, args...), Hint: hint}
	c.errs = append(c.errs, e)
	return e
}

// hint attaches a suggestion to an error.
func (e *Error) hint(h string) *Error {
	if h != "" {
		e.Hint = h
	}
	return e
}

// callIDs returns the call ids seen so far, in order.
func (c *checker) callIDs() []string { return c.order }

// methodCall checks one entry of methodCalls: the [name, arguments, callId]
// triple of RFC 8620, Section 3.2.
func (c *checker) methodCall(raw json.RawMessage, where string) *Call {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		c.errorf(where, "a method call is written as [name, arguments, callId]",
			"expected an array, found %s", jsonKind(raw))
		return nil
	}
	if len(parts) != 3 {
		c.errorf(where, "a method call is written as [name, arguments, callId]",
			"expected 3 elements, found %d", len(parts))
		return nil
	}

	var methodName, callID string
	if err := json.Unmarshal(parts[0], &methodName); err != nil {
		c.errorf(where+"[0]", "", "expected the method name as a string, found %s", jsonKind(parts[0]))
		return nil
	}
	if err := json.Unmarshal(parts[2], &callID); err != nil {
		c.errorf(where+"[2]", "", "expected the call id as a string, found %s", jsonKind(parts[2]))
		return nil
	}

	method, ok := c.spec.Method(methodName)
	if !ok {
		c.errorf(where+"[0]", hintFor(methodName, c.spec.MethodNames()), "unknown method %q", methodName)
		return nil
	}
	if callID == "" {
		c.errorf(where+"[2]", "", "the call id is empty")
		return nil
	}
	if prev, dup := c.byID[callID]; dup {
		c.errorf(where+"[2]", "", "call id %q is already used by the %s call", callID, prev.Method.Name)
		return nil
	}

	call := &Call{ID: callID, Method: method}
	c.byID[callID] = call
	c.order = append(c.order, callID)

	args, err := c.spec.ArgumentsOf(methodName)
	if err != nil {
		c.errorf(where, "", "%v", err)
		return call
	}
	call.Args = c.arguments(call, args, parts[1], where+".arguments")
	call.Properties = c.properties(call, where+".arguments")
	return call
}

// arguments checks the argument object of a method call. Only here, at the top
// level of the arguments, may a member name begin with "#" to mark a back
// reference.
func (c *checker) arguments(call *Call, argsType *spec.Object, raw json.RawMessage, where string) *Object {
	keys, err := objectKeys(raw)
	if err != nil {
		c.errorf(where, "", "expected an object of arguments, found %s", jsonKind(raw))
		return &Object{}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		c.errorf(where, "", "expected an object of arguments, found %s", jsonKind(raw))
		return &Object{}
	}

	out := &Object{}
	seen := make(map[string]string, len(keys))
	for _, key := range keys {
		name := strings.TrimPrefix(key, "#")
		if prev, dup := seen[name]; dup {
			c.errorf(where+"."+key, "",
				"argument %q is already set by %q; an argument is either given directly or by a back reference, not both",
				name, prev)
			continue
		}
		seen[name] = key

		field, known := argsType.Field(name)
		if !known {
			if len(argsType.Fields) > 0 {
				c.errorf(where+"."+key, hintFor(name, argsType.PropertyNames()),
					"%s has no argument %q", call.Method.Name, name)
				continue
			}
			// The method takes whatever it is given, as Core/echo does, so
			// there is no declared type to check the value against.
			field = &spec.Field{Name: name, Type: spec.Any}
		}
		if key != name {
			ref := c.resultRef(call, field, members[key], where+"."+key)
			if ref != nil {
				out.Fields = append(out.Fields, ObjectField{Key: key, Value: ref})
			}
			continue
		}
		savedPatch, savedSort := c.patchTarget, c.sortTarget
		if field.PatchTarget != "" {
			c.patchTarget = field.PatchTarget
		}
		if field.SortTarget != "" {
			c.sortTarget = field.SortTarget
		}
		node := c.value(field.ParsedType(), members[key], where+"."+key, field.Doc)
		c.patchTarget, c.sortTarget = savedPatch, savedSort
		out.Fields = append(out.Fields, ObjectField{Key: key, Value: node})
	}
	return out
}

// resultRef checks a back reference, as defined in RFC 8620, Section 3.7. It
// verifies that the referenced call comes earlier, that the method name matches
// the call it names, and that the value the path selects can stand in for the
// argument being filled.
func (c *checker) resultRef(call *Call, field *spec.Field, raw json.RawMessage, where string) *ResultRef {
	var ref jmapc.ResultReference
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ref); err != nil {
		c.errorf(where, `a back reference is written as {"resultOf": ..., "name": ..., "path": ...}`,
			"%s", syntaxMessage(err))
		return nil
	}

	from, ok := c.byID[ref.ResultOf]
	if !ok {
		c.errorf(where+".resultOf", hintFor(ref.ResultOf, c.callIDs()),
			"no earlier method call has the id %q", ref.ResultOf)
		return nil
	}
	if from == call {
		c.errorf(where+".resultOf", "", "a back reference cannot point at its own call")
		return nil
	}
	if ref.Name != from.Method.Name {
		c.errorf(where+".name", fmt.Sprintf("call %q invokes %s", ref.ResultOf, from.Method.Name),
			"the referenced call is %s, but the reference names %s", from.Method.Name, ref.Name)
		return nil
	}

	got, err := c.spec.ResolvePath(from.Method.Name, ref.Path)
	if err != nil {
		c.errorf(where+".path", propertyHint(err), "%v", err)
		return nil
	}
	want := field.ParsedType()
	if !assignable(got, want) {
		c.errorf(where, "",
			"argument %q of %s expects %s, but %s selects %s from the result of %s",
			field.Name, call.Method.Name, want, ref.Path, got, from.Method.Name)
		return nil
	}
	return &ResultRef{Argument: field.Name, Ref: ref, From: from}
}

// properties extracts the property names a /get call selects, when the query
// states them literally, and checks each one against the data type.
func (c *checker) properties(call *Call, where string) []string {
	if call.Method.PropertiesArgument == "" || call.Args == nil {
		return nil
	}
	node, ok := call.Args.Find(call.Method.PropertiesArgument)
	if !ok {
		return nil
	}
	arr, ok := node.(*Array)
	if !ok {
		// The properties are a parameter or null, so the call may fetch
		// anything and the shared type is the only sound choice.
		return nil
	}
	dataType, ok := c.spec.Object(call.Method.DataType)
	if !ok {
		return nil
	}
	var props []string
	for i, item := range arr.Items {
		lit, ok := item.(*Literal)
		if !ok {
			return nil
		}
		var name string
		if err := json.Unmarshal(lit.JSON, &name); err != nil {
			return nil
		}
		if _, known := dataType.Field(name); !known && !isDynamicProperty(name) {
			c.errorf(fmt.Sprintf("%s.%s[%d]", where, call.Method.PropertiesArgument, i),
				hintFor(name, dataType.PropertyNames()),
				"%s has no property %q", dataType.Name, name)
			continue
		}
		props = append(props, name)
	}
	return props
}

// isDynamicProperty reports whether a property name is one of the header field
// forms of RFC 8621, Section 4.1.3, which are not fixed members of the type.
func isDynamicProperty(name string) bool {
	return strings.HasPrefix(name, "header:")
}

// resolveUsing returns the capability URIs the request should declare, either
// checking the ones the query states or deriving them from the methods called.
func (c *checker) resolveUsing(declared []string, calls []*Call) []string {
	needed := map[string]bool{spec.CapabilityCore: true}
	for _, call := range calls {
		if call.Method.Capability != "" {
			needed[call.Method.Capability] = true
		}
	}
	if len(declared) == 0 {
		return sortedCapabilities(needed)
	}
	have := make(map[string]bool, len(declared))
	out := make([]string, 0, len(declared))
	for i, u := range declared {
		uri, ok := capabilityAliases[u]
		if !ok {
			uri = u
		}
		if !strings.HasPrefix(uri, "urn:") {
			c.errorf(fmt.Sprintf("using[%d]", i), hintFor(u, aliasNames()),
				"%q is not a capability URI", u)
			continue
		}
		have[uri] = true
		out = append(out, uri)
	}
	for _, call := range calls {
		if cap := call.Method.Capability; cap != "" && !have[cap] {
			c.errorf("using", fmt.Sprintf("add %q to using", cap),
				"%s needs the capability %s, which the query does not declare", call.Method.Name, cap)
			have[cap] = true
		}
	}
	return out
}

// sortedCapabilities returns the capability URIs in a stable order, with the
// core capability first because every request depends on it.
func sortedCapabilities(set map[string]bool) []string {
	out := []string{spec.CapabilityCore}
	var rest []string
	for uri := range set {
		if uri != spec.CapabilityCore {
			rest = append(rest, uri)
		}
	}
	sortStrings(rest)
	return append(out, rest...)
}

// aliasNames returns the short capability names a query may use.
func aliasNames() []string {
	out := make([]string, 0, len(capabilityAliases))
	for name := range capabilityAliases {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

// isGoIdentifier reports whether s can be used as an exported Go identifier.
var goIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isGoIdentifier(s string) bool { return goIdentifier.MatchString(s) }

// syntaxMessage renders a JSON decoding error without the "json:" prefix and
// the Go type names that mean nothing to someone reading a query file.
func syntaxMessage(err error) string {
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "json: ")
	if i := strings.Index(msg, " in Go struct field"); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.ReplaceAll(msg, "query.fileSyntax", "the query")
	msg = strings.ReplaceAll(msg, "jmapc.ResultReference", "the back reference")
	return msg
}
