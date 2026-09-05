package spec

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ObjectKind says what role an object type plays, which decides how its Go
// representation treats absent values.
type ObjectKind int

const (
	// KindData is a JMAP data type such as Email, or a structure nested inside
	// one.
	KindData ObjectKind = iota
	// KindArguments is the argument object of a method call, where most
	// properties may be omitted.
	KindArguments
	// KindResponse is the response object of a method call, where the server
	// fills in every property it says it will.
	KindResponse
)

// Object is a JMAP object type: a named set of properties. Both data types
// such as Email and the argument and response objects of a method call are
// represented this way.
type Object struct {
	// Name is the type name used in type expressions, such as "Email".
	Name string
	// Doc is a comment describing the type, carried into generated code.
	Doc string
	// Fields are the properties of the object, in specification order.
	Fields []*Field
	// Capability is the URI a request must declare in "using" to name this
	// type. It is empty for types from the core specification.
	Capability string
	// Kind says whether the object is a data type, a method's arguments, or a
	// method's response.
	Kind ObjectKind
	// Sort lists the properties a /query call may sort this type by. A server
	// need not support sorting on everything it stores, and the specifications
	// say which properties it must.
	Sort []*SortProperty
}

// SortProperty is one property a /query may sort by, together with any extra
// members the comparator takes when it does.
type SortProperty struct {
	// Name is the property name as it appears in a comparator.
	Name string
	// Doc describes what sorting by it means.
	Doc string
	// Extra are the additional comparator members this property requires, such
	// as the keyword that "hasKeyword" sorts on.
	Extra []*Field
}

// SortProperty returns the sort property with the given name.
func (o *Object) SortProperty(name string) (*SortProperty, bool) {
	for _, p := range o.Sort {
		if p.Name == name {
			return p, true
		}
	}
	return nil, false
}

// SortNames returns the names of every sortable property, sorted, for use in
// diagnostics.
func (o *Object) SortNames() []string {
	names := make([]string, len(o.Sort))
	for i, p := range o.Sort {
		names[i] = p.Name
	}
	sort.Strings(names)
	return names
}

// Field is one property of an object type.
type Field struct {
	// Name is the property name as it appears on the wire.
	Name string
	// Type is the JMAP type expression for the property's value.
	Type string
	// Doc is a comment describing the property.
	Doc string
	// Required marks a property that must be present for a value to be of this
	// type at all. It is what tells one member of a union from another when a
	// value would otherwise fit either.
	Required bool
	// Capability is the URI a request must declare in order to use this
	// property, for one that a specification other than its type's own adds.
	// The S/MIME properties of an Email are the case this exists for: the type
	// belongs to JMAP for Mail, but those four properties belong to RFC 9219,
	// and a query that touches them has to say so.
	Capability string
	// Enum lists the values the property may take, for a property whose
	// specification fixes them. It is left empty where the set is open, as it
	// is for a mailbox role or a Content-Disposition, since rejecting a value
	// the server would have accepted is worse than letting a typo through.
	//
	// For a property whose type is a set — "String[Boolean]", as a
	// participant's roles are — the values are the keys of that set, not the
	// booleans they map to.
	Enum []string
	// ServerSet marks a property the server assigns and the client may not
	// include when creating or updating a record.
	ServerSet bool
	// CreationIDs marks an argument whose keys are creation ids: names the
	// query invents for the records it creates, which the rest of the request
	// refers to as "#" followed by the id, and which the response reports the
	// created records under. A caller reading one of those records back has to
	// spell the name again, so a generator gives it one in code.
	CreationIDs bool
	// Immutable marks a property that may be set on create but never changed.
	Immutable bool
	// Default records the value the server assumes when the property is
	// omitted from a method call's arguments.
	Default string
	// PatchTarget names the data type that the PatchObjects in this field
	// apply to. The type of a PatchObject says nothing about what it patches,
	// so without this a patch could not be checked at all.
	PatchTarget string
	// SortTarget names the data type whose sortable properties the Comparators
	// in this field may name. As with PatchTarget, the type of a Comparator
	// does not say what it sorts.
	SortTarget string

	parsed *Type
}

// ParsedType returns the field's type expression, parsed and cached.
func (f *Field) ParsedType() *Type {
	if f.parsed == nil {
		f.parsed = MustParseType(f.Type)
	}
	return f.parsed
}

// Field returns the property with the given name.
func (o *Object) Field(name string) (*Field, bool) {
	for _, f := range o.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return nil, false
}

// PropertyNames returns the names of every property, sorted, for use in
// diagnostics that suggest what the caller may have meant.
func (o *Object) PropertyNames() []string {
	names := make([]string, len(o.Fields))
	for i, f := range o.Fields {
		names[i] = f.Name
	}
	sort.Strings(names)
	return names
}

// Method describes one JMAP method call, tying its name to the shape of its
// arguments and of its response.
type Method struct {
	// Name is the method name, such as "Email/get".
	Name string
	// Capability is the URI a request must declare in "using" to call it.
	Capability string
	// Doc is a comment describing the method.
	Doc string
	// Arguments names the object type describing the call's arguments.
	Arguments string
	// Response names the object type describing the call's response.
	Response string
	// DataType names the data type the method operates on, such as "Email"
	// for "Email/get". It is empty for methods that are not tied to one type.
	DataType string
	// PropertiesArgument names the argument that selects a subset of the data
	// type's properties, if the method has one. Only /get does.
	PropertiesArgument string
	// ResultProperty names the response property holding the records, so that
	// a narrowed set of properties can be applied to the right field.
	ResultProperty string
	// NestedPropertiesArgument names an argument that narrows the properties
	// of a type nested inside the records rather than of the records
	// themselves, as bodyProperties does for the body parts of an Email.
	NestedPropertiesArgument string
	// NestedType names the type that argument narrows.
	NestedType string
}

// TypeNamePrefix returns the method name with its slash removed, so that
// "Email/get" becomes "EmailGet". Generated declarations for a method's
// arguments and response are named after it, in any language.
func (m *Method) TypeNamePrefix() string {
	parts := strings.Split(m.Name, "/")
	for i, p := range parts {
		parts[i] = exportedName(p)
	}
	return strings.Join(parts, "")
}

// Spec is a catalogue of the object types and methods a generator can resolve
// names against.
type Spec struct {
	objects map[string]*Object
	methods map[string]*Method
}

// New returns an empty catalogue.
func New() *Spec {
	return &Spec{
		objects: make(map[string]*Object),
		methods: make(map[string]*Method),
	}
}

// AddObject registers an object type. It panics on a duplicate name, because
// the catalogue is built from static declarations where a clash is a bug.
func (s *Spec) AddObject(o *Object) *Object {
	if _, dup := s.objects[o.Name]; dup {
		panic(fmt.Sprintf("spec: object %q registered twice", o.Name))
	}
	s.objects[o.Name] = o
	return o
}

// AddMethod registers a method.
func (s *Spec) AddMethod(m *Method) *Method {
	if _, dup := s.methods[m.Name]; dup {
		panic(fmt.Sprintf("spec: method %q registered twice", m.Name))
	}
	s.methods[m.Name] = m
	return m
}

// Object returns the object type with the given name.
func (s *Spec) Object(name string) (*Object, bool) {
	o, ok := s.objects[name]
	return o, ok
}

// Method returns the method with the given name.
func (s *Spec) Method(name string) (*Method, bool) {
	m, ok := s.methods[name]
	return m, ok
}

// Objects returns every registered object type, ordered by name.
func (s *Spec) Objects() []*Object {
	out := make([]*Object, 0, len(s.objects))
	for _, o := range s.objects {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Methods returns every registered method, ordered by name.
func (s *Spec) Methods() []*Method {
	out := make([]*Method, 0, len(s.methods))
	for _, m := range s.methods {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// MethodNames returns every registered method name, sorted, for diagnostics.
func (s *Spec) MethodNames() []string {
	names := make([]string, 0, len(s.methods))
	for name := range s.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ArgumentsOf returns the object type describing the arguments of the named
// method.
func (s *Spec) ArgumentsOf(method string) (*Object, error) {
	m, ok := s.Method(method)
	if !ok {
		return nil, fmt.Errorf("unknown method %q", method)
	}
	o, ok := s.Object(m.Arguments)
	if !ok {
		return nil, fmt.Errorf("method %q refers to unknown argument type %q", method, m.Arguments)
	}
	return o, nil
}

// ResponseOf returns the object type describing the response of the named
// method.
func (s *Spec) ResponseOf(method string) (*Object, error) {
	m, ok := s.Method(method)
	if !ok {
		return nil, fmt.Errorf("unknown method %q", method)
	}
	o, ok := s.Object(m.Response)
	if !ok {
		return nil, fmt.Errorf("method %q refers to unknown response type %q", method, m.Response)
	}
	return o, nil
}

// UnknownPropertyError says that a path named a property its type does not
// have. It carries the alternatives so that the caller can suggest one, which a
// formatted string could not.
type UnknownPropertyError struct {
	// TypeName is the type the property was looked for on.
	TypeName string
	// Property is the name that was not found.
	Property string
	// Known is every property that type does have, sorted.
	Known []string
}

func (e *UnknownPropertyError) Error() string {
	return fmt.Sprintf("%s has no property %q", e.TypeName, e.Property)
}

// ResolvePath walks a JMAP result reference pointer, as defined in RFC 8620,
// Section 3.7, over the response type of a method and reports the type of the
// value it selects. This is what lets a back reference be checked against the
// argument it feeds, instead of failing at the server.
func (s *Spec) ResolvePath(method, path string) (*Type, error) {
	resp, err := s.ResponseOf(method)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &Type{Name: resp.Name}, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path %q must start with %q", path, "/")
	}
	tokens := strings.Split(path[1:], "/")
	for i := range tokens {
		tokens[i] = unescapePointer(tokens[i])
	}
	return s.walk(&Type{Name: resp.Name}, tokens, path)
}

// walk applies the remaining pointer tokens to t.
func (s *Spec) walk(t *Type, tokens []string, path string) (*Type, error) {
	if len(tokens) == 0 {
		return t, nil
	}
	token := tokens[0]
	rest := tokens[1:]

	switch {
	case t.IsArray():
		if token == "*" {
			// A "*" maps the remainder of the pointer over the array's
			// elements and flattens one level of the result, so that
			// "/list/*/id" yields Id[] rather than Id[][].
			inner, err := s.walk(t.Elem, rest, path)
			if err != nil {
				return nil, err
			}
			if inner.IsArray() {
				return inner, nil
			}
			return &Type{Elem: inner}, nil
		}
		if _, err := strconv.Atoi(token); err != nil {
			return nil, fmt.Errorf("path %q: %q indexes an array of %s but is neither a number nor %q", path, token, t.Elem, "*")
		}
		return s.walk(t.Elem, rest, path)

	case t.IsMap():
		return s.walk(t.Value, rest, path)

	case t.IsUnion():
		return nil, fmt.Errorf("path %q: cannot select %q from union type %s", path, token, t)

	case t.IsPrimitive():
		return nil, fmt.Errorf("path %q: cannot select %q from %s", path, token, t.Name)
	}

	o, ok := s.Object(t.Name)
	if !ok {
		return nil, fmt.Errorf("path %q: unknown type %q", path, t.Name)
	}
	f, ok := o.Field(token)
	if !ok {
		return nil, &UnknownPropertyError{TypeName: o.Name, Property: token, Known: o.PropertyNames()}
	}
	return s.walk(f.ParsedType(), rest, path)
}

// unescapePointer decodes the "~1" and "~0" escapes of RFC 6901.
func unescapePointer(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	return strings.ReplaceAll(token, "~0", "~")
}

// ResolvePatch resolves the JSON pointer that keys one member of a PatchObject
// against the data type being patched, and reports the type each segment of the
// pointer selects by, along with the type of the value at the end and the
// property it belongs to, which carries its documentation and the values it is
// allowed to take.
//
// unknown marks the segments a parameter stands in for. A parameter in place of
// a property name leaves everything past it unknowable, so resolution stops
// there and the rest is Any.
func (s *Spec) ResolvePatch(dataType string, segments []string, unknown []bool) (keyTypes []*Type, value *Type, target *Field, err error) {
	keyTypes = make([]*Type, len(segments))
	cur := &Type{Name: dataType}
	anyType := &Type{Name: Any}

	// nested is what a PatchObject reached through the pointer applies to. A
	// PatchObject has no properties of its own, so without this the pointer
	// would run out of type the moment it entered one, and the overrides of a
	// recurring event — patches to the event, keyed by occurrence — could not
	// be checked past their key.
	var nested string

	for i, seg := range segments {
		seg = unescapePointer(seg)
		if cur.IsObject() && nested != "" {
			if o, ok := s.Object(cur.Name); ok && len(o.Fields) == 0 {
				cur, nested = &Type{Name: nested}, ""
			}
		}
		switch {
		case cur.Name == Any || cur.IsUnion():
			keyTypes[i] = anyType
			cur = anyType
			target = nil

		case cur.IsArray():
			keyTypes[i] = &Type{Name: UnsignedInt}
			cur = cur.Elem

		case cur.IsMap():
			keyTypes[i] = cur.Key
			cur = cur.Value

		case cur.IsObject():
			o, ok := s.Object(cur.Name)
			if !ok {
				return nil, nil, nil, fmt.Errorf("unknown type %q", cur.Name)
			}
			keyTypes[i] = &Type{Name: String}
			if unknown[i] {
				// A parameter stands where a property name belongs, so which
				// property this is cannot be known here.
				cur = anyType
				target = nil
				break
			}
			f, known := o.Field(seg)
			if !known {
				return nil, nil, nil, &UnknownPropertyError{
					TypeName: o.Name, Property: seg, Known: o.PropertyNames(),
				}
			}
			cur = f.ParsedType()
			target = f
			nested = f.PatchTarget

		default:
			return nil, nil, nil, fmt.Errorf("cannot look inside %s to reach %q", cur, seg)
		}
	}
	return keyTypes, cur, target, nil
}

// SetErrorTypeName is the type a /set response uses to report why it could not
// act on one record.
const SetErrorTypeName = "SetError"

// SetErrorFields names the response properties of a method that report
// per-record failures: notCreated, notUpdated, notDestroyed, notCopied, and
// whatever a vendor extension calls its own. A /set answers 200 while
// refusing individual records, so a caller that reads only the transport
// error sees success where there was none.
//
// The properties are found by their type rather than their name, so a schema
// that declares one is checked as the standard methods are.
func (s *Spec) SetErrorFields(method string) []string {
	resp, err := s.ResponseOf(method)
	if err != nil {
		return nil
	}
	var names []string
	for _, f := range resp.Fields {
		t, err := ParseType(f.Type)
		if err != nil {
			continue
		}
		if reportsSetErrors(t) {
			names = append(names, f.Name)
		}
	}
	return names
}

// reportsSetErrors reports whether a type is a map of SetError, allowing for
// the null the specifications write it with.
func reportsSetErrors(t *Type) bool {
	if t == nil {
		return false
	}
	for _, alt := range t.Union {
		if reportsSetErrors(alt) {
			return true
		}
	}
	return t.IsMap() && t.Value != nil && t.Value.Name == SetErrorTypeName
}
