package spec

import (
	"encoding/json"
	"fmt"
	"os"
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
}

// SchemaField is one property of a type, or one argument of a method.
type SchemaField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Doc         string `json:"doc"`
	Required    bool   `json:"required"`
	ServerSet   bool   `json:"serverSet"`
	Immutable   bool   `json:"immutable"`
	Default     string `json:"default"`
	PatchTarget string `json:"patchTarget"`
	SortTarget  string `json:"sortTarget"`
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
	claimed := map[string]string{}
	claim := func(name, by string) error {
		if _, dup := s.Object(name); dup {
			return fmt.Errorf("%s would define the type %q, which already exists", by, name)
		}
		if prev, dup := claimed[name]; dup {
			return fmt.Errorf("%s and %s both define the type %q", prev, by, name)
		}
		claimed[name] = by
		return nil
	}

	for _, t := range sc.Types {
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
		if _, dup := s.Method(m.Name); dup {
			return fmt.Errorf("the method %q already exists", m.Name)
		}
		goName := (&Method{Name: m.Name}).GoName()
		if err := claim(goName+"Arguments", "the method "+m.Name); err != nil {
			return err
		}
		if err := claim(goName+"Response", "the method "+m.Name); err != nil {
			return err
		}
	}
	return nil
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
	o := s.AddObject(&Object{
		Name:       t.Name,
		Doc:        doc,
		Capability: capabilityOr(t.Capability, sc.Capability),
		Fields:     fields,
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
	goName := (&Method{Name: m.Name}).GoName()
	args, err := schemaFields(m.Name, m.Arguments)
	if err != nil {
		return err
	}
	resp, err := schemaFields(m.Name, m.Response)
	if err != nil {
		return err
	}
	capability := capabilityOr(m.Capability, sc.Capability)
	argsType := s.AddObject(&Object{
		Name:       goName + "Arguments",
		Capability: capability,
		Kind:       KindArguments,
		Doc:        goName + "Arguments holds the arguments of the " + m.Name + " method.",
		Fields:     args,
	})
	respType := s.AddObject(&Object{
		Name:       goName + "Response",
		Capability: capability,
		Kind:       KindResponse,
		Doc:        goName + "Response holds the response to the " + m.Name + " method.",
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
			ServerSet:   f.ServerSet,
			Immutable:   f.Immutable,
			Default:     f.Default,
			PatchTarget: f.PatchTarget,
			SortTarget:  f.SortTarget,
		})
	}
	return out, nil
}

// checkReferences reports any type expression naming a type nothing defines,
// which is how a typo in a schema surfaces as a message about the schema rather
// than as a puzzling failure later.
func (s *Spec) checkReferences() error {
	for _, o := range s.Objects() {
		for _, f := range o.Fields {
			if err := s.checkTypeNames(MustParseType(f.Type), o.Name+"."+f.Name); err != nil {
				return err
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
