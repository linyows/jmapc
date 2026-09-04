package rust

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// file writes the module for one query: the types it needs and the function
// that sends it.
func (g *QueryGenerator) file(p *plan) []byte {
	var body bytes.Buffer
	g.writeParams(&body, p)
	g.writeNestedTypes(&body, p)
	g.writeRecordTypes(&body, p)
	g.writeResponseTypes(&body, p)
	g.writeResultType(&body, p)
	g.writeFunc(&body, p)
	g.writePages(&body, p)

	var buf bytes.Buffer
	writeHeader(&buf, p.q.Path)
	g.writeUses(&buf, p, body.String())
	buf.Write(body.Bytes())
	return finish(&buf)
}

// writeUses names what the module takes from the runtime and from the types
// beside it. Rust wants every name brought in by hand, and objects to one
// brought in and not used, so the candidates the data model suggests are
// narrowed to the ones the code below actually names.
//
// The order is the one rustfmt settles on: the standard library, then the
// crates, then this module's own siblings.
func (g *QueryGenerator) writeUses(buf *bytes.Buffer, p *plan, body string) {
	imports := &imports{types: map[string]bool{}}
	for _, param := range p.q.Params {
		imports.collect(param.ValueType())
	}
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" {
			// The shared response type comes from types.rs.
			imports.types[spec.RustTypeName(c.Method.Response)] = true
			continue
		}
		if resp, err := g.Spec.ResponseOf(c.Method.Name); err == nil {
			for _, f := range resp.Fields {
				if f.Name != c.Method.ResultProperty {
					imports.collect(f.ParsedType())
				}
			}
		}
		g.collectRecordTypes(c, info, imports)
	}
	if p.q.CreatedIDs {
		imports.types["Id"] = true
	}

	code := codeOf(body)
	if names(code, "BTreeMap") {
		buf.WriteString("use std::collections::BTreeMap;\n\n")
	}
	switch {
	case names(code, "Serialize") && names(code, "Deserialize"):
		buf.WriteString("use serde::{Deserialize, Serialize};\n")
	case names(code, "Deserialize"):
		buf.WriteString("use serde::Deserialize;\n")
	case names(code, "Serialize"):
		buf.WriteString("use serde::Serialize;\n")
	}
	buf.WriteString("use serde_json::json;\n\n")

	runtime := []string{"Client", "Error", "Invocation", "Request", "Transport", "decode"}
	if len(g.setErrorChecks(p)) > 0 {
		runtime = append(runtime, "SetErrors", "SetFailure", "collect_set_errors")
	}
	sortUse(runtime)
	writeUse(buf, "super::client", runtime)

	used := make([]string, 0, len(imports.types))
	for name := range imports.types {
		if names(code, name) {
			used = append(used, name)
		}
	}
	if len(used) > 0 {
		sortUse(used)
		writeUse(buf, "super::types", used)
	}
	buf.WriteString("\n")
}

// codeOf strips a generated module down to what a name has to appear in for the
// module to need it brought in: the documentation and the string literals are
// dropped, so that a type mentioned in a comment or a method name does not look
// like a use of it.
var stringLiteral = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

func codeOf(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		b.WriteString(stringLiteral.ReplaceAllString(line, `""`))
		b.WriteString("\n")
	}
	return b.String()
}

// names reports whether code uses the given identifier, as the identifier
// rather than as part of a longer one.
func names(code, ident string) bool {
	for i := 0; ; {
		j := strings.Index(code[i:], ident)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(ident)
		if !identChar(code, start-1) && !identChar(code, end) {
			return true
		}
		i = end
	}
}

// identChar reports whether the byte at i, where there is one, could be part of
// an identifier.
func identChar(code string, i int) bool {
	if i < 0 || i >= len(code) {
		return false
	}
	c := code[i]
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// sortUse puts the names of a use declaration in the order rustfmt does, which
// is what a function or a module is called before what a type is called, each
// group in alphabetical order.
func sortUse(names []string) {
	sort.Slice(names, func(i, j int) bool {
		li, lj := lowerFirst(names[i]), lowerFirst(names[j])
		if li != lj {
			return li
		}
		return names[i] < names[j]
	})
}

// lowerFirst reports whether a name begins with a lowercase letter, which in
// Rust means it names a value rather than a type.
func lowerFirst(name string) bool {
	return name != "" && name[0] >= 'a' && name[0] <= 'z'
}

// writeUse writes one use declaration, on a line of its own where it fits and
// filled across several where it does not, as rustfmt would lay it out.
func writeUse(buf *bytes.Buffer, path string, names []string) {
	if len(names) == 1 {
		fmt.Fprintf(buf, "use %s::%s;\n", path, names[0])
		return
	}
	if line := fmt.Sprintf("use %s::{%s};", path, strings.Join(names, ", ")); len(line) <= lineWidth {
		buf.WriteString(line + "\n")
		return
	}
	fmt.Fprintf(buf, "use %s::{\n", path)
	const indent = "    "
	line := indent
	for i, name := range names {
		piece := name + ","
		if len(line) > len(indent) && len(line)+1+len(piece) > lineWidth {
			buf.WriteString(strings.TrimRight(line, " ") + "\n")
			line = indent
		}
		if len(line) > len(indent) {
			line += " "
		}
		line += piece
		if i == len(names)-1 {
			buf.WriteString(strings.TrimRight(line, " ") + "\n")
		}
	}
	buf.WriteString("};\n")
}

// imports collects the types a module might have to name, which writeUses then
// narrows to the ones it does.
type imports struct {
	types map[string]bool
}

// collect adds every type in a type expression that has to be brought in.
func (i *imports) collect(t *spec.Type) {
	switch {
	case t == nil:
	case t.IsArray():
		i.collect(t.Elem)
	case t.IsMap():
		i.collect(t.Key)
		i.collect(t.Value)
	case t.IsUnion():
		i.types[spec.RustUnionName(t)] = true
		for _, m := range t.Union {
			i.collect(m)
		}
	case t.IsObject():
		i.types[spec.RustTypeName(t.Name)] = true
	default:
		// A primitive that is a named alias in Rust is brought in too. The
		// nullability comes off first, since it is the alias inside the Option
		// that has to be named.
		bare := *t
		bare.Nullable = false
		if alias := bare.RustType(); isAliasName(alias) {
			i.types[alias] = true
		}
	}
}

// collectRecordTypes adds the types the narrowed record and nested types refer
// to, which are the properties they keep rather than the whole of the data
// type.
func (g *QueryGenerator) collectRecordTypes(c *query.Call, info *call, imports *imports) {
	add := func(o *spec.Object, properties []string) {
		for _, name := range properties {
			f, known := o.Field(name)
			if !known {
				if h, err := spec.ParseHeaderProperty(name); err == nil && h != nil {
					imports.collect(spec.MustParseType(h.Type))
				}
				continue
			}
			imports.collect(f.ParsedType())
		}
	}
	if dataType, ok := g.Spec.Object(c.Method.DataType); ok {
		properties := c.Properties
		if properties == nil {
			properties = dataType.PropertyNames()
		}
		add(dataType, shared.RecordProperties(properties))
	}
	if info.nestedType != "" {
		if nested, ok := g.Spec.Object(c.Method.NestedType); ok {
			add(nested, c.NestedProperties)
		}
		// A narrowed type replaces the shared one, so that name is not brought
		// in.
		delete(imports.types, spec.RustTypeName(c.Method.NestedType))
	}
}

// isAliasName reports whether a rendered primitive is one of the named string
// aliases rather than a built-in type.
func isAliasName(s string) bool {
	for _, alias := range spec.RustPrimitiveAliases() {
		if s == alias.Name {
			return true
		}
	}
	return false
}

// writeParams writes the struct holding what the caller supplies.
func (g *QueryGenerator) writeParams(buf *bytes.Buffer, p *plan) {
	if p.paramsType == "" {
		return
	}
	writeDoc(buf, "", p.paramsType+" holds the values "+p.q.Name+" leaves open.")
	buf.WriteString("#[derive(Debug, Clone, PartialEq, Default)]\n")
	fmt.Fprintf(buf, "pub struct %s {\n", p.paramsType)
	for i, param := range p.q.Params {
		if i > 0 {
			buf.WriteString("\n")
		}
		writeDoc(buf, "    ", param.Doc)
		fmt.Fprintf(buf, "    pub %s: %s,\n", spec.RustFieldName(param.Field), param.RustType())
	}
	buf.WriteString("}\n\n")
}

// writeNestedTypes writes the struct for a type nested inside the records,
// whose properties a separate argument narrows.
func (g *QueryGenerator) writeNestedTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.nestedType == "" {
			continue
		}
		nested, ok := g.Spec.Object(c.Method.NestedType)
		if !ok {
			continue
		}
		writeDoc(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.nestedType, nested.Name, c.Method.Name, p.q.Name))
		writeDerive(buf, false)
		buf.WriteString("#[serde(rename_all = \"camelCase\")]\n")
		fmt.Fprintf(buf, "pub struct %s {\n", info.nestedType)
		for i, name := range c.NestedProperties {
			if i > 0 {
				buf.WriteString("\n")
			}
			g.writeRecordField(buf, nested, name, info.nestedType, c.Method.NestedType)
		}
		buf.WriteString("}\n\n")
	}
}

// writeRecordTypes writes a struct for each call that narrows what it fetches.
func (g *QueryGenerator) writeRecordTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" {
			continue
		}
		dataType, ok := g.Spec.Object(c.Method.DataType)
		if !ok {
			continue
		}
		writeDoc(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.recordType, dataType.Name, c.Method.Name, p.q.Name))
		writeDerive(buf, false)
		buf.WriteString("#[serde(rename_all = \"camelCase\")]\n")
		fmt.Fprintf(buf, "pub struct %s {\n", info.recordType)
		properties := c.Properties
		if properties == nil {
			properties = dataType.PropertyNames()
		}
		for i, name := range shared.RecordProperties(properties) {
			if i > 0 {
				buf.WriteString("\n")
			}
			g.writeRecordField(buf, dataType, name, info.nestedType, c.Method.NestedType)
		}
		buf.WriteString("}\n\n")
	}
}

// writeRecordField writes one field of a generated record struct. A record
// comes back from the server, so a property it asked for is there: what the
// query narrowed to is not optional, only nullable where the type says so.
func (g *QueryGenerator) writeRecordField(buf *bytes.Buffer, dataType *spec.Object, name, nestedTo, nestedFrom string) {
	field, known := dataType.Field(name)
	if !known {
		if header, err := spec.ParseHeaderProperty(name); err == nil && header != nil {
			writeDoc(buf, "    ", shared.HeaderPropertyDoc(header))
			writeMember(buf, dataType.Name, name, spec.MustParseType(header.Type), false, true)
			return
		}
		writeDoc(buf, "    ", shared.DynamicPropertyDoc(name))
		writeDynamicMember(buf, name)
		return
	}
	writeDoc(buf, "    ", field.Doc)
	writeNestedMember(buf, dataType.Name, name, field.ParsedType(), nestedTo, nestedFrom)
}

// writeDynamicMember writes a property the data model does not describe, whose
// value is therefore whatever the server sent.
func writeDynamicMember(buf *bytes.Buffer, wireName string) {
	ident := spec.RustFieldName(wireName)
	writeSerdeAttr(buf, "    ", renameAndDefault(wireName, ident))
	fmt.Fprintf(buf, "    pub %s: serde_json::Value,\n", ident)
}

// renameAndDefault is the serde attribute for a field that is read from the
// server and may be left out of what it sends.
func renameAndDefault(wireName, ident string) []string {
	if rename := spec.SerdeRename(wireName, ident); rename != "" {
		return []string{fmt.Sprintf("rename = %q", rename), "default"}
	}
	return []string{"default"}
}

// writeNestedMember writes a record's field, pointing any reference to the
// narrowed type at the generated one.
func writeNestedMember(buf *bytes.Buffer, owner, wireName string, t *spec.Type, nestedTo, nestedFrom string) {
	if nestedTo == "" {
		writeMember(buf, owner, wireName, t, false, true)
		return
	}
	var swapped bytes.Buffer
	writeMember(&swapped, owner, wireName, t, false, true)
	buf.WriteString(strings.ReplaceAll(swapped.String(), spec.RustTypeName(nestedFrom), nestedTo))
}

// writeResponseTypes writes a response struct for each call whose records are a
// generated type.
func (g *QueryGenerator) writeResponseTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" {
			continue
		}
		respType, err := g.Spec.ResponseOf(c.Method.Name)
		if err != nil {
			continue
		}
		writeDoc(buf, "", fmt.Sprintf("%s holds the response to the %s call in %s.",
			info.responseType, c.Method.Name, p.q.Name))
		writeDerive(buf, false)
		buf.WriteString("#[serde(rename_all = \"camelCase\")]\n")
		fmt.Fprintf(buf, "pub struct %s {\n", info.responseType)
		for i, field := range respType.Fields {
			if i > 0 {
				buf.WriteString("\n")
			}
			writeDoc(buf, "    ", field.Doc)
			if field.Name == c.Method.ResultProperty {
				ident := spec.RustFieldName(field.Name)
				writeSerdeAttr(buf, "    ", renameAndDefault(field.Name, ident))
				fmt.Fprintf(buf, "    pub %s: Vec<%s>,\n", ident, info.recordType)
				continue
			}
			writeMember(buf, respType.Name, field.Name, field.ParsedType(), false, true)
		}
		buf.WriteString("}\n\n")
	}
}

// writeResultType writes the struct holding every call's response, for a query
// that does not single one out.
func (g *QueryGenerator) writeResultType(buf *bytes.Buffer, p *plan) {
	if p.resultType == "" {
		return
	}
	writeDoc(buf, "", fmt.Sprintf("%s holds the response to each method call %s makes.", p.resultType, p.q.Name))
	buf.WriteString("#[derive(Debug, Clone, PartialEq)]\n")
	fmt.Fprintf(buf, "pub struct %s {\n", p.resultType)
	for i, c := range p.q.Calls {
		if i > 0 {
			buf.WriteString("\n")
		}
		writeDoc(buf, "    ", fmt.Sprintf("The response to the %s call, made as %q.", c.Method.Name, c.ID))
		fmt.Fprintf(buf, "    pub %s: %s,\n", spec.RustFieldName(c.Field), p.calls[c].responseType)
	}
	if p.q.CreatedIDs {
		buf.WriteString("\n")
		writeDoc(buf, "    ", "The creation ids of everything created by this request, together with "+
			"those carried in. Pass it to the next request so that a reference to any of them still resolves.")
		buf.WriteString("    pub created_ids: BTreeMap<Id, Id>,\n")
	}
	buf.WriteString("}\n\n")
}

// writeMod writes the mod.rs that declares the generated modules, so that the
// directory is a module a crate can take in with one line.
func writeMod(modules []string) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, "the JMAP queries jmapc generated this directory from")
	buf.WriteString("pub mod client;\n")
	buf.WriteString("pub mod types;\n\n")
	for _, m := range modules {
		fmt.Fprintf(&buf, "pub mod %s;\n", m)
	}
	buf.WriteString("\n")
	writeDoc(&buf, "", "The runtime, brought up to the top so that a caller names the client "+
		"rather than the module it happens to live in. The data model stays where it is: it holds "+
		"a type for everything JMAP describes, and most programs want a few of them by name.")
	var reexport bytes.Buffer
	reexported := []string{
		"Auth", "Client", "ClientOptions", "Error", "HttpRequest", "HttpResponse", "Invocation",
		"MethodError", "MethodErrors", "Request", "RequestError", "Response", "ResultReference",
		"Session", "SetErrors", "SetFailure", "Transport", "TransportError", "decode",
	}
	sortUse(reexported)
	writeUse(&reexport, "client", reexported)
	buf.WriteString("pub " + reexport.String() + "\n")
	for _, m := range modules {
		fmt.Fprintf(&buf, "pub use %s::*;\n", m)
	}
	return finish(&buf)
}

// literalExpr renders a JSON value the query stated outright. JSON is what the
// json! macro takes, so it goes in as it is.
func literalExpr(raw json.RawMessage) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "null"
	}
	return compact.String()
}

// quote renders a Rust string literal.
func quote(s string) string { return strconv.Quote(s) }
