package query

import (
	"bytes"
	"encoding/json"
	"errors"
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

// The members a query file carries for the generator rather than for the
// server all begin with an underscore, which is the whole rule: an underscore
// means jmapc reads it, and anything else is the JMAP Request object as RFC
// 8620 defines it. JMAP names its own members in lowerCamelCase, so nothing it
// adds later can collide.
//
// An underscore rather than a dot, because jmapc writes the path to a problem
// with dots — methodCalls[0].arguments.filter — and a member named ".comment"
// would read as part of one.
const (
	// DocMember documents the query and becomes the generated function's
	// comment.
	DocMember = "_doc"
	// ReturnsMember names the call whose response the function returns.
	ReturnsMember = "_returns"
	// WatchesMember names the call whose state drives a watch: the generated
	// client follows that type's changes, calling the query whenever the
	// server says there is something to catch up on.
	WatchesMember = "_watches"
	// PagesMember names the call a generated pager advances: one request
	// answers with one window of a longer list, and the pager asks for the
	// next until there is none.
	PagesMember = "_pages"
	// CreatedIDsMember asks for the creation ids of a request to be carried in
	// and out, so that one request can go on from where another left off.
	CreatedIDsMember = "_createdIds"
	// SchemaMember names the JSON Schema an editor checks the file against,
	// which "jmapc schema" writes. jmapc reads it only to allow it.
	SchemaMember = "$schema"
	// CommentArgument explains what a call is for. It sits in that call's
	// arguments, and the generator strips it before the request goes out:
	// RFC 8620 requires a server to reject an argument it does not know.
	CommentArgument = "_comment"
)

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
// A member jmapc reads begins with an underscore and one the specification
// defines does not, so that a reader can tell at a glance which is which.
type fileSyntax struct {
	// Schema names the JSON Schema an editor checks the file against. It is
	// the one member here that is neither jmapc's nor the specification's:
	// "$schema" is what an editor looks for, and jmapc reads the file whether
	// it is there or not.
	Schema string `json:"$schema"`
	// Doc documents the query, and becomes the generated function's comment.
	Doc string `json:"_doc"`
	// Returns names the call whose result the generated function returns. It
	// may be left out, in which case every result is returned.
	Returns string `json:"_returns"`
	// CreatedIDs asks the generated function to take the creation ids of an
	// earlier request and to report its own.
	CreatedIDs bool `json:"_createdIds"`
	// Watches names the call whose state a watching client follows, and asks
	// for the function that follows it.
	Watches string `json:"_watches"`
	// Pages names the call a pager advances, and asks for the function that
	// walks the whole of what one request returns a window of.
	Pages string `json:"_pages"`
	// Using lists the capabilities the request declares, as RFC 8620 defines
	// it. It may be left out, in which case it is derived from the methods
	// called.
	Using []string `json:"using"`
	// MethodCalls are the calls the request makes, as RFC 8620 defines them.
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
	"principals":       spec.CapabilityPrincipals,
	"smimeverify":      spec.CapabilitySMIMEVerify,
	"blob":             spec.CapabilityBlob,
	"quota":            spec.CapabilityQuota,
	"sieve":            spec.CapabilitySieve,
	"mdn":              spec.CapabilityMDN,
}

// CapabilityAliases returns the short names a query may use in place of a
// capability URI, sorted.
func CapabilityAliases() []string { return aliasNames() }

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
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, ErrorList{{File: path, Msg: syntaxMessage(err)}}
	}

	c := &checker{
		spec:   p.Spec,
		file:   path,
		params: newParamSet(),
		byID:   make(map[string]*Call),
		used:   make(map[string]bool),
	}
	q := &Query{Name: name, Path: path, Doc: f.Doc, CreatedIDs: f.CreatedIDs}

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
	if f.Returns != "" && f.CreatedIDs {
		c.errorf(ReturnsMember, "",
			"a query carrying %s returns every response, since the creation ids belong to the request rather than to any one call",
			CreatedIDsMember)
	}
	if f.Returns != "" {
		call, ok := c.byID[f.Returns]
		if !ok {
			c.errorf(ReturnsMember, "", "no method call has the id %q", f.Returns).
				hint(hintFor(f.Returns, c.callIDs()))
		}
		q.Returns = call
	}
	c.watch(q, f)
	c.pages(q, f)
	c.checkOptional(q)
	assignFieldNames(q)

	if len(c.errs) > 0 {
		return nil, c.errs
	}
	return q, nil
}

// checkOptional holds a parameter that may be left out to the one reading it
// has: this argument is not in the request. A parameter used in a second place
// would have to be there for that other use, and one the generated loop fills
// in is not the caller's to leave out at all.
func (c *checker) checkOptional(q *Query) {
	for _, p := range q.Params {
		if !p.Optional {
			continue
		}
		if len(p.Places) > 1 {
			c.errorf(p.Places[0], "use a parameter of its own for the value that is always there",
				"%q may be left out, so it stands for an argument that may not be there, but it is used again at %s",
				p.Name, strings.Join(p.Places[1:], ", "))
		}
		switch p {
		case q.WatchState:
			c.errorf(p.Places[0], "write it as {{"+p.Name+"}}",
				"the %s of a watched call is where the loop has reached, so it cannot be left out",
				SinceStateArgument)
		case q.PageStart:
			c.errorf(p.Places[0], "write it as {{"+p.Name+"}}",
				"the start of a paged call is where the next request begins, so it cannot be left out")
		}
	}
}

// watch checks the call a query says drives a watch, and records what the
// generated loop needs to follow it: which call reports the state, and which
// parameter carries it back in.
//
// A watched call is one that reports what changed since a state — a /changes
// call — because that is the only kind a loop can go on from. What the server
// pushes is that a type has moved on, not what changed, so the loop asks; and
// what it asks with is the state the last answer left it at.
func (c *checker) watch(q *Query, f fileSyntax) {
	if f.Watches == "" {
		return
	}
	call, ok := c.byID[f.Watches]
	if !ok {
		c.errorf(WatchesMember, "", "no method call has the id %q", f.Watches).
			hint(hintFor(f.Watches, c.callIDs()))
		return
	}
	if !c.reportsChanges(call) {
		c.errorf(WatchesMember, "a watched call reports what changed since a state, as "+call.Method.DataType+"/changes does",
			"%s cannot be watched", call.Method.Name)
		return
	}
	if f.CreatedIDs {
		c.errorf(WatchesMember, "",
			"a watching query cannot carry creation ids: they belong to one request, and a watch makes many")
	}
	if f.Returns != "" && f.Returns != f.Watches {
		c.errorf(ReturnsMember, "leave "+ReturnsMember+" out, or name the watched call",
			"a watching query cannot return only %q, since the loop reads the state it goes on from out of the %q response",
			f.Returns, f.Watches)
	}

	since, given := call.Args.Find(SinceStateArgument)
	switch value := since.(type) {
	case *ParamRef:
		q.WatchState = value.Param
	default:
		_ = value
		where := fmt.Sprintf("%s.arguments.%s", callPath(q, call), SinceStateArgument)
		if !given {
			where = WatchesMember
		}
		c.errorf(where, `write "`+SinceStateArgument+`": "{{sinceState}}"`,
			"the %s of a watched call is the state the loop has reached, so it has to be a parameter", SinceStateArgument)
	}
	if _, referenced := call.Args.Find("#" + AccountIDArgument); referenced {
		c.errorf(WatchesMember, "",
			"the account of a watched call comes from an earlier call, and a watch has to know whose events to listen for before it makes any")
	}
	q.Watches = call
}

// pages checks the call a query says a pager advances, and records what the
// generated loop needs: which parameter says where the next request starts,
// and how the answer says where that is.
//
// Two kinds of call return part of an answer and say where the rest is. A
// /query returns a window of a longer list, and the next window starts after
// this one. A /changes reports what changed since a state, and answers with as
// much as it cares to, saying so.
func (c *checker) pages(q *Query, f fileSyntax) {
	if f.Pages == "" {
		return
	}
	call, ok := c.byID[f.Pages]
	if !ok {
		c.errorf(PagesMember, "", "no method call has the id %q", f.Pages).
			hint(hintFor(f.Pages, c.callIDs()))
		return
	}

	var start string
	switch {
	case c.returnsWindow(call):
		q.PageKind, start = PageQuery, PositionArgument
	case c.reportsChanges(call):
		q.PageKind, start = PageChanges, SinceStateArgument
	default:
		c.errorf(PagesMember,
			"a paged call returns a window of a longer list, as "+call.Method.DataType+"/query does, or what changed since a state, as "+call.Method.DataType+"/changes does",
			"%s cannot be paged", call.Method.Name)
		return
	}
	if f.Watches != "" {
		c.errorf(PagesMember, "",
			"a watching query already asks again while the server says there is more, so it does not also take %s", PagesMember)
	}
	if f.CreatedIDs {
		c.errorf(PagesMember, "",
			"a paged query cannot carry creation ids: they belong to one request, and a pager makes many")
	}
	if f.Returns != "" && f.Returns != f.Pages {
		c.errorf(ReturnsMember, "leave "+ReturnsMember+" out, or name the paged call",
			"a paged query cannot return only %q, since the loop reads where the next request starts out of the %q response",
			f.Returns, f.Pages)
	}

	value, given := call.Args.Find(start)
	if param, ok := value.(*ParamRef); ok {
		q.PageStart = param.Param
	} else {
		where := fmt.Sprintf("%s.arguments.%s", callPath(q, call), start)
		if !given {
			where = PagesMember
		}
		c.errorf(where, `write "`+start+`": "{{`+start+`}}"`,
			"the %s of a paged call is where the next request starts, so it has to be a parameter", start)
	}
	q.Pages = call
}

// returnsWindow reports whether a call answers with one window of a longer
// list and says where that window sits, which is what a pager walks. As with a
// /changes, the data model says so rather than the method name: a vendor
// extension declaring the standard methods for a type of its own is paged
// exactly as Email is.
func (c *checker) returnsWindow(call *Call) bool {
	if call.Method.DataType == "" {
		return false
	}
	args, err := c.spec.ArgumentsOf(call.Method.Name)
	if err != nil {
		return false
	}
	if _, ok := args.Field(PositionArgument); !ok {
		return false
	}
	resp, err := c.spec.ResponseOf(call.Method.Name)
	if err != nil {
		return false
	}
	_, hasPosition := resp.Field(PositionArgument)
	_, hasIDs := resp.Field(IDsProperty)
	return hasPosition && hasIDs
}

// reportsChanges reports whether a call answers with what changed since a
// state, which is what a loop can go on from. The data model says so rather
// than the method name: a vendor extension declaring the standard methods for
// a type of its own can be watched exactly as Email can.
func (c *checker) reportsChanges(call *Call) bool {
	if call.Method.DataType == "" {
		return false
	}
	args, err := c.spec.ArgumentsOf(call.Method.Name)
	if err != nil {
		return false
	}
	if _, ok := args.Field(SinceStateArgument); !ok {
		return false
	}
	resp, err := c.spec.ResponseOf(call.Method.Name)
	if err != nil {
		return false
	}
	_, hasState := resp.Field(NewStateProperty)
	_, hasMore := resp.Field(HasMoreChangesProperty)
	return hasState && hasMore
}

// callPath names a call the way a diagnostic about its arguments does.
func callPath(q *Query, call *Call) string {
	for i, c := range q.Calls {
		if c == call {
			return fmt.Sprintf("methodCalls[%d]", i)
		}
	}
	return "methodCalls"
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

	// enum holds the values the property being checked may take, for one whose
	// specification fixes them. It travels with the property so that it reaches
	// the elements of an array and the keys of a set.
	enum []string

	// argumentValue says that the value about to be checked is a whole
	// argument of a method call, which is the one place a parameter may be
	// left out: leaving it out takes the argument with it. It is cleared as
	// soon as the value is reached, so that nothing nested inside inherits it.
	argumentValue bool

	// used collects the capabilities the query turned out to need beyond those
	// its methods imply, for properties that belong to a specification other
	// than their type's own.
	used map[string]bool
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
	call.NestedProperties = c.nestedProperties(call, where+".arguments")
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

		if name == CommentArgument {
			comment, ok := stringValue(members[key])
			if !ok {
				c.errorf(where+"."+key, "", "%s must be a string", CommentArgument)
				continue
			}
			call.Comment = comment
			continue
		}

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
		savedPatch, savedSort, savedEnum := c.patchTarget, c.sortTarget, c.enum
		if field.PatchTarget != "" {
			c.patchTarget = field.PatchTarget
		}
		if field.SortTarget != "" {
			c.sortTarget = field.SortTarget
		}
		c.enum = field.Enum
		c.useCapability(field)
		c.argumentValue = true
		node := c.value(field.ParsedType(), members[key], where+"."+key, field.Doc)
		c.patchTarget, c.sortTarget, c.enum = savedPatch, savedSort, savedEnum
		if ref, isParam := node.(*ParamRef); isParam && ref.Param.Optional && name == AccountIDArgument {
			c.errorf(where+"."+key, "leave "+AccountIDArgument+" out altogether, and it is filled in from the primary account",
				"%s cannot be left out on its own, since a method call is made against an account",
				AccountIDArgument)
		}
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
		selected, known := dataType.Field(name)
		if !known {
			where := fmt.Sprintf("%s.%s[%d]", where, call.Method.PropertiesArgument, i)
			header, err := spec.ParseHeaderProperty(name)
			switch {
			case err != nil:
				var badForm *spec.HeaderPropertyError
				hint := ""
				if errors.As(err, &badForm) && len(badForm.Forms) > 0 {
					hint = hintFor(badForm.Property, badForm.Forms)
					if hint == "" {
						hint = "the parsed forms are " + strings.Join(badForm.Forms, ", ")
					}
				}
				c.errorf(where, hint, "%v", err)
				continue
			case header != nil:
				// A property naming one header field of the message. Its type
				// comes from the form asked for, so it is not a member of the
				// data type and cannot be checked against one.
			case !isDynamicProperty(name):
				c.errorf(where, hintFor(name, dataType.PropertyNames()),
					"%s has no property %q", dataType.Name, name)
				continue
			}
		}
		c.useCapability(selected)
		props = append(props, name)
	}
	return props
}

// nestedProperties extracts the property names selected for a type nested
// inside the records, and checks each one against that type. bodyProperties is
// the only such argument: it narrows the body parts of an Email rather than the
// Email itself.
func (c *checker) nestedProperties(call *Call, where string) []string {
	if call.Method.NestedPropertiesArgument == "" || call.Args == nil {
		return nil
	}
	node, ok := call.Args.Find(call.Method.NestedPropertiesArgument)
	if !ok {
		return nil
	}
	arr, ok := node.(*Array)
	if !ok {
		return nil
	}
	nested, ok := c.spec.Object(call.Method.NestedType)
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
		if _, known := nested.Field(name); !known {
			c.errorf(fmt.Sprintf("%s.%s[%d]", where, call.Method.NestedPropertiesArgument, i),
				hintFor(name, nested.PropertyNames()),
				"%s has no property %q", nested.Name, name)
			continue
		}
		props = append(props, name)
	}
	return props
}

// isDynamicProperty reports whether a property name is one the server gives
// meaning to rather than one the data model fixes.
func isDynamicProperty(name string) bool {
	switch {
	case strings.HasPrefix(name, "header:"):
		// The header field forms of RFC 8621, Section 4.1.3.
		return true
	case strings.HasPrefix(name, "digest:"):
		// A digest in whatever algorithm the session says it supports,
		// RFC 9404, Section 4.2.
		return true
	case name == "data":
		// RFC 9404 asks the server to return the octets as text or as base64,
		// whichever fits, so what comes back is one of those two properties.
		return true
	}
	return false
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
	for uri := range c.used {
		needed[uri] = true
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
	for _, uri := range sortedKeys(c.used) {
		if !have[uri] {
			c.errorf("using", fmt.Sprintf("add %q to using", uri),
				"the query uses properties from %s, which it does not declare", uri)
			have[uri] = true
		}
	}
	return out
}

// useCapability records that the query touched a property belonging to a
// specification other than its type's own.
func (c *checker) useCapability(f *spec.Field) {
	if f != nil && f.Capability != "" {
		c.used[f.Capability] = true
	}
}

// sortedKeys returns a map's keys in a stable order, so that two runs report
// the same thing.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
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
