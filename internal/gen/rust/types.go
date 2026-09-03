// Package rust writes Rust from the JMAP data model and the queries checked
// against it. What it produces asks for serde and serde_json and nothing else:
// how the bytes reach the server is a Transport the caller supplies, so the
// generated code brings no HTTP stack, no TLS backend and no async runtime
// along with it.
package rust

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/spec"
)

// TypeGenerator writes the Rust declarations for the object types in a
// catalogue.
type TypeGenerator struct {
	// Spec is the catalogue to generate from.
	Spec *spec.Spec
	// Skip names object types to leave out.
	Skip map[string]bool
}

// Generate returns the source of types.rs.
func (g *TypeGenerator) Generate() ([]byte, error) {
	var buf bytes.Buffer
	writeHeader(&buf, "the JMAP data model in internal/spec")
	buf.WriteString("use std::collections::BTreeMap;\n\n")
	buf.WriteString("use serde::{Deserialize, Serialize};\n\n")

	shared.WriteComment(&buf, "", "The primitive types that carry a format rather than a shape. "+
		"Each is a String underneath, and named so that the format is visible in a signature and "+
		"a reader can see what a bare string is standing for.")
	buf.WriteString("\n")
	for _, alias := range spec.RustPrimitiveAliases() {
		writeDoc(&buf, "", alias.Doc)
		fmt.Fprintf(&buf, "pub type %s = String;\n\n", alias.Name)
	}

	writeDoc(&buf, "", "A set of changes to apply to a record, keyed by JSON pointer into it. "+
		"A null value removes what the pointer names.")
	buf.WriteString("pub type PatchObject = BTreeMap<String, serde_json::Value>;\n\n")

	objects := g.objects()
	g.writeUnions(&buf, objects)
	for _, o := range objects {
		g.writeObject(&buf, o)
	}
	return finish(&buf), nil
}

// objects returns the types to write, in catalogue order.
func (g *TypeGenerator) objects() []*spec.Object {
	var out []*spec.Object
	for _, o := range g.Spec.Objects() {
		if g.Skip[o.Name] {
			continue
		}
		out = append(out, o)
	}
	return out
}

// writeUnions writes an enum for every union of shapes the catalogue uses. Rust
// has no anonymous union, so each one is given a name built from its members
// and written once, however many properties hold it.
func (g *TypeGenerator) writeUnions(buf *bytes.Buffer, objects []*spec.Object) {
	unions := map[string]*spec.Type{}
	for _, o := range objects {
		for _, f := range o.Fields {
			spec.CollectUnions(f.ParsedType(), unions)
		}
	}
	names := spec.SortedUnionNames(unions)
	if len(names) == 0 {
		return
	}
	shared.WriteComment(buf, "", "The unions the data model uses, where a property holds either "+
		"of two shapes. Serde tries the alternatives in turn and keeps the first that fits, which "+
		"is what tells them apart: a shape is known by the properties it requires.")
	buf.WriteString("\n")
	for _, name := range names {
		g.writeUnion(buf, name, unions[name])
	}
}

// writeUnion writes one union enum, and the Default that lets a struct holding
// one derive its own.
func (g *TypeGenerator) writeUnion(buf *bytes.Buffer, name string, t *spec.Type) {
	writeDoc(buf, "", "A value that is "+describeUnion(t)+".")
	buf.WriteString("#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]\n")
	buf.WriteString("#[serde(untagged)]\n")
	fmt.Fprintf(buf, "pub enum %s {\n", name)
	for _, m := range t.Union {
		fmt.Fprintf(buf, "    %s(%s),\n", unionVariant(m), m.RustType())
	}
	buf.WriteString("}\n\n")

	fmt.Fprintf(buf, "impl Default for %s {\n", name)
	fmt.Fprintf(buf, "    fn default() -> Self {\n")
	fmt.Fprintf(buf, "        Self::%s(Default::default())\n", unionVariant(t.Union[0]))
	buf.WriteString("    }\n}\n\n")
}

// unionVariant names one alternative of a union enum.
func unionVariant(t *spec.Type) string {
	return spec.RustUnionName(&spec.Type{Union: []*spec.Type{t}})
}

// describeUnion renders a union for prose.
func describeUnion(t *spec.Type) string {
	names := make([]string, len(t.Union))
	for i, m := range t.Union {
		names[i] = article(m.RustType()) + " " + m.RustType()
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// article picks the indefinite article a name takes, by the letter it starts
// with, which is as far as generated prose needs to go.
func article(name string) string {
	if name == "" {
		return "a"
	}
	if strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an"
	}
	return "a"
}

// writeObject writes the struct for one object type.
func (g *TypeGenerator) writeObject(buf *bytes.Buffer, o *spec.Object) {
	doc := o.Doc
	if o.Capability != "" && o.Capability != spec.CapabilityCore {
		doc += "\n\nA request using this type must declare " + o.Capability + "."
	}
	writeDoc(buf, "", doc)
	writeDerive(buf, o.Kind != spec.KindResponse)
	buf.WriteString("#[serde(rename_all = \"camelCase\")]\n")
	if len(o.Fields) == 0 {
		fmt.Fprintf(buf, "pub struct %s {}\n\n", spec.RustTypeName(o.Name))
		return
	}
	fmt.Fprintf(buf, "pub struct %s {\n", spec.RustTypeName(o.Name))
	for i, f := range o.Fields {
		if i > 0 {
			buf.WriteString("\n")
		}
		g.writeField(buf, o, f)
	}
	buf.WriteString("}\n\n")
}

// writeDerive writes the derives every generated struct carries. A struct the
// caller builds also derives Default, so that a record with fifty optional
// properties can be written as the two that matter and a rest.
func writeDerive(buf *bytes.Buffer, withDefault bool) {
	if withDefault {
		buf.WriteString("#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]\n")
		return
	}
	buf.WriteString("#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]\n")
}

// writeField writes one field of a struct. Everything a client may leave out is
// an Option that is skipped when it is None, which is what Rust says with what
// a Go omitzero tag says.
func (g *TypeGenerator) writeField(buf *bytes.Buffer, o *spec.Object, f *spec.Field) {
	doc := f.Doc
	switch {
	case f.ServerSet && o.Kind == spec.KindData:
		doc = strings.TrimSpace(doc + "\n\nThe server sets this property; it may not be set by the client.")
	case f.Immutable && o.Kind == spec.KindData:
		doc = strings.TrimSpace(doc + "\n\nThis property may be set when the record is created, but not changed afterwards.")
	}
	if f.Default != "" {
		doc = strings.TrimSpace(doc + "\n\nThe server assumes " + f.Default + " when this property is omitted.")
	}
	if f.Capability != "" && f.Capability != o.Capability {
		doc = strings.TrimSpace(doc + "\n\nA request using this property must declare " + f.Capability + ".")
	}
	writeDoc(buf, "    ", doc)
	writeMember(buf, o.Name, f.Name, f.ParsedType(), omittable(o, f), o.Kind == spec.KindResponse)
}

// omittable reports whether a property may be left out of the value entirely,
// which every property of a record or an argument object may be unless the
// specification requires it. A required property is what tells one member of a
// union from another, so it is left as it is rather than wrapped in an Option.
func omittable(o *spec.Object, f *spec.Field) bool {
	return o.Kind != spec.KindResponse && !f.Required
}

// writeMember writes one field, with the serde attributes its name and its
// optionality call for.
func writeMember(buf *bytes.Buffer, owner, wireName string, t *spec.Type, optional, lenient bool) {
	ident := spec.RustFieldName(wireName)
	rendered := t.RustType()
	// A property whose type is the type it belongs to would make the struct
	// infinitely large, so it is held behind a Box. Going through a Vec or a
	// BTreeMap already puts it behind a pointer, and needs nothing.
	if t.Name == owner {
		rendered = boxed(rendered)
	}
	if optional && !strings.HasPrefix(rendered, "Option<") {
		rendered = "Option<" + rendered + ">"
	}

	var attrs []string
	if rename := spec.SerdeRename(wireName, ident); rename != "" {
		attrs = append(attrs, fmt.Sprintf("rename = %q", rename))
	}
	switch {
	case optional:
		attrs = append(attrs, "default", `skip_serializing_if = "Option::is_none"`)
	case lenient && hasEmptyForm(rendered):
		// A response is written by the server, so a property it says it always
		// sends is not an Option. Where the empty value says the same thing as
		// the property being absent, though, a server that leaves it out is
		// better read than rejected.
		attrs = append(attrs, "default")
	}
	writeSerdeAttr(buf, "    ", attrs)
	fmt.Fprintf(buf, "    pub %s: %s,\n", ident, rendered)
}

// writeSerdeAttr writes a serde attribute, on one line where it fits and one
// item to a line where it does not, as rustfmt would lay it out.
func writeSerdeAttr(buf *bytes.Buffer, indent string, attrs []string) {
	if len(attrs) == 0 {
		return
	}
	joined := strings.Join(attrs, ", ")
	if len(joined) <= attrWidth {
		buf.WriteString(indent + "#[serde(" + joined + ")]\n")
		return
	}
	buf.WriteString(indent + "#[serde(\n")
	for i, attr := range attrs {
		comma := ","
		if i == len(attrs)-1 {
			comma = ""
		}
		fmt.Fprintf(buf, "%s    %s%s\n", indent, attr, comma)
	}
	buf.WriteString(indent + ")]\n")
}

// boxed wraps a rendered type in a Box, reaching inside an Option so that the
// None stays where it was.
func boxed(rendered string) string {
	if inner, ok := strings.CutPrefix(rendered, "Option<"); ok {
		return "Option<Box<" + strings.TrimSuffix(inner, ">") + ">>"
	}
	return "Box<" + rendered + ">"
}

// hasEmptyForm reports whether a rendered type has an empty value that means
// what an absent property means.
func hasEmptyForm(rendered string) bool {
	return strings.HasPrefix(rendered, "Option<") ||
		strings.HasPrefix(rendered, "Vec<") ||
		strings.HasPrefix(rendered, "BTreeMap<")
}

// writeDoc writes text as a Rust documentation comment.
func writeDoc(buf *bytes.Buffer, indent, text string) {
	shared.WriteCommentMarker(buf, indent, "///", text)
}

// finish returns what has been written, ending in exactly one newline: the
// blank line each item is separated by would otherwise trail off the end of the
// file, which rustfmt takes off again.
func finish(buf *bytes.Buffer) []byte {
	return append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
}

// writeHeader writes the banner every generated file carries.
func writeHeader(buf *bytes.Buffer, source string) {
	buf.WriteString("// Code generated by jmapc. DO NOT EDIT.\n")
	fmt.Fprintf(buf, "// Source: %s\n\n", strings.ReplaceAll(source, `\`, "/"))
}
