package ts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
)

// writeFunc writes the function that sends the query and decodes its response.
func (g *QueryGenerator) writeFunc(buf *bytes.Buffer, p *plan) {
	g.writeFuncDoc(buf, p)

	params := []string{"client: Client"}
	if p.paramsType != "" {
		params = append(params, "p: "+p.paramsType)
	}
	if p.q.CreatedIDs {
		params = append(params, "createdIds?: { [creationId: Id]: Id }")
	}
	fmt.Fprintf(buf, "export async function %s(%s): Promise<%s> {\n",
		p.funcName, strings.Join(params, ", "), p.returnType)

	for _, capability := range p.sessionCapabilities {
		fmt.Fprintf(buf, "  const %s = await client.primaryAccountId(%s)\n",
			accountIDVar(capability), strconv.Quote(capability))
	}
	if len(p.sessionCapabilities) > 0 {
		buf.WriteString("\n")
	}

	g.writeRequest(buf, p)

	buf.WriteString("  const res = await client.request(req)\n\n")
	// Where nothing can be refused there is nothing to hold the response for,
	// so the value is returned as it is decoded.
	checks := g.setErrorChecks(p)
	bind := "return "
	if len(checks) > 0 {
		bind = "const out = "
	}
	if p.q.Returns != nil {
		fmt.Fprintf(buf, "  %sdecode<%s>(req, res, %s)\n",
			bind, p.returnType, strconv.Quote(p.q.Returns.ID))
	} else {
		fmt.Fprintf(buf, "  %s{\n", bind)
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "    %s: decode<%s>(req, res, %s),\n",
				tsMemberName(c.Field), p.calls[c].responseType, strconv.Quote(c.ID))
		}
		if p.q.CreatedIDs {
			buf.WriteString("    createdIds: res.createdIds ?? {},\n")
		}
		buf.WriteString("  }\n")
	}
	if len(checks) > 0 {
		g.writeSetErrorChecks(buf, checks)
		buf.WriteString("  return out\n")
	}
	buf.WriteString("}\n")
}

// setErrorCheck is one method call whose response reports records the server
// refused.
type setErrorCheck struct {
	call   *query.Call
	object string // the expression holding the response
	decode string // the type to decode it from, empty where out holds it
	fields []string
}

// setErrorChecks finds the calls whose responses report per-record failures.
// A call the query does not return is decoded for its refusals alone, since
// otherwise naming one call in "_returns" would quietly stop the others from
// being checked.
func (g *QueryGenerator) setErrorChecks(p *plan) []setErrorCheck {
	var checks []setErrorCheck
	for i, c := range p.q.Calls {
		fields := g.Spec.SetErrorFields(c.Method.Name)
		if len(fields) == 0 {
			continue
		}
		ch := setErrorCheck{call: c, fields: fields, object: "out"}
		switch {
		case p.q.Returns == nil:
			ch.object = "out." + tsMemberName(c.Field)
		case c != p.q.Returns:
			ch.object = fmt.Sprintf("refused%d", i)
			ch.decode = p.calls[c].responseType
		}
		checks = append(checks, ch)
	}
	return checks
}

// writeSetErrorChecks writes the code that throws where the server refused a
// record. The response is carried on the error, since the rest of it happened.
func (g *QueryGenerator) writeSetErrorChecks(buf *bytes.Buffer, checks []setErrorCheck) {
	if len(checks) == 0 {
		return
	}
	buf.WriteString("\n")
	for _, ch := range checks {
		if ch.decode == "" {
			continue
		}
		fmt.Fprintf(buf, "  const %s = decode<%s>(req, res, %s)\n",
			ch.object, ch.decode, strconv.Quote(ch.call.ID))
	}
	buf.WriteString("  const failures: SetFailure[] = []\n")
	for _, ch := range checks {
		fmt.Fprintf(buf, "  collectSetErrors(%s, %s, {\n",
			strconv.Quote(ch.call.Method.Name), strconv.Quote(ch.call.ID))
		for _, name := range ch.fields {
			fmt.Fprintf(buf, "    %s: %s.%s,\n", name, ch.object, tsMemberName(name))
		}
		buf.WriteString("  }, failures)\n")
	}
	buf.WriteString("  if (failures.length > 0) throw new SetErrors(failures, out)\n")
	buf.WriteString("\n")
}

// writeFuncDoc writes the function's documentation.
func (g *QueryGenerator) writeFuncDoc(buf *bytes.Buffer, p *plan) {
	doc := strings.TrimSpace(p.q.Doc)
	if doc == "" {
		doc = fmt.Sprintf("%s sends the JMAP request in %s.", p.funcName, p.q.Path)
	}
	methods := make([]string, len(p.q.Calls))
	for i, c := range p.q.Calls {
		methods[i] = c.Method.Name
	}
	doc += fmt.Sprintf("\n\nIt makes %s in a single request, so that %s.",
		shared.JoinMethods(methods), shared.RoundTripPhrase(len(p.q.Calls)))
	if p.q.Returns != nil {
		doc += fmt.Sprintf(" It returns the response to the %s call.", p.q.Returns.Method.Name)
	}
	if len(p.sessionCapabilities) > 0 {
		doc += "\n\nThe query does not say which account to use, so the primary account of the session is used, which costs a session lookup on first use."
	}
	if p.q.CreatedIDs {
		doc += "\n\nIt takes the creation ids of an earlier request and reports its own, so that a reference to something created there still resolves here."
	}
	shared.WriteComment(buf, "", doc)
}

// writeRequest writes the literal request the function sends.
func (g *QueryGenerator) writeRequest(buf *bytes.Buffer, p *plan) {
	buf.WriteString("  const req: Request = {\n")
	uris := make([]string, len(p.q.Using))
	for i, uri := range p.q.Using {
		uris[i] = strconv.Quote(uri)
	}
	fmt.Fprintf(buf, "    using: [%s],\n", strings.Join(uris, ", "))
	buf.WriteString("    methodCalls: [\n")
	for _, c := range p.q.Calls {
		g.writeInvocation(buf, p, c)
	}
	buf.WriteString("    ],\n")
	if p.q.CreatedIDs {
		buf.WriteString("    createdIds,\n")
	}
	buf.WriteString("  }\n\n")
}

// writeInvocation writes one method call of the request.
func (g *QueryGenerator) writeInvocation(buf *bytes.Buffer, p *plan, c *query.Call) {
	if c.Comment != "" {
		shared.WriteComment(buf, "      ", c.Comment)
	}
	fmt.Fprintf(buf, "      [%s, {\n", strconv.Quote(c.Method.Name))
	if v := p.calls[c].accountIDVar; v != "" {
		fmt.Fprintf(buf, "        %s: %s,\n", strconv.Quote(query.AccountIDArgument), v)
	}
	for _, field := range c.Args.Fields {
		fmt.Fprintf(buf, "        %s: %s,\n", g.keyExpr(field), g.expr(field.Value, "        "))
	}
	fmt.Fprintf(buf, "      }, %s],\n", strconv.Quote(c.ID))
}

// keyExpr renders an argument name, which a query may build from parameters.
func (g *QueryGenerator) keyExpr(f query.ObjectField) string {
	if len(f.KeySegments) == 0 {
		return strconv.Quote(f.Key)
	}
	parts := make([]string, 0, len(f.KeySegments))
	for _, seg := range f.KeySegments {
		if seg.Param == nil {
			parts = append(parts, strconv.Quote(seg.Text))
			continue
		}
		parts = append(parts, "p."+tsMemberName(seg.Param.Field))
	}
	// A computed member name goes in brackets, and TypeScript joins the parts
	// with + as any other string expression.
	return "[" + strings.Join(parts, " + ") + "]"
}

// expr renders one argument value.
func (g *QueryGenerator) expr(n query.Node, indent string) string {
	switch v := n.(type) {
	case *query.ResultRef:
		return fmt.Sprintf("{ resultOf: %s, name: %s, path: %s }",
			strconv.Quote(v.Ref.ResultOf), strconv.Quote(v.Ref.Name), strconv.Quote(v.Ref.Path))

	case *query.ParamRef:
		return "p." + tsMemberName(v.Param.Field)

	case *query.Literal:
		return literalExpr(v.JSON)

	case *query.Object:
		if !v.HasParam() {
			return literalExpr(v.Raw)
		}
		var b strings.Builder
		b.WriteString("{\n")
		for _, f := range v.Fields {
			fmt.Fprintf(&b, "%s  %s: %s,\n", indent, g.keyExpr(f), g.expr(f.Value, indent+"  "))
		}
		b.WriteString(indent + "}")
		return b.String()

	case *query.Array:
		if !v.HasParam() {
			return literalExpr(v.Raw)
		}
		var b strings.Builder
		b.WriteString("[\n")
		for _, item := range v.Items {
			fmt.Fprintf(&b, "%s  %s,\n", indent, g.expr(item, indent+"  "))
		}
		b.WriteString(indent + "]")
		return b.String()
	}
	return "undefined"
}

// literalExpr renders a JSON value the query stated outright. JSON is a subset
// of TypeScript's own literal syntax, so it goes in as it is.
func literalExpr(raw json.RawMessage) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "undefined"
	}
	return compact.String()
}
