package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/linyows/jmapc/internal/spec"
)

// PropertiesName is the file, beside the requests, that names the sets of
// properties they select. It is not a JMAP request, so it does not carry the
// request extension: a file ending in .jmap.json holds a Request object as RFC
// 8620 defines it, and this one holds jmapc's own declarations.
const PropertiesName = "properties.json"

// setPattern matches a reference to a named set of properties. The "@" is not
// "{{": a parameter is filled in by the caller at run time, and a set is
// resolved while the code is generated, so a reader can tell which of the two
// a value is without knowing the names.
//
// A property name cannot begin with "@", so nothing a request may legitimately
// select is taken for a reference.
var setPattern = regexp.MustCompile(`^@([A-Za-z_][A-Za-z0-9_]*)$`)

// PropertySet is one named set of properties: a shape a request asks for, and
// the name the generated type for it goes by.
//
// A request that spells its properties out gets a type named after the call
// that asked, so the same fourteen properties read by six requests become six
// types that a caller has to convert between. Naming the set names the type,
// and the six requests answer with one.
type PropertySet struct {
	// Name is what the set is called, which is the name of the generated type.
	Name string
	// Doc is what the author said the set is for, carried into the generated
	// type's comment.
	Doc string
	// Type is the JMAP data type the set narrows, such as "Email".
	Type string
	// Extends is the set this one adds to, or nil where it stands alone. The
	// generated type embeds the one it extends, so a function written for the
	// base takes the derived without converting.
	Extends *PropertySet
	// Own are the properties declared here, in the order they were written,
	// without those the extended set already holds.
	Own []string
}

// Properties returns every property the set holds: those of the set it
// extends, then its own, in declaration order. That is the order the generated
// fields are written in.
func (s *PropertySet) Properties() []string {
	if s.Extends == nil {
		return s.Own
	}
	base := s.Extends.Properties()
	out := make([]string, 0, len(base)+len(s.Own))
	return append(append(out, base...), s.Own...)
}

// PropertySets is the file of named property sets, in the order they were
// declared.
type PropertySets struct {
	// Path is the file the sets were read from.
	Path string
	// Sets are the sets, in declaration order.
	Sets []*PropertySet

	byName map[string]*PropertySet
}

// Find returns the set with the given name.
func (p *PropertySets) Find(name string) (*PropertySet, bool) {
	if p == nil {
		return nil, false
	}
	s, ok := p.byName[name]
	return s, ok
}

// Names returns the names of the declared sets, in declaration order.
func (p *PropertySets) Names() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Sets))
	for _, s := range p.Sets {
		out = append(out, s.Name)
	}
	return out
}

// LoadPropertySets reads the property sets at path, checking them against the
// catalogue. A missing file is not an error: a project that names no set has
// none.
func LoadPropertySets(path string, catalogue *spec.Spec) (*PropertySets, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParsePropertySets(path, src, catalogue)
}

// setSyntax is the shape of one entry in the property sets file. Nothing here
// is JMAP's, so the members are named plainly rather than with the leading
// underscore a request file uses to tell jmapc's members from the
// specification's.
type setSyntax struct {
	// Doc says what the set is for, and becomes the generated type's comment.
	Doc string `json:"doc"`
	// Type is the JMAP data type the properties are selected from. It may be
	// left out where the set extends another, which fixes it already.
	Type string `json:"type"`
	// Extends names the set this one adds to.
	Extends string `json:"extends"`
	// Properties are the property names the set holds, in the order the
	// generated fields are written in.
	Properties []string `json:"properties"`
}

// ParsePropertySets checks the property sets in src, which came from the file
// at path. Like a request, it reports every problem it finds rather than
// stopping at the first.
func ParsePropertySets(path string, src []byte, catalogue *spec.Spec) (*PropertySets, error) {
	if kind := jsonKind(json.RawMessage(src)); kind != "an object" {
		return nil, ErrorList{{File: path,
			Msg:  fmt.Sprintf("expected an object naming one set of properties per member, found %s", kind),
			Hint: `a file of sets is written as {"EmailSummary": {"type": "Email", "properties": [...]}}`}}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(src, &members); err != nil {
		return nil, ErrorList{{File: path, Msg: syntaxMessage(err)}}
	}
	// The keys are read a second time to keep the order they were written in,
	// which a map does not hold and which the generated file follows.
	keys, err := objectKeys(json.RawMessage(src))
	if err != nil {
		return nil, ErrorList{{File: path, Msg: syntaxMessage(err)}}
	}

	p := &PropertySets{Path: path, byName: make(map[string]*PropertySet, len(keys))}
	var errs ErrorList
	fail := func(where, hint, format string, args ...any) {
		errs = append(errs, &Error{File: path, Where: where, Msg: fmt.Sprintf(format, args...), Hint: hint})
	}

	// The sets are read in two passes: every name is known before any
	// "extends" is resolved, so that a set may extend one declared after it and
	// the order in the file stays the author's to choose.
	syntax := make(map[string]*setSyntax, len(keys))
	for _, key := range keys {
		if key == SchemaMember {
			continue
		}
		if !isSetName(key) {
			fail(key, "a set is named after the type to generate, as in EmailSummary",
				"%q is not a name a generated type can take", key)
			continue
		}
		var s setSyntax
		dec := json.NewDecoder(bytes.NewReader(members[key]))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			fail(key, `a set is written as {"type": ..., "properties": [...]}`, "%s", syntaxMessage(err))
			continue
		}
		syntax[key] = &s
		set := &PropertySet{Name: key, Doc: s.Doc}
		p.Sets = append(p.Sets, set)
		p.byName[key] = set
	}

	for _, set := range p.Sets {
		p.resolve(set, syntax[set.Name], catalogue, fail)
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return p, nil
}

// resolve settles one set: which set it extends, which data type it narrows,
// and which properties it holds.
func (p *PropertySets) resolve(set *PropertySet, s *setSyntax, catalogue *spec.Spec, fail func(where, hint, format string, args ...any)) {
	if s.Extends != "" {
		base, known := p.byName[s.Extends]
		switch {
		case !known:
			fail(set.Name+".extends", hintFor(s.Extends, p.Names()), "no set is named %q", s.Extends)
		case base == set:
			fail(set.Name+".extends", "", "a set cannot extend itself")
		default:
			set.Extends = base
		}
	}
	if cycle := extendsCycle(set); cycle != "" {
		fail(set.Name+".extends", "", "the sets extend one another in a circle: %s", cycle)
		set.Extends = nil
	}

	set.Type = s.Type
	if set.Extends != nil {
		switch {
		case s.Type == "":
			set.Type = baseType(set.Extends)
		case s.Type != baseType(set.Extends):
			fail(set.Name+".type", "leave the type out, and it is the one the extended set narrows",
				"%s narrows %s, but the set it extends narrows %s", set.Name, s.Type, baseType(set.Extends))
			set.Type = baseType(set.Extends)
		}
	}
	if set.Type == "" {
		fail(set.Name, `name the type the properties are selected from, as {"type": "Email", ...}`,
			"%s says nothing about which type its properties belong to", set.Name)
		return
	}
	dataType, known := catalogue.Object(set.Type)
	if !known {
		fail(set.Name+".type", hintFor(set.Type, objectNames(catalogue)), "unknown type %q", set.Type)
		return
	}

	if len(s.Properties) == 0 {
		fail(set.Name+".properties", "", "%s selects no properties", set.Name)
		return
	}
	held := make(map[string]bool)
	for _, name := range inherited(set.Extends) {
		held[name] = true
	}
	for i, name := range s.Properties {
		where := fmt.Sprintf("%s.properties[%d]", set.Name, i)
		if held[name] {
			// Naming a property twice would write the field twice, and the
			// second is either a mistake or a misreading of what the extended
			// set already holds.
			fail(where, "", "%q is selected twice", name)
			continue
		}
		if _, hint, err := checkProperty(dataType, name); err != nil {
			fail(where, hint, "%v", err)
			continue
		}
		held[name] = true
		set.Own = append(set.Own, name)
	}
}

// inherited returns every property the extended set holds, and nothing where
// the set extends nothing.
func inherited(base *PropertySet) []string {
	if base == nil {
		return nil
	}
	return base.Properties()
}

// baseType returns the data type a set narrows, following what it extends.
func baseType(set *PropertySet) string {
	for set.Extends != nil {
		set = set.Extends
	}
	return set.Type
}

// extendsCycle returns the circle a set sits in, written out, or an empty
// string where it sits in none.
func extendsCycle(set *PropertySet) string {
	seen := map[*PropertySet]bool{set: true}
	names := []string{set.Name}
	for cur := set.Extends; cur != nil; cur = cur.Extends {
		names = append(names, cur.Name)
		if seen[cur] {
			return strings.Join(names, " extends ")
		}
		seen[cur] = true
	}
	return ""
}

// isSetName reports whether a name can be the name of a generated type. It is
// the rule a request file name is held to, with the leading capital a type
// needs to be exported in every language jmapc writes.
func isSetName(name string) bool {
	return isGoIdentifier(name) && name[0] >= 'A' && name[0] <= 'Z'
}

// objectNames returns the names of every type in the catalogue, for a hint.
func objectNames(catalogue *spec.Spec) []string {
	objects := catalogue.Objects()
	out := make([]string, 0, len(objects))
	for _, o := range objects {
		out = append(out, o.Name)
	}
	return out
}

// checkProperty reports what is wrong with selecting a property of a data
// type, and returns the field the data model has for it. A property the server
// gives meaning to rather than the data model — a header field, a digest — is
// not a member of the type, and comes back with no field and no error.
func checkProperty(dataType *spec.Object, name string) (*spec.Field, string, error) {
	field, known := dataType.Field(name)
	if known {
		return field, "", nil
	}
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
		return nil, hint, err
	case header != nil:
		// A property naming one header field of the message. Its type comes
		// from the form asked for, so it is not a member of the data type and
		// cannot be checked against one.
		return nil, "", nil
	case isDynamicProperty(name):
		return nil, "", nil
	}
	return nil, hintFor(name, dataType.PropertyNames()),
		fmt.Errorf("%s has no property %q", dataType.Name, name)
}
