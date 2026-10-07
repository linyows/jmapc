// Package gen turns the JMAP data model, and the requests written against it,
// into Go source.
package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// RequestGenerator turns checked requests into Go functions. It works on a whole
// package at once, because the names it invents have to be unique across every
// request generated into the same place.
type RequestGenerator struct {
	// Spec is the catalogue the requests were checked against.
	Spec *spec.Spec
	// Package is the name of the package to generate into.
	Package string
	// Qualifier prefixes references to the runtime package.
	Qualifier string
	// Requests are the requests to generate, in file name order.
	Requests []*request.Request
	// Properties are the named sets of properties the requests ask for, whose
	// types are generated once for the whole package. It is nil where the
	// project names no set.
	Properties *request.PropertySets
}

// call is what the Go generator writes for one method call.
type call struct {
	shared.CallPlan
}

// plan is the naming decided for one request before any code is written.
type plan struct {
	q *request.Request
	// creations are the constants naming the creation ids the request invents.
	creations []shared.Creation
	// paramsType is the generated parameter struct, empty when the request takes
	// no parameters.
	paramsType string
	// resultType is the generated struct holding every call's response, empty
	// when the request returns a single call's response.
	resultType string
	// returnType is the Go type the generated function returns, without the
	// leading "*".
	returnType string
	// calls maps each method call to the names generated for it.
	calls map[*request.Call]*call
	// sessionCapabilities lists the capabilities whose primary account the
	// function has to look up, in a stable order.
	sessionCapabilities []string
	// watchName is the function that follows the watched call's changes, empty
	// where the request is not watched.
	watchName string
	// pagesName is the function that walks the paged call's windows, empty
	// where the request is not paged.
	pagesName string
}

// Generate returns the source of one file per request, keyed by file name.
func (g *RequestGenerator) Generate() (map[string][]byte, error) {
	plans, err := g.plan()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(plans)+2)
	// from records what each file was generated from, so that two things
	// generated into one file are refused rather than one silently replacing
	// the other. File names are lower case, so "Foo" and "foo" are two
	// requests and one file.
	from := make(map[string]string, len(plans)+2)
	add := func(file, what string, src []byte) error {
		if earlier, taken := from[file]; taken {
			return fmt.Errorf("gen: %s and %s would both be generated into %s; rename one of them", earlier, what, file)
		}
		from[file] = what
		out[file] = src
		return nil
	}
	if len(g.Properties.Names()) > 0 {
		src, err := g.propertiesFile()
		if err != nil {
			return nil, err
		}
		if err := add(PropertiesFileName, "the sets of properties", src); err != nil {
			return nil, err
		}
	}
	if len(plans) > 0 {
		src, err := g.verifyFile(plans)
		if err != nil {
			return nil, err
		}
		if err := add(VerifyFileName, "the function that verifies the requests", src); err != nil {
			return nil, err
		}
	}
	for _, p := range plans {
		src, err := g.file(p)
		if err != nil {
			return nil, err
		}
		if err := add(fileName(p.q.Name), "the request "+p.q.Name, src); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fileName returns the file a request is generated into.
func fileName(queryName string) string {
	return strings.ToLower(queryName) + "_gen.go"
}

// plan settles every generated name up front, so that two requests in the same
// package cannot claim the same one.
func (g *RequestGenerator) plan() ([]*plan, error) {
	requests := append([]*request.Request(nil), g.Requests...)
	sort.Slice(requests, func(i, j int) bool { return requests[i].Name < requests[j].Name })

	taken := make(map[string]bool)
	// The sets are named first, because their names are the ones an author
	// chose for a type a caller names: a request has to give way to them
	// rather than the other way about.
	for _, name := range g.Properties.Names() {
		if name == VerifyFunc {
			return nil, fmt.Errorf("gen: the set of properties %s would be generated under the name of the function that verifies the requests; name it something else", name)
		}
		taken[name] = true
	}
	taken[VerifyFunc] = true
	for _, q := range requests {
		if q.Name == VerifyFunc {
			return nil, fmt.Errorf("gen: the request %s would be generated under the name of the function that verifies the requests; name it something else", q.Name)
		}
		if taken[q.Name] {
			if _, set := g.Properties.Find(q.Name); set {
				return nil, fmt.Errorf("gen: the request %s and the set of properties of the same name would both be generated as %s", q.Name, q.Name)
			}
			return nil, fmt.Errorf("gen: two requests are named %s", q.Name)
		}
		taken[q.Name] = true
	}

	plans := make([]*plan, 0, len(requests))
	for _, q := range requests {
		p := &plan{q: q, calls: make(map[*request.Call]*call, len(q.Calls))}
		if len(q.Params) > 0 {
			p.paramsType = shared.Unique(taken, q.Name+"Params")
		}
		p.creations = shared.Creations(taken, q.Name, q.Creations, spec.ExportedName)
		calls, capabilities := shared.PlanCalls(g.Spec, q, taken, shared.Namer{
			Prefix:    q.Name,
			Field:     func(name string) string { return name },
			Part:      spec.ExportedName,
			Set:       func(name string) string { return name },
			Runtime:   func(name string) string { return g.Qualifier + spec.ExportedName(name) },
			AccountID: accountIDVar,
		})
		for _, c := range q.Calls {
			p.calls[c] = &call{CallPlan: *calls[c]}
		}
		p.sessionCapabilities = capabilities
		if q.Returns != nil {
			p.returnType = p.calls[q.Returns].ResponseType
		} else {
			p.resultType = shared.Unique(taken, q.Name+"Result")
			p.returnType = p.resultType
		}
		if q.Watches != nil {
			p.watchName = shared.Unique(taken, q.Name+"Watch")
		}
		if q.Pages != nil {
			p.pagesName = shared.Unique(taken, q.Name+"Pages")
		}
		plans = append(plans, p)
	}
	return plans, nil
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

// file writes the source for one request.
func (g *RequestGenerator) file(p *plan) ([]byte, error) {
	var body bytes.Buffer
	g.writeParams(&body, p)
	g.writeCreations(&body, p)
	g.writeRecordTypes(&body, p)
	g.writeResponseTypes(&body, p)
	g.writeResultType(&body, p)
	g.writeFunc(&body, p)
	g.writeWatch(&body, p)
	g.writePages(&body, p)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s\n", shared.GeneratedBanner)
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

// packagesUsed returns the names of the packages body refers to, as in json.
// for encoding/json. It reads body as Go rather than as text, so that a name
// in a comment, which the documentation of a request may well hold, does not
// bring in a package nothing uses. Where body does not parse, it falls back to
// the text, and formatting the file reports what is wrong with it.
func packagesUsed(body []byte) map[string]bool {
	used := map[string]bool{}
	file, err := parser.ParseFile(token.NewFileSet(), "", append([]byte("package p\n"), body...), 0)
	if err != nil {
		for _, name := range []string{"json", "errors", "iter"} {
			used[name] = bytes.Contains(body, []byte(name+"."))
		}
		return used
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok {
				used[x.Name] = true
			}
		}
		return true
	})
	return used
}

// writeImports writes the import block, including only what the body uses.
func (g *RequestGenerator) writeImports(buf *bytes.Buffer, body []byte) {
	used := packagesUsed(body)
	imports := []string{"context"}
	if used["json"] {
		imports = append(imports, "encoding/json")
	}
	if used["errors"] {
		imports = append(imports, "errors")
	}
	if used["iter"] {
		imports = append(imports, "iter")
	}
	buf.WriteString("import (\n")
	for _, path := range imports {
		fmt.Fprintf(buf, "\t%q\n", path)
	}
	fmt.Fprintf(buf, "\n\t%q\n", "github.com/linyows/jmapc")
	buf.WriteString(")\n")
}

// writeCreations writes a constant for each creation id the request invents. A
// /set reports what it created under the name the request gave it, and without
// this the caller spells that name a second time, in another file, with
// nothing holding the two together.
func (g *RequestGenerator) writeCreations(buf *bytes.Buffer, p *plan) {
	for _, c := range p.creations {
		shared.WriteComment(buf, "", fmt.Sprintf(
			"%s is the creation id %s gives a record it creates, which the response reports it under.",
			c.Name, p.q.Name))
		fmt.Fprintf(buf, "const %s %sID = %q\n\n", c.Name, g.Qualifier, c.ID)
	}
}

// writeParams writes the struct holding the values the caller supplies.
func (g *RequestGenerator) writeParams(buf *bytes.Buffer, p *plan) {
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
func (g *RequestGenerator) writeRecordTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.RecordType == "" || !info.WritesTypes || info.SharedRecord {
			continue
		}
		dataType, ok := shared.RecordObject(g.Spec, c.Method)
		if !ok {
			continue
		}
		if info.NestedType != "" && !info.SharedNested {
			g.writeNestedType(buf, p, c, info)
		}

		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
			info.RecordType, dataType.Name, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "type %s struct {\n", info.RecordType)
		properties := c.Properties
		if properties == nil {
			// Only the nested type was narrowed, so the record keeps all of
			// its own properties and only the ones referring to the nested
			// type change.
			properties = dataType.PropertyNames()
		}
		for i, name := range shared.RecordProperties(dataType, properties, c.Method.ReturnsID) {
			if i > 0 {
				buf.WriteString("\n")
			}
			g.writeRecordField(buf, dataType, properties, name, info.NestedType, c.Method.NestedType)
		}
		buf.WriteString("}\n\n")
	}
}

// writeNestedType writes the struct for a type nested inside the records, whose
// properties a separate argument narrows. Its own reference to itself — the
// sub-parts of a body part — points at the generated type rather than the
// runtime one, so a whole tree of parts carries only what was asked for.
func (g *RequestGenerator) writeNestedType(buf *bytes.Buffer, p *plan, c *request.Call, info *call) {
	nested, ok := g.Spec.Object(c.Method.NestedType)
	if !ok {
		return
	}
	shared.WriteComment(buf, "", fmt.Sprintf("%s holds the properties of %s that the %s call in %s asks for.",
		info.NestedType, nested.Name, c.Method.Name, p.q.Name))
	fmt.Fprintf(buf, "type %s struct {\n", info.NestedType)
	for i, name := range c.NestedProperties {
		if i > 0 {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, nested, c.NestedProperties, name, info.NestedType, c.Method.NestedType)
	}
	buf.WriteString("}\n\n")
}

// writeRecordField writes one field of a generated record type. Where nestedTo
// is set, a reference to the type named by nestedFrom becomes a reference to
// the generated one instead.
func (g *RequestGenerator) writeRecordField(buf *bytes.Buffer, dataType *spec.Object, asked []string, name, nestedTo, nestedFrom string) {
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
	shared.WriteComment(buf, "\t", shared.RecordFieldDoc(dataType, asked, field))
	fmt.Fprintf(buf, "\t%s %s `json:%q`\n",
		spec.ExportedName(name), g.nestedGoType(field.ParsedType(), nestedTo, nestedFrom), name)
}

// nestedGoType renders a field's Go type, pointing any reference to the
// narrowed type at the generated one.
func (g *RequestGenerator) nestedGoType(t *spec.Type, nestedTo, nestedFrom string) string {
	goType := t.GoType(g.Qualifier)
	if nestedTo == "" {
		return goType
	}
	return strings.ReplaceAll(goType, g.Qualifier+spec.ExportedName(nestedFrom), nestedTo)
}

// writeResponseTypes writes a response struct for each call whose records are a
// generated type, mirroring the runtime response but holding those records.
func (g *RequestGenerator) writeResponseTypes(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		info := p.calls[c]
		if info.RecordType == "" || !info.WritesTypes {
			continue
		}
		respType, err := g.Spec.ResponseOf(c.Method.Name)
		if err != nil {
			continue
		}
		shared.WriteComment(buf, "", fmt.Sprintf("%s holds the response to the %s call in %s.",
			info.ResponseType, c.Method.Name, p.q.Name))
		fmt.Fprintf(buf, "type %s struct {\n", info.ResponseType)
		for i, field := range respType.Fields {
			if i > 0 {
				buf.WriteString("\n")
			}
			shared.WriteComment(buf, "\t", field.Doc)
			goType := field.ParsedType().GoType(g.Qualifier)
			if field.Name == c.Method.ResultProperty {
				goType = shared.ResultType(field.ParsedType(), c.Method.DataType, info.RecordType,
					func(t *spec.Type) string { return t.GoType(g.Qualifier) }, goShape(g.Qualifier))
			}
			fmt.Fprintf(buf, "\t%s %s `json:%q`\n", spec.ExportedName(field.Name), goType, field.Name)
		}
		buf.WriteString("}\n\n")
	}
}

// writeResultType writes the struct holding every call's response, for a request
// that does not single one out.
func (g *RequestGenerator) writeResultType(buf *bytes.Buffer, p *plan) {
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
		fmt.Fprintf(buf, "\t%s %s\n", c.Field, p.calls[c].ResponseType)
	}
	if p.q.CreatedIDs {
		buf.WriteString("\n")
		shared.WriteComment(buf, "\t", "The creation ids of everything created by this request, together with "+
			"those carried in. Pass it to the next request so that a reference to any of them still resolves.")
		fmt.Fprintf(buf, "\tCreatedIDs map[%[1]sID]%[1]sID\n", g.Qualifier)
	}
	buf.WriteString("}\n\n")
}

// goShape writes the containers around a call's records as Go writes them: a
// nil slice or map is already null, and a record that may be null is a pointer.
func goShape(qualifier string) shared.Shape {
	return shared.Shape{
		List: func(_ *spec.Type, s string) string { return "[]" + s },
		Map:  func(key *spec.Type, s string) string { return "map[" + key.GoType(qualifier) + "]" + s },
		Null: func(t *spec.Type, s string) string {
			if t.IsArray() || t.IsMap() {
				return s
			}
			return "*" + s
		},
	}
}
