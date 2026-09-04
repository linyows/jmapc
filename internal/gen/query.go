// Package gen turns the JMAP data model, and the queries written against it,
// into Go source.
package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// QueryGenerator turns checked queries into Go functions. It works on a whole
// package at once, because the names it invents have to be unique across every
// query generated into the same place.
type QueryGenerator struct {
	// Spec is the catalogue the queries were checked against.
	Spec *spec.Spec
	// Package is the name of the package to generate into.
	Package string
	// Qualifier prefixes references to the runtime package.
	Qualifier string
	// Queries are the queries to generate, in file name order.
	Queries []*query.Query
}

// call holds the names the generator settled on for one method call.
type call struct {
	// responseType is the Go type the call's response decodes into.
	responseType string
	// recordType names the generated record type, empty when the call fetches
	// whole records and the runtime type is used.
	recordType string
	// nestedType names the generated type for the records' nested type, as
	// bodyProperties gives an Email's body parts, empty when that is not
	// narrowed either.
	nestedType string
	// accountIDExpr is the Go expression for the accountId argument the query
	// left out, empty when the query supplies one.
	accountIDExpr string
}

// plan is the naming decided for one query before any code is written.
type plan struct {
	q *query.Query
	// paramsType is the generated parameter struct, empty when the query takes
	// no parameters.
	paramsType string
	// resultType is the generated struct holding every call's response, empty
	// when the query returns a single call's response.
	resultType string
	// returnType is the Go type the generated function returns, without the
	// leading "*".
	returnType string
	// calls maps each method call to the names generated for it.
	calls map[*query.Call]*call
	// sessionCapabilities lists the capabilities whose primary account the
	// function has to look up, in a stable order.
	sessionCapabilities []string
	// watchName is the function that follows the watched call's changes, empty
	// where the query is not watched.
	watchName string
}

// Generate returns the source of one file per query, keyed by file name.
func (g *QueryGenerator) Generate() (map[string][]byte, error) {
	plans, err := g.plan()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(plans))
	for _, p := range plans {
		src, err := g.file(p)
		if err != nil {
			return nil, err
		}
		out[fileName(p.q.Name)] = src
	}
	return out, nil
}

// fileName returns the file a query is generated into.
func fileName(queryName string) string {
	return strings.ToLower(queryName) + "_gen.go"
}

// plan settles every generated name up front, so that two queries in the same
// package cannot claim the same one.
func (g *QueryGenerator) plan() ([]*plan, error) {
	queries := append([]*query.Query(nil), g.Queries...)
	sort.Slice(queries, func(i, j int) bool { return queries[i].Name < queries[j].Name })

	taken := make(map[string]bool)
	for _, q := range queries {
		if taken[q.Name] {
			return nil, fmt.Errorf("gen: two queries are named %s", q.Name)
		}
		taken[q.Name] = true
	}

	plans := make([]*plan, 0, len(queries))
	for _, q := range queries {
		p := &plan{q: q, calls: make(map[*query.Call]*call, len(q.Calls))}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, q.Name+"Params")
		}
		for _, c := range q.Calls {
			info := &call{}
			// Narrowing the nested type means the record type has to be
			// generated too, since its own fields change to refer to it.
			if c.Properties != nil || c.NestedProperties != nil {
				info.recordType = shared.Unique(taken, q.Name+spec.ExportedName(c.Method.DataType))
				info.responseType = shared.Unique(taken, q.Name+c.Field+"Response")
			} else {
				info.responseType = g.Qualifier + spec.ExportedName(c.Method.Response)
			}
			if c.NestedProperties != nil {
				info.nestedType = shared.Unique(taken, q.Name+spec.ExportedName(c.Method.NestedType))
			}
			p.calls[c] = info
		}
		if q.Returns != nil {
			p.returnType = p.calls[q.Returns].responseType
		} else {
			p.resultType = shared.Unique(taken, q.Name+"Result")
			p.returnType = p.resultType
		}
		g.planAccountIDs(p)
		if q.Watches != nil {
			p.watchName = shared.Unique(taken, q.Name+"Watch")
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// planAccountIDs works out which calls need an accountId filling in, and from
// which capability's primary account. A query that does not care which account
// it runs against should not have to say so in every call.
func (g *QueryGenerator) planAccountIDs(p *plan) {
	seen := make(map[string]bool)
	for _, c := range p.q.Calls {
		args, err := g.Spec.ArgumentsOf(c.Method.Name)
		if err != nil {
			continue
		}
		if _, takesAccount := args.Field(query.AccountIDArgument); !takesAccount {
			continue
		}
		if _, given := c.Args.Find(query.AccountIDArgument); given {
			continue
		}
		if _, referenced := c.Args.Find("#" + query.AccountIDArgument); referenced {
			continue
		}
		capability := c.Method.Capability
		if capability == "" {
			capability = spec.CapabilityCore
		}
		if !seen[capability] {
			seen[capability] = true
			p.sessionCapabilities = append(p.sessionCapabilities, capability)
		}
		p.calls[c].accountIDExpr = accountIDVar(capability)
	}
	sort.Strings(p.sessionCapabilities)
}

// accountIDVar names the local variable holding the primary account id for a
// capability.
func accountIDVar(capability string) string {
	short := capability
	if i := strings.LastIndex(short, ":"); i >= 0 {
		short = short[i+1:]
	}
	return spec.UnexportedName(short) + "AccountID"
}

// file writes the source for one query.
func (g *QueryGenerator) file(p *plan) ([]byte, error) {
	var body bytes.Buffer
	g.writeParams(&body, p)
	g.writeRecordTypes(&body, p)
	g.writeResponseTypes(&body, p)
	g.writeResultType(&body, p)
	g.writeFunc(&body, p)
	g.writeWatch(&body, p)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by jmapc. DO NOT EDIT.\n")
	// The path is spelled with forward slashes whatever the host uses, so that
	// generating on Windows and on Unix produce the same file. This is not
	// filepath.ToSlash, which only rewrites the separator of the host it runs
	// on and so would leave a Windows path alone everywhere else.
	fmt.Fprintf(&buf, "// Source: %s\n\n", strings.ReplaceAll(p.q.Path, `\`, "/"))
	fmt.Fprintf(&buf, "package %s\n\n", g.Package)
	g.writeImports(&buf, body.Bytes())
	buf.Write(body.Bytes())

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gen: formatting %s: %w\n%s", p.q.Name, err, buf.String())
	}
	return src, nil
}

// writeImports writes the import block, including only what the body uses.
func (g *QueryGenerator) writeImports(buf *bytes.Buffer, body []byte) {
	imports := []string{"context"}
	if bytes.Contains(body, []byte("json.")) {
		imports = append(imports, "encoding/json")
	}
	buf.WriteString("import (\n")
	for _, path := range imports {
		fmt.Fprintf(buf, "\t%q\n", path)
	}
	fmt.Fprintf(buf, "\n\t%q\n", "github.com/linyows/jmapc")
	buf.WriteString(")\n")
}

// writeParams writes the struct holding the values the caller supplies.
func (g *QueryGenerator) writeParams(buf *bytes.Buffer, p *plan) {
	if p.paramsType == "" {
		return
	}
	shared.WriteComment(buf, "", p.paramsType+" holds the values "+p.q.Name+" leaves open.")
	fmt.Fprintf(buf, "type %s struct {\n", p.paramsType)
	for i, param := range p.q.Params {
		if i > 0 {
			buf.WriteString("\n")
		}
		shared.WriteComment(buf, "\t", param.Doc)
		fmt.Fprintf(buf, "\t%s %s\n", param.Field, param.GoType(g.Qualifier))
	}
	buf.WriteString("}\n\n")
}

// writeRecordTypes writes a struct for each /get call that names the properties
// it wants, holding exactly those properties and no others.
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
		if info.nestedType != "" {
			g.writeNestedType(buf, p, c, info)
		}

		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.recordType, dataType.Name, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "type %s struct {\n", info.recordType)
		properties := c.Properties
		if properties == nil {
			// Only the nested type was narrowed, so the record keeps all of
			// its own properties and only the ones referring to the nested
			// type change.
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

// writeNestedType writes the struct for a type nested inside the records, whose
// properties a separate argument narrows. Its own reference to itself — the
// sub-parts of a body part — points at the generated type rather than the
// runtime one, so a whole tree of parts carries only what was asked for.
func (g *QueryGenerator) writeNestedType(buf *bytes.Buffer, p *plan, c *query.Call, info *call) {
	nested, ok := g.Spec.Object(c.Method.NestedType)
	if !ok {
		return
	}
	shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
		info.nestedType, nested.Name, c.Method.Name, p.q.Name))
	fmt.Fprintf(buf, "type %s struct {\n", info.nestedType)
	for i, name := range c.NestedProperties {
		if i > 0 {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, nested, name, info.nestedType, c.Method.NestedType)
	}
	buf.WriteString("}\n\n")
}

// writeRecordField writes one field of a generated record type. Where nestedTo
// is set, a reference to the type named by nestedFrom becomes a reference to
// the generated one instead.
func (g *QueryGenerator) writeRecordField(buf *bytes.Buffer, dataType *spec.Object, name, nestedTo, nestedFrom string) {
	field, known := dataType.Field(name)
	if !known {
		// A property naming one header field of the message has a type after
		// all: the form asked for decides it.
		if header, err := spec.ParseHeaderProperty(name); err == nil && header != nil {
			shared.WriteComment(buf, "\t", shared.HeaderPropertyDoc(header))
			fmt.Fprintf(buf, "\t%s %s `json:%q`\n",
				spec.ExportedName(name), spec.MustParseType(header.Type).GoType(g.Qualifier), name)
			return
		}
		// Anything else the server gives meaning to is left as raw JSON for
		// the caller to interpret.
		shared.WriteComment(buf, "\t", shared.DynamicPropertyDoc(name))
		fmt.Fprintf(buf, "\t%s json.RawMessage `json:%q`\n", spec.ExportedName(name), name)
		return
	}
	shared.WriteComment(buf, "\t", field.Doc)
	fmt.Fprintf(buf, "\t%s %s `json:%q`\n",
		spec.ExportedName(name), g.nestedGoType(field.ParsedType(), nestedTo, nestedFrom), name)
}

// nestedGoType renders a field's Go type, pointing any reference to the
// narrowed type at the generated one.
func (g *QueryGenerator) nestedGoType(t *spec.Type, nestedTo, nestedFrom string) string {
	goType := t.GoType(g.Qualifier)
	if nestedTo == "" {
		return goType
	}
	return strings.ReplaceAll(goType, g.Qualifier+spec.ExportedName(nestedFrom), nestedTo)
}

// writeResponseTypes writes a response struct for each call whose records are a
// generated type, mirroring the runtime response but holding those records.
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
		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the response to the %s call in %s.",
			info.responseType, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "type %s struct {\n", info.responseType)
		for i, field := range respType.Fields {
			if i > 0 {
				buf.WriteString("\n")
			}
			shared.WriteComment(buf, "\t", field.Doc)
			goType := field.ParsedType().GoType(g.Qualifier)
			if field.Name == c.Method.ResultProperty {
				goType = "[]" + info.recordType
			}
			fmt.Fprintf(buf, "\t%s %s `json:%q`\n", spec.ExportedName(field.Name), goType, field.Name)
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
	shared.WriteComment(buf, "", fmt.Sprintf("%s holds the response to each method call %s makes.", p.resultType, p.q.Name))
	fmt.Fprintf(buf, "type %s struct {\n", p.resultType)
	for i, c := range p.q.Calls {
		if i > 0 {
			buf.WriteString("\n")
		}
		shared.WriteComment(buf, "\t", fmt.Sprintf("The response to the %s call, made as %q.", c.Method.Name, c.ID))
		fmt.Fprintf(buf, "\t%s %s\n", c.Field, p.calls[c].responseType)
	}
	if p.q.CreatedIDs {
		buf.WriteString("\n")
		shared.WriteComment(buf, "\t", "The creation ids of everything created by this request, together with "+
			"those carried in. Pass it to the next request so that a reference to any of them still resolves.")
		fmt.Fprintf(buf, "\tCreatedIDs map[%[1]sID]%[1]sID\n", g.Qualifier)
	}
	buf.WriteString("}\n\n")
}
