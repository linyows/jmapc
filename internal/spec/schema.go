package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Schema describes types and methods a server offers beyond the specifications
// jmapc knows. JMAP is meant to be extended: a server advertises a capability
// URI, and with it come types and methods of its own. A schema file states
// those in the same terms the built-in catalogue uses, so a query against a
// vendor extension is checked exactly as one against Email is.
type Schema struct {
	// Capability is the URI a request must declare to use anything in the
	// schema. Every type and method takes it unless it names its own.
	Capability string `json:"capability"`
	// Types are the object types the schema defines.
	Types []*SchemaType `json:"types"`
	// Methods are the methods the schema defines that do not follow the shape
	// of a standard one.
	Methods []*SchemaMethod `json:"methods"`
}

// SchemaType is one object type in a schema.
type SchemaType struct {
	// Name is the type name, used in type expressions.
	Name string `json:"name"`
	// Doc documents the type, and should begin with the type's name.
	Doc string `json:"doc"`
	// Capability overrides the schema's capability for this type.
	Capability string `json:"capability"`
	// Properties are the type's properties, in the order they should appear.
	Properties []*SchemaField `json:"properties"`
	// Methods lists which of the standard methods the type supports, by their
	// bare names: get, changes, set, copy, query, queryChanges.
	Methods []string `json:"methods"`
	// Sort lists the properties a /query may sort the type by.
	Sort []*SchemaSort `json:"sort"`
	// Arguments adds extra arguments to the type's standard methods, keyed by
	// method name, such as "query".
	Arguments map[string][]*SchemaField `json:"arguments"`
	// Dynamic lists the properties beyond its fields a /get may ask the type
	// for, as Object.Dynamic does: an entry ending in a colon stands for every
	// name it begins, and any other for a name of its own.
	Dynamic []string `json:"dynamic"`
}

// SchemaField is one property of a type, or one argument of a method.
type SchemaField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Doc         string   `json:"doc"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum"`
	Capability  string   `json:"capability"`
	ServerSet   bool     `json:"serverSet"`
	Immutable   bool     `json:"immutable"`
	Default     string   `json:"default"`
	PatchTarget string   `json:"patchTarget"`
	SortTarget  string   `json:"sortTarget"`
}

// SchemaSort is one sortable property of a type.
type SchemaSort struct {
	Name  string         `json:"name"`
	Doc   string         `json:"doc"`
	Extra []*SchemaField `json:"extra"`
}

// SchemaMethod is a method whose arguments and response the schema states
// outright, for one that does not follow a standard shape.
type SchemaMethod struct {
	Name       string         `json:"name"`
	Doc        string         `json:"doc"`
	Capability string         `json:"capability"`
	DataType   string         `json:"dataType"`
	Arguments  []*SchemaField `json:"arguments"`
	Response   []*SchemaField `json:"response"`
	// Properties names the argument that selects a subset of the data type's
	// properties, if the method has one.
	Properties string `json:"properties"`
	// ResultProperty names the response property holding the records.
	ResultProperty string `json:"resultProperty"`
	// ReturnsID says the method returns the id of every record it returns,
	// whatever properties it is asked for, as a standard /get does.
	ReturnsID bool `json:"returnsId"`
}

// LoadSchema reads a schema from a file.
func LoadSchema(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Schema
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sc); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &sc, nil
}

// standardMethodNames maps the bare method names a schema uses to the flags
// RegisterStandard takes.
var standardMethodNames = map[string]func(*StandardMethods){
	"get":          func(m *StandardMethods) { m.Get = true },
	"changes":      func(m *StandardMethods) { m.Changes = true },
	"set":          func(m *StandardMethods) { m.Set = true },
	"copy":         func(m *StandardMethods) { m.Copy = true },
	"query":        func(m *StandardMethods) { m.Query = true },
	"queryChanges": func(m *StandardMethods) { m.QueryChanges = true },
}

// Extend adds a schema's types and methods to the catalogue. Unlike the
// built-in registrations, which panic on a mistake because they are code, this
// reports what is wrong: a schema comes from a file someone wrote.
func (s *Spec) Extend(sc *Schema) error {
	if sc.Capability == "" {
		return fmt.Errorf("the schema does not say which capability it belongs to")
	}
	if !strings.HasPrefix(sc.Capability, "urn:") {
		return fmt.Errorf("%q is not a capability URI", sc.Capability)
	}

	if err := s.reserveNames(sc); err != nil {
		return err
	}

	// Types come first and all at once, so that they may refer to each other
	// however they like.
	for _, t := range sc.Types {
		if err := s.addSchemaType(sc, t); err != nil {
			return err
		}
	}
	for _, t := range sc.Types {
		if err := s.addSchemaMethods(sc, t); err != nil {
			return err
		}
	}
	for _, m := range sc.Methods {
		if err := s.addSchemaMethod(sc, m); err != nil {
			return err
		}
	}
	return s.checkReferences()
}

// reserveNames checks every name a schema will claim before anything is
// registered. Registering a duplicate is a programming error in the built-in
// catalogue and panics, but a schema is a file someone wrote, so a clash there
// has to come back as a message.
func (s *Spec) reserveNames(sc *Schema) error {
	// Names are compared without case. A generator writes a type's name with
	// a capital, Email and email alike, and a file system may not tell them
	// apart either, so two names differing only in case are one name.
	existing := map[string]string{}
	for _, o := range s.Objects() {
		existing[strings.ToLower(o.Name)] = o.Name
	}
	claimed := map[string]string{}
	claim := func(name, by string) error {
		if have, dup := existing[strings.ToLower(name)]; dup {
			if have == name {
				return fmt.Errorf("%s would define the type %q, which already exists", by, name)
			}
			return fmt.Errorf("%s would define the type %q, which is the type %q but for case", by, name, have)
		}
		if prev, dup := claimed[strings.ToLower(name)]; dup {
			return fmt.Errorf("%s and %s both define the type %q", prev, by, name)
		}
		claimed[strings.ToLower(name)] = by
		return nil
	}

	for _, t := range sc.Types {
		if err := checkTypeName(t.Name); err != nil {
			return err
		}
		if err := claim(t.Name, "the type "+t.Name); err != nil {
			return err
		}
		for _, method := range t.Methods {
			if _, known := standardMethodNames[method]; !known {
				return fmt.Errorf("%q is not a standard method; the standard methods are %s",
					method, strings.Join(standardMethodNameList(), ", "))
			}
			full := t.Name + "/" + method
			if _, dup := s.Method(full); dup {
				return fmt.Errorf("the method %q already exists", full)
			}
			prefix := t.Name + ExportedName(method)
			if err := claim(prefix+"Arguments", "the method "+full); err != nil {
				return err
			}
			if err := claim(prefix+"Response", "the method "+full); err != nil {
				return err
			}
		}
	}
	for _, m := range sc.Methods {
		if m.Name == "" {
			return fmt.Errorf("a method in the schema has no name")
		}
		if !methodNamePattern.MatchString(m.Name) {
			return fmt.Errorf("%q is not a method name: a method is named as Type/method, each part letters and digits starting with a letter", m.Name)
		}
		if _, dup := s.Method(m.Name); dup {
			return fmt.Errorf("the method %q already exists", m.Name)
		}
		prefix := (&Method{Name: m.Name}).TypeNamePrefix()
		if err := claim(prefix+"Arguments", "the method "+m.Name); err != nil {
			return err
		}
		if err := claim(prefix+"Response", "the method "+m.Name); err != nil {
			return err
		}
	}
	return nil
}

// typeNamePattern is what a type name a schema defines looks like: a name the
// type expressions can refer to, and every generator can write as one. It
// begins with a capital, as the types of the specifications do, which is the
// spelling every generator writes it in.
var typeNamePattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

// methodNamePattern is what a method name looks like: one slash between two
// names, each letters and digits starting with a letter.
var methodNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*/[A-Za-z][A-Za-z0-9]*$`)

// checkTypeName reports a name a schema cannot give a type of its own.
func checkTypeName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a type in the schema has no name")
	case !typeNamePattern.MatchString(name):
		return fmt.Errorf("%q is not a type name: a type is named with letters and digits, starting with a capital", name)
	}
	for _, primitive := range append(primitiveNames(), Any) {
		// A type expression naming it would mean the one JMAP has, and the
		// type the schema defines could never be referred to; one differing
		// only in case is written as that one by a generator.
		if strings.EqualFold(name, primitive) {
			return fmt.Errorf("%q is the name of a type JMAP already has", name)
		}
	}
	return nil
}

// primitiveNames returns the names of the primitive types.
func primitiveNames() []string {
	names := make([]string, 0, len(primitives))
	for name := range primitives {
		names = append(names, name)
	}
	return names
}

// addSchemaType registers one object type from a schema.
func (s *Spec) addSchemaType(sc *Schema, t *SchemaType) error {
	if t.Name == "" {
		return fmt.Errorf("a type in the schema has no name")
	}
	if _, dup := s.Object(t.Name); dup {
		return fmt.Errorf("the type %q is already defined", t.Name)
	}
	fields, err := schemaFields(t.Name, t.Properties)
	if err != nil {
		return err
	}
	doc := t.Doc
	if doc == "" {
		doc = t.Name + " is a type defined by " + capabilityOr(t.Capability, sc.Capability) + "."
	}
	for _, d := range t.Dynamic {
		if d == "" || d == ":" {
			return fmt.Errorf("%s names an empty dynamic property", t.Name)
		}
		if hasField(fields, d) {
			return fmt.Errorf("%s names %q as a dynamic property, and has a property of that name", t.Name, d)
		}
	}
	o := s.AddObject(&Object{
		Name:       t.Name,
		Doc:        doc,
		Capability: capabilityOr(t.Capability, sc.Capability),
		Fields:     fields,
		Dynamic:    t.Dynamic,
	})
	for _, sp := range t.Sort {
		if sp.Name == "" {
			return fmt.Errorf("a sort property of %q has no name", t.Name)
		}
		extra, err := schemaFields(t.Name, sp.Extra)
		if err != nil {
			return err
		}
		o.Sort = append(o.Sort, &SortProperty{Name: sp.Name, Doc: sp.Doc, Extra: extra})
	}
	return nil
}

// addSchemaMethods registers the standard methods a schema type asks for.
func (s *Spec) addSchemaMethods(sc *Schema, t *SchemaType) error {
	if len(t.Methods) == 0 {
		return nil
	}
	var methods StandardMethods
	for _, name := range t.Methods {
		set, known := standardMethodNames[name]
		if !known {
			return fmt.Errorf("%q is not a standard method; the standard methods are %s",
				name, strings.Join(standardMethodNameList(), ", "))
		}
		set(&methods)
	}
	if methods.Query || methods.QueryChanges {
		if _, ok := s.Object(t.Name + "FilterCondition"); !ok {
			return fmt.Errorf("%s supports /query, so the schema must also define %sFilterCondition",
				t.Name, t.Name)
		}
	}
	s.RegisterStandard(t.Name, capabilityOr(t.Capability, sc.Capability), methods)

	for _, suffix := range sortedArgumentKeys(t.Arguments) {
		extra := t.Arguments[suffix]
		method := t.Name + "/" + suffix
		if _, ok := s.Method(method); !ok {
			return fmt.Errorf("%s adds arguments to %s, which it does not define", t.Name, method)
		}
		fields, err := schemaFields(method, extra)
		if err != nil {
			return err
		}
		args, err := s.ArgumentsOf(method)
		if err != nil {
			return err
		}
		for _, f := range fields {
			if other := clashingField(args.Fields, f.Name); other != nil {
				return fmt.Errorf("%s already has the argument %q, and a schema adds arguments rather than redefining them",
					method, other.Name)
			}
		}
		s.AppendArguments(method, fields...)
	}
	return nil
}

// addSchemaMethod registers a method whose shape the schema states outright.
func (s *Spec) addSchemaMethod(sc *Schema, m *SchemaMethod) error {
	if m.Name == "" {
		return fmt.Errorf("a method in the schema has no name")
	}
	if _, dup := s.Method(m.Name); dup {
		return fmt.Errorf("the method %q is already defined", m.Name)
	}
	prefix := (&Method{Name: m.Name}).TypeNamePrefix()
	args, err := schemaFields(m.Name, m.Arguments)
	if err != nil {
		return err
	}
	resp, err := schemaFields(m.Name, m.Response)
	if err != nil {
		return err
	}
	if m.DataType != "" {
		if _, ok := s.Object(m.DataType); !ok {
			return fmt.Errorf("%s works on the type %q, which nothing defines", m.Name, m.DataType)
		}
	}
	if m.Properties != "" && m.DataType == "" {
		return fmt.Errorf("%s selects properties through %q, and names no dataType for them to be properties of", m.Name, m.Properties)
	}
	if m.Properties != "" && !hasField(args, m.Properties) {
		return fmt.Errorf("%s selects properties through %q, which is not one of its arguments", m.Name, m.Properties)
	}
	if m.Properties != "" && m.ResultProperty == "" {
		return fmt.Errorf("%s selects properties through %q, and names no resultProperty for the records they narrow", m.Name, m.Properties)
	}
	if m.ResultProperty != "" && !hasField(resp, m.ResultProperty) {
		return fmt.Errorf("%s returns its records in %q, which its response does not have", m.Name, m.ResultProperty)
	}
	if m.ReturnsID && (m.Properties == "" || m.ResultProperty == "") {
		return fmt.Errorf("%s says it returns the id of every record, and needs properties and resultProperty to say which argument narrows the records and where they are", m.Name)
	}
	if m.ReturnsID {
		// Properties needs a dataType, which is known to be defined by now.
		o, _ := s.Object(m.DataType)
		if _, hasID := o.Field("id"); !hasID {
			return fmt.Errorf("%s says it returns the id of every record, and %s has no id", m.Name, m.DataType)
		}
	}
	if m.ResultProperty != "" && m.DataType != "" {
		for _, f := range resp {
			if f.Name == m.ResultProperty && !holdsRecords(f.ParsedType(), m.DataType) {
				return fmt.Errorf("%s returns its records in %q, which is a %s rather than a list of %s or a map to them",
					m.Name, m.ResultProperty, f.Type, m.DataType)
			}
		}
	}
	capability := capabilityOr(m.Capability, sc.Capability)
	argsType := s.AddObject(&Object{
		Name:       prefix + "Arguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        prefix + "Arguments holds the arguments of the " + m.Name + " method.",
		Fields:     args,
	})
	respType := s.AddObject(&Object{
		Name:       prefix + "Response",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        prefix + "Response holds the response to the " + m.Name + " method.",
		Fields:     resp,
	})
	doc := m.Doc
	if doc == "" {
		doc = "Calls the " + m.Name + " method."
	}
	s.AddMethod(&Method{
		Name:               m.Name,
		Capability:         capability,
		Doc:                doc,
		Arguments:          argsType.Name,
		Response:           respType.Name,
		DataType:           m.DataType,
		PropertiesArgument: m.Properties,
		ResultProperty:     m.ResultProperty,
		ReturnsID:          m.ReturnsID,
	})
	return nil
}

// schemaFields converts a schema's field declarations, checking that each type
// expression parses. Where is used only to say what the fields belong to.
func schemaFields(where string, in []*SchemaField) ([]*Field, error) {
	out := make([]*Field, 0, len(in))
	for _, f := range in {
		if f.Name == "" {
			return nil, fmt.Errorf("a property of %s has no name", where)
		}
		if other := clashingField(out, f.Name); other != nil {
			if other.Name == f.Name {
				return nil, fmt.Errorf("%s defines %q twice", where, f.Name)
			}
			return nil, fmt.Errorf("%s defines %q and %q, which a generator writes as one name", where, other.Name, f.Name)
		}
		if f.Type == "" {
			return nil, fmt.Errorf("%s.%s has no type", where, f.Name)
		}
		if _, err := ParseType(f.Type); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", where, f.Name, err)
		}
		out = append(out, &Field{
			Name:        f.Name,
			Type:        f.Type,
			Doc:         f.Doc,
			Required:    f.Required,
			Enum:        f.Enum,
			Capability:  f.Capability,
			ServerSet:   f.ServerSet,
			Immutable:   f.Immutable,
			Default:     f.Default,
			PatchTarget: f.PatchTarget,
			SortTarget:  f.SortTarget,
		})
	}
	return out, nil
}

// clashingField returns the field of fields that name would be generated as:
// one of that name, or one a generator writes as the same identifier, as Go
// writes accountId and AccountId as AccountID and Rust both as account_id.
func clashingField(fields []*Field, name string) *Field {
	for _, f := range fields {
		if f.Name == name || exportedName(f.Name) == exportedName(name) || RustName(f.Name) == RustName(name) {
			return f
		}
	}
	return nil
}

// hasField reports whether fields holds one named name.
func hasField(fields []*Field, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

// checkReferences reports any type expression naming a type nothing defines,
// and any patch or sort aimed at one, which is how a typo in a schema surfaces
// as a message about the schema rather than as a puzzling failure later.
func (s *Spec) checkReferences() error {
	for _, o := range s.Objects() {
		for _, f := range o.Fields {
			where := o.Name + "." + f.Name
			if err := s.checkTypeNames(MustParseType(f.Type), where); err != nil {
				return err
			}
			if _, ok := s.Object(f.PatchTarget); f.PatchTarget != "" && !ok {
				return fmt.Errorf("%s patches the type %q, which nothing defines", where, f.PatchTarget)
			}
			if _, ok := s.Object(f.SortTarget); f.SortTarget != "" && !ok {
				return fmt.Errorf("%s sorts the type %q, which nothing defines", where, f.SortTarget)
			}
		}
	}
	return nil
}

// checkTypeNames walks a type expression looking for names the catalogue does
// not define.
func (s *Spec) checkTypeNames(t *Type, where string) error {
	switch {
	case t.IsArray():
		return s.checkTypeNames(t.Elem, where)
	case t.IsMap():
		if err := s.checkTypeNames(t.Key, where); err != nil {
			return err
		}
		return s.checkTypeNames(t.Value, where)
	case t.IsUnion():
		for _, m := range t.Union {
			if err := s.checkTypeNames(m, where); err != nil {
				return err
			}
		}
	case t.IsObject():
		if _, ok := s.Object(t.Name); !ok {
			return fmt.Errorf("%s refers to the type %q, which nothing defines", where, t.Name)
		}
	}
	return nil
}

// capabilityOr returns the first capability that is set.
func capabilityOr(specific, fallback string) string {
	if specific != "" {
		return specific
	}
	return fallback
}

// standardMethodNameList returns the bare standard method names, sorted.
func standardMethodNameList() []string {
	names := make([]string, 0, len(standardMethodNames))
	for name := range standardMethodNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortedArgumentKeys returns the method suffixes a schema adds arguments to, in
// a stable order, so that two runs of the generator agree.
func sortedArgumentKeys(m map[string][]*SchemaField) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// holdsRecords reports whether t is a list of records of dataType or a map to
// them, at any depth, which is the shape a generator can write narrowed records
// into.
func holdsRecords(t *Type, dataType string) bool {
	switch {
	case t.IsArray():
		return t.Elem.Name == dataType || holdsRecords(t.Elem, dataType)
	case t.IsMap():
		return t.Value.Name == dataType || holdsRecords(t.Value, dataType)
	}
	return false
}
