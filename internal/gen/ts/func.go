package ts

import (
	"bytes"
	"fmt"
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
	var buf bytes.Buffer
	writeHeader(&buf, p.q.Path)
	g.writeImports(&buf, p)
	g.writeParams(&buf, p)
	g.writeNestedTypes(&buf, p)
	g.writeRecordTypes(&buf, p)
	g.writeResponseTypes(&buf, p)
	g.writeResultType(&buf, p)
	g.writeFunc(&buf, p)
	g.writePages(&buf, p)
	return buf.Bytes()
}

// writeImports names what the module takes from the runtime and the types,
// which TypeScript wants stated rather than inferred.
func (g *QueryGenerator) writeImports(buf *bytes.Buffer, p *plan) {
	// The specifier carries the .js extension that TypeScript asks for when it
	// emits ES modules: Node resolves it against the compiled output, and a
	// bundler resolves it against the source either way. Without it the module
	// compiles and then fails to load.
	// Types first, then values, rather than sorted: the reader is looking for
	// what the module uses, not for a name in a list.
	runtime := []string{"type Client", "type Request"}
	values := []string{"decode"}
	if len(g.setErrorChecks(p)) > 0 {
		runtime = append(runtime, "type SetFailure")
		values = append(values, "SetErrors", "collectSetErrors")
	}
	runtime = append(runtime, values...)
	fmt.Fprintf(buf, "import { %s } from \"./client.js\"\n", strings.Join(runtime, ", "))

	names := map[string]bool{}
	for _, param := range p.q.Params {
		collectTypeNames(param.ValueType(), names)
	}
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" {
			// The shared response type comes from types.ts.
			names[spec.ExportedName(c.Method.Response)] = true
			continue
		}
		if resp, err := g.Spec.ResponseOf(c.Method.Name); err == nil {
			for _, f := range resp.Fields {
				if f.Name != c.Method.ResultProperty {
					collectTypeNames(f.ParsedType(), names)
				}
			}
		}
		g.collectRecordTypeNames(c, info, names)
	}
	if p.q.CreatedIDs {
		names["Id"] = true
	}

	if len(names) > 0 {
		sorted := make([]string, 0, len(names))
		for name := range names {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		fmt.Fprintf(buf, "import type { %s } from \"./types.js\"\n", strings.Join(sorted, ", "))
	}
	buf.WriteString("\n")
}

// collectRecordTypeNames adds the types the narrowed record and nested types
// refer to, which are the properties they keep rather than the whole of the
// data type.
func (g *QueryGenerator) collectRecordTypeNames(c *query.Call, info *call, names map[string]bool) {
	add := func(o *spec.Object, properties []string) {
		for _, name := range properties {
			f, known := o.Field(name)
			if !known {
				if h, err := spec.ParseHeaderProperty(name); err == nil && h != nil {
					collectTypeNames(spec.MustParseType(h.Type), names)
				}
				continue
			}
			collectTypeNames(f.ParsedType(), names)
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
	}
	// A narrowed type replaces the runtime one, so that name is not imported.
	if info.nestedType != "" {
		delete(names, spec.ExportedName(c.Method.NestedType))
	}
}

// collectTypeNames adds every object type a type expression names.
func collectTypeNames(t *spec.Type, names map[string]bool) {
	switch {
	case t == nil:
	case t.IsArray():
		collectTypeNames(t.Elem, names)
	case t.IsMap():
		collectTypeNames(t.Key, names)
		collectTypeNames(t.Value, names)
	case t.IsUnion():
		for _, m := range t.Union {
			collectTypeNames(m, names)
		}
	case t.IsObject():
		names[spec.ExportedName(t.Name)] = true
	default:
		// A primitive that is a named alias in TypeScript is imported too.
		if alias := t.TSType(); alias != "" && isAliasName(alias) {
			names[alias] = true
		}
	}
}

// isAliasName reports whether a rendered primitive is one of the named string
// aliases rather than a built-in type.
func isAliasName(s string) bool {
	for _, alias := range spec.TSPrimitiveAliases() {
		if s == alias.Name {
			return true
		}
	}
	return false
}

// writeParams writes the interface holding what the caller supplies.
func (g *QueryGenerator) writeParams(buf *bytes.Buffer, p *plan) {
	if p.paramsType == "" {
		return
	}
	shared.WriteComment(buf, "", p.paramsType+" holds the values "+p.q.Name+" leaves open.")
	fmt.Fprintf(buf, "export interface %s {\n", p.paramsType)
	for i, param := range p.q.Params {
		if i > 0 {
			buf.WriteString("\n")
		}
		shared.WriteComment(buf, "  ", param.Doc)
		// A parameter that may be left out is an optional member, so leaving
		// it out is what the caller writes, and undefined is what the request
		// builder looks for.
		optional := ""
		if param.Optional {
			optional = "?"
		}
		fmt.Fprintf(buf, "  %s%s: %s\n", tsMemberName(param.Field), optional, param.ValueType().TSType())
	}
	buf.WriteString("}\n\n")
}

// writeNestedTypes writes the type for a type nested inside the records, whose
// properties a separate argument narrows.
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
		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.nestedType, nested.Name, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "export interface %s {\n", info.nestedType)
		for i, name := range c.NestedProperties {
			if i > 0 {
				buf.WriteString("\n")
			}
			g.writeRecordField(buf, nested, name, info.nestedType, c.Method.NestedType)
		}
		buf.WriteString("}\n\n")
	}
}

// writeRecordTypes writes a type for each call that narrows what it fetches.
func (g *QueryGenerator) writeRecordTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" || !info.writesTypes {
			continue
		}
		dataType, ok := g.Spec.Object(c.Method.DataType)
		if !ok {
			continue
		}
		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.recordType, dataType.Name, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "export interface %s {\n", info.recordType)
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

// writeRecordField writes one member of a generated record type.
func (g *QueryGenerator) writeRecordField(buf *bytes.Buffer, dataType *spec.Object, name, nestedTo, nestedFrom string) {
	memberName := name
	if spec.TSNeedsQuoting(memberName) {
		memberName = strconv.Quote(memberName)
	}

	field, known := dataType.Field(name)
	if !known {
		if header, err := spec.ParseHeaderProperty(name); err == nil && header != nil {
			shared.WriteComment(buf, "  ", shared.HeaderPropertyDoc(header))
			fmt.Fprintf(buf, "  %s: %s\n", memberName, spec.MustParseType(header.Type).TSType())
			return
		}
		shared.WriteComment(buf, "  ", shared.DynamicPropertyDoc(name))
		fmt.Fprintf(buf, "  %s: unknown\n", memberName)
		return
	}
	shared.WriteComment(buf, "  ", field.Doc)
	fmt.Fprintf(buf, "  %s: %s\n", memberName, g.nestedTSType(field.ParsedType(), nestedTo, nestedFrom))
}

// nestedTSType renders a member's type, pointing any reference to the narrowed
// type at the generated one.
func (g *QueryGenerator) nestedTSType(t *spec.Type, nestedTo, nestedFrom string) string {
	rendered := t.TSType()
	if nestedTo == "" {
		return rendered
	}
	return strings.ReplaceAll(rendered, spec.ExportedName(nestedFrom), nestedTo)
}

// writeResponseTypes writes a response type for each call whose records are a
// generated type.
func (g *QueryGenerator) writeResponseTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.recordType == "" || !info.writesTypes {
			continue
		}
		respType, err := g.Spec.ResponseOf(c.Method.Name)
		if err != nil {
			continue
		}
		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the response to the %s call in %s.",
			info.responseType, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "export interface %s {\n", info.responseType)
		for i, field := range respType.Fields {
			if i > 0 {
				buf.WriteString("\n")
			}
			shared.WriteComment(buf, "  ", field.Doc)
			tsType := field.ParsedType().TSType()
			if field.Name == c.Method.ResultProperty {
				tsType = info.recordType + "[]"
			}
			name := field.Name
			if spec.TSNeedsQuoting(name) {
				name = strconv.Quote(name)
			}
			fmt.Fprintf(buf, "  %s: %s\n", name, tsType)
		}
		buf.WriteString("}\n\n")
	}
}

// writeResultType writes the type holding every call's response, for a query
// that does not single one out.
func (g *QueryGenerator) writeResultType(buf *bytes.Buffer, p *plan) {
	if p.resultType == "" {
		return
	}
	shared.WriteComment(buf, "", fmt.Sprintf("%s holds the response to each method call %s makes.", p.resultType, p.q.Name))
	fmt.Fprintf(buf, "export interface %s {\n", p.resultType)
	for i, c := range p.q.Calls {
		if i > 0 {
			buf.WriteString("\n")
		}
		shared.WriteComment(buf, "  ", fmt.Sprintf("The response to the %s call, made as %q.", c.Method.Name, c.ID))
		fmt.Fprintf(buf, "  %s: %s\n", tsMemberName(c.Field), p.calls[c].responseType)
	}
	if p.q.CreatedIDs {
		buf.WriteString("\n")
		shared.WriteComment(buf, "  ", "The creation ids of everything created by this request, together with "+
			"those carried in. Pass it to the next request so that a reference to any of them still resolves.")
		buf.WriteString("  createdIds: { [creationId: Id]: Id }\n")
	}
	buf.WriteString("}\n\n")
}
