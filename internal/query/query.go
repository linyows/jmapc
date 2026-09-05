// Package query parses the JMAP queries a user writes and checks them against
// the JMAP data model, so that a mistake in a query is reported where it was
// written rather than by the server at run time.
package query

import (
	"encoding/json"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/spec"
)

// Query is one parsed and checked query file: a single JMAP request, with the
// parameters its author left open.
type Query struct {
	// Name is the query's name, taken from the file name, and becomes the name
	// of the generated function.
	Name string
	// Path is the file the query was read from.
	Path string
	// Shape is the query with its names taken off, so that two queries that
	// differ only in what they call things can be told apart from two that
	// ask for different things. See shapeOf.
	Shape string
	// Doc is the query's documentation, carried into the generated function.
	Doc string
	// Using lists the capability URIs the request declares.
	Using []string
	// Calls are the method calls, in the order the server will run them.
	Calls []*Call
	// Params are the parameters the query takes, in the order they first
	// appear.
	Params []*Param
	// Creations are the creation ids the query invents, in the order they were
	// written. A record created under one of these is reported back under the
	// same name, so a generator gives the name to the caller rather than
	// leaving it to be spelled again.
	Creations []string
	// Returns is the call whose result the generated function returns, or nil
	// when it returns all of them.
	Returns *Call
	// Watches is the call whose state a generated watch follows, or nil where
	// the query is not watched. A watch calls the query whenever the server
	// reports that call's type has changed.
	Watches *Call
	// WatchState is the parameter carrying the state a watched call reports
	// the changes since. The loop fills it in, from the state the last answer
	// left it at.
	WatchState *Param
	// Pages is the call a generated pager advances, or nil where the query
	// asked for no pager. One request answers with one window of a longer
	// list, and the pager asks for the next until there is none.
	Pages *Call
	// PageStart is the parameter saying where the next request starts, which
	// the pager fills in from the answer before it.
	PageStart *Param
	// PageKind says how that is worked out.
	PageKind PageKind
	// CreatedIDs reports whether the generated function carries the creation
	// ids of a request in and out, which is what lets a proxy split one
	// request across several and have the references still resolve.
	CreatedIDs bool
}

// Call is one method call within a query.
type Call struct {
	// ID is the call id the query gave it, which back references point at.
	ID string
	// Method is the catalogue entry for the method being called.
	Method *spec.Method
	// Args is the argument object as written, with parameters and back
	// references resolved.
	Args *Object
	// Properties are the property names a /get call selects, when the query
	// states them literally. It is nil when the call fetches every property.
	Properties []string
	// NestedProperties are the property names selected for a type nested
	// inside the records, as bodyProperties selects them for the body parts of
	// an Email. It is nil when the call narrows nothing.
	NestedProperties []string
	// Comment is what the query said this call is for, carried into the
	// generated code. It comes from the _comment member of the arguments,
	// which never reaches the server.
	Comment string
	// Field is the name this call's result takes among the results, before any
	// language has spelled it. A generator turns it into whatever an
	// identifier looks like in the language it writes.
	Field string
}

// The members of a /changes call the generated watch reads. A call that has
// them is one a loop can go on from, whichever specification declared it.
const (
	// SinceStateArgument is the state a /changes call reports the changes
	// since, which the loop supplies from the last answer it had.
	SinceStateArgument = "sinceState"
	// NewStateProperty is the state the changes leave the client at.
	NewStateProperty = "newState"
	// HasMoreChangesProperty says the server answered with only part of what
	// changed, and the call should be repeated from newState.
	HasMoreChangesProperty = "hasMoreChanges"
)

// The members of a /query call a generated pager reads. A call that has them
// returns one window of a longer list, and says where the window sits.
const (
	// PositionArgument is where the window the call returns starts, which the
	// pager moves on by what the last answer held.
	PositionArgument = "position"
	// IDsProperty holds the ids in the window.
	IDsProperty = "ids"
	// TotalProperty is how many records match in all, which a server reports
	// only where the call asked it to.
	TotalProperty = "total"
)

// PageKind says how a pager works the next request out from the last answer.
type PageKind int

const (
	// NotPaged is a query that asked for no pager.
	NotPaged PageKind = iota
	// PageQuery advances a /query: the window moves on by the ids that came
	// back, and the end is a window with nothing in it.
	PageQuery
	// PageChanges advances a /changes: the state moves on to the one the
	// answer reports, and the end is the server saying there is no more.
	PageChanges
)

// AccountIDArgument is the argument every standard JMAP method takes to say
// which account it applies to. A query that omits it has it filled in from the
// session's primary account.
const AccountIDArgument = "accountId"

// AccountIDCapability reports the capability whose primary account fills in
// this call's accountId, and whether one has to be filled in at all. A query
// that is not specific to an account should not have to state that in every
// call, so the answer is no only where the method takes no account, or the
// query names one itself.
func (c *Call) AccountIDCapability(s *spec.Spec) (string, bool) {
	args, err := s.ArgumentsOf(c.Method.Name)
	if err != nil {
		return "", false
	}
	if _, takesAccount := args.Field(AccountIDArgument); !takesAccount {
		return "", false
	}
	if _, given := c.Args.Find(AccountIDArgument); given {
		return "", false
	}
	if _, referenced := c.Args.Find("#" + AccountIDArgument); referenced {
		return "", false
	}
	if c.Method.Capability == "" {
		return spec.CapabilityCore, true
	}
	return c.Method.Capability, true
}

// Param is a value the query leaves open, written as "$name" where the value
// belongs.
type Param struct {
	// Name is the parameter name as written in the query, without the "$".
	Name string
	// Field is the name this parameter takes among the parameters, before any
	// language has spelled it.
	Field string
	// Type is the type the parameter must have, taken from the argument it
	// stands in for.
	Type *spec.Type
	// Where records the first place in the query the parameter appeared, for
	// use in diagnostics and in the generated documentation.
	Where string
	// Doc is the documentation of the argument the parameter stands in for.
	Doc string
	// Weak marks a parameter whose type came from a use that did not really
	// know it, such as a name embedded in a JSON pointer. The first use that
	// does know settles the type.
	Weak bool
	// Optional marks a parameter the caller may leave out, written
	// "{{name?}}". The argument it stands for is then left out of the request
	// altogether, which is a different request from one that sends null.
	//
	// Only an argument of a method call may be left out, and only where the
	// parameter standing for it is used nowhere else, so that "left out" has
	// one meaning: this member is not there.
	Optional bool
	// Places records every place in the query the parameter is used, by the
	// path each was written at. An optional parameter has exactly one.
	Places []string
}

// OptionalParam returns the parameter this argument stands for where the
// caller may leave it out, and nil everywhere else. An argument left out is
// not in the request at all, so a generator has to build the argument object
// rather than state it outright.
func (f ObjectField) OptionalParam() *Param {
	ref, isParam := f.Value.(*ParamRef)
	if !isParam || !ref.Param.Optional {
		return nil
	}
	return ref.Param
}

// HasOptionalArgs reports whether any argument of the call may be left out.
func (c *Call) HasOptionalArgs() bool {
	for _, f := range c.Args.Fields {
		if f.OptionalParam() != nil {
			return true
		}
	}
	return false
}

// ValueType returns the parameter's type with the null taken off. A parameter
// always carries a value, so a nullable argument yields the underlying type: a
// query that means null can simply write null.
func (p *Param) ValueType() *spec.Type {
	t := *p.Type
	t.Nullable = false
	return &t
}

// absentType returns the parameter's type marked nullable where the parameter
// may be left out, so that a language spells "no value" the way it already
// spells null: a pointer in Go, an Option in Rust, and nothing at all where
// the type has a form of its own for it, as a Go slice does.
func (p *Param) absentType() *spec.Type {
	t := p.ValueType()
	t.Nullable = p.Optional
	return t
}

// GoType returns the Go type of the parameter.
func (p *Param) GoType(qualifier string) string {
	return p.absentType().GoType(qualifier)
}

// RustType returns the Rust type of the parameter.
func (p *Param) RustType() string {
	return p.absentType().RustType()
}

// addPlace records a use of the parameter. The same path arriving twice is one
// use: a union tries its alternatives in turn, and the one that fits walks the
// value the others already walked.
func (p *Param) addPlace(where string) {
	for _, seen := range p.Places {
		if seen == where {
			return
		}
	}
	p.Places = append(p.Places, where)
}

// Node is one value inside an argument object: a literal, a parameter, a back
// reference, or a composite of those.
type Node interface {
	isNode()
	// HasParam reports whether the node or anything under it depends on a
	// parameter, which decides whether it can be emitted as a constant.
	HasParam() bool
}

// Literal is a JSON value the query states outright.
type Literal struct {
	// JSON is the value exactly as it was written.
	JSON json.RawMessage
}

// ParamRef stands in for a value the caller supplies.
type ParamRef struct {
	// Param is the parameter this value comes from.
	Param *Param
}

// Object is a JSON object whose members may themselves depend on parameters.
type Object struct {
	// Fields are the members, in the order they were written.
	Fields []ObjectField
	// Raw is the object exactly as it was written, which lets a subtree that
	// depends on nothing be emitted as the constant it is.
	Raw json.RawMessage
}

// ObjectField is one member of an Object.
type ObjectField struct {
	// Key is the member name as written, including the leading "#" of a back
	// reference.
	Key string
	// KeySegments is set when the member name is built from parameters, which
	// is how a /set names the record to update, or how a patch points into a
	// property keyed by id. It is nil when the name is a constant.
	KeySegments []KeySegment
	// Value is the member's value.
	Value Node
}

// KeySegment is one piece of a member name: either literal text or a parameter
// standing in for it.
type KeySegment struct {
	// Text is the literal text of the segment, empty for a parameter.
	Text string
	// Param is the parameter the segment stands for, nil for literal text.
	Param *Param
}

// Array is a JSON array whose elements may depend on parameters.
type Array struct {
	// Items are the elements, in order.
	Items []Node
	// Raw is the array exactly as it was written.
	Raw json.RawMessage
}

// ResultRef is a back reference: it takes the place of an argument and is
// filled in by the server from the result of an earlier call in the same
// request.
type ResultRef struct {
	// Argument is the argument being filled in, without the leading "#".
	Argument string
	// Ref is the reference as it goes on the wire.
	Ref jmapc.ResultReference
	// From is the call the reference reads from.
	From *Call
}

func (*Literal) isNode()   {}
func (*ParamRef) isNode()  {}
func (*Object) isNode()    {}
func (*Array) isNode()     {}
func (*ResultRef) isNode() {}

func (*Literal) HasParam() bool  { return false }
func (*ParamRef) HasParam() bool { return true }
func (*ResultRef) HasParam() bool {
	// A back reference is a constant in the request body; the server is what
	// fills it in.
	return false
}

func (o *Object) HasParam() bool {
	for _, f := range o.Fields {
		if len(f.KeySegments) > 0 || f.Value.HasParam() {
			return true
		}
	}
	return false
}

func (a *Array) HasParam() bool {
	for _, item := range a.Items {
		if item.HasParam() {
			return true
		}
	}
	return false
}

// Find returns the field with the given key.
func (o *Object) Find(key string) (Node, bool) {
	for _, f := range o.Fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}
