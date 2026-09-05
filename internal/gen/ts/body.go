package ts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
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

	// A method call the server would not run does not take the answers to the
	// others with it: the response is read either way, and what came of it is
	// put on the error before it goes on.
	buf.WriteString("  let res: Response\n")
	buf.WriteString("  let failed: MethodErrors | undefined\n")
	buf.WriteString("  try {\n")
	buf.WriteString("    res = await client.request(req)\n")
	buf.WriteString("  } catch (e) {\n")
	buf.WriteString("    if (!(e instanceof MethodErrors)) throw e\n")
	buf.WriteString("    res = e.response\n")
	buf.WriteString("    failed = e\n")
	buf.WriteString("  }\n\n")

	checks := g.setErrorChecks(p)
	if p.q.Returns != nil {
		// The one call this returns is the whole of the answer, so a failure
		// there leaves nothing to hand back.
		fmt.Fprintf(buf, "  if (failed && !answered(res, %s)) throw failed\n",
			strconv.Quote(p.q.Returns.ID))
		fmt.Fprintf(buf, "  const out = decode<%s>(req, res, %s)\n",
			p.returnType, strconv.Quote(p.q.Returns.ID))
	} else {
		// A call the server would not run is left out rather than taking the
		// others with it, so what reaches a caller reading the error is a
		// Partial of this.
		fmt.Fprintf(buf, "  const out = {\n")
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "    ...(answered(res, %[1]s) ? { %[2]s: decode<%[3]s>(req, res, %[1]s) } : {}),\n",
				strconv.Quote(c.ID), tsMemberName(c.Field), p.calls[c].responseType)
		}
		if p.q.CreatedIDs {
			buf.WriteString("    createdIds: res.createdIds ?? {},\n")
		}
		fmt.Fprintf(buf, "  } as %s\n", p.returnType)
	}
	buf.WriteString("  if (failed) {\n    failed.result = out\n    throw failed\n  }\n")
	if len(checks) > 0 {
		g.writeSetErrorChecks(buf, checks)
	}
	buf.WriteString("  return out\n")
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
		// An argument the caller may leave out is spread in, so that leaving
		// it out leaves the member itself out rather than sending undefined.
		if param := field.OptionalParam(); param != nil {
			held := "p." + tsMemberName(param.Field)
			fmt.Fprintf(buf, "        ...(%s !== undefined ? { %s: %s } : {}),\n",
				held, g.keyExpr(field), held)
			continue
		}
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

// writePages writes the generator that walks the whole of what one request
// returns a window of. TypeScript spells that as an async generator, so a
// caller writes for await over it and the loop is somebody else's.
func (g *QueryGenerator) writePages(buf *bytes.Buffer, p *plan) {
	if p.pagesName == "" {
		return
	}
	buf.WriteString("\n")
	g.writePagesDoc(buf, p)
	fmt.Fprintf(buf, "export async function* %s(client: Client, p: %s): AsyncGenerator<%s> {\n",
		p.pagesName, p.paramsType, p.returnType)

	start := tsMemberName(p.q.PageStart.Field)
	fmt.Fprintf(buf, "  let start = p.%s\n", start)
	buf.WriteString("  for (;;) {\n")
	fmt.Fprintf(buf, "    p.%s = start\n", start)
	fmt.Fprintf(buf, "    const res = await %s(client, p)\n", p.funcName)

	window := "res"
	if p.q.Returns == nil {
		window = "res." + tsMemberName(p.q.Pages.Field)
	}
	fmt.Fprintf(buf, "    const window = %s\n", window)

	switch p.q.PageKind {
	case query.PageQuery:
		ids := tsMemberName(spec.ExportedName(query.IDsProperty))
		// A window with nothing in it is the end of the list rather than a
		// page of it, and handing it back would make every caller check.
		fmt.Fprintf(buf, "    if (window.%s.length === 0) {\n      return\n    }\n", ids)
		buf.WriteString("    yield res\n")
		fmt.Fprintf(buf, "    start = window.%s + window.%s.length\n",
			tsMemberName(spec.ExportedName(query.PositionArgument)), ids)
		// Where the call asked for the total, the end is known without asking
		// for a window that is not there.
		fmt.Fprintf(buf, "    if (window.%[1]s > 0 && start >= window.%[1]s) {\n      return\n    }\n",
			tsMemberName(spec.ExportedName(query.TotalProperty)))

	case query.PageChanges:
		// An answer saying nothing changed still carries the state to go on
		// from, so it is worth handing back.
		buf.WriteString("    yield res\n")
		fmt.Fprintf(buf, "    if (!window.%s) {\n      return\n    }\n",
			tsMemberName(spec.ExportedName(query.HasMoreChangesProperty)))
		fmt.Fprintf(buf, "    start = window.%s\n", tsMemberName(spec.ExportedName(query.NewStateProperty)))
	}

	buf.WriteString("  }\n")
	buf.WriteString("}\n")
}

// writePagesDoc writes the generator's documentation.
func (g *QueryGenerator) writePagesDoc(buf *bytes.Buffer, p *plan) {
	doc := fmt.Sprintf("%s walks the whole of what %s returns one part of, calling it again for each part until there is none left.",
		p.pagesName, p.funcName)
	doc += fmt.Sprintf("\n\nIt starts from the %s the parameters carry and works the next one out from each answer. ",
		p.q.PageStart.Name)
	switch p.q.PageKind {
	case query.PageQuery:
		doc += "A window with nothing in it ends the walk rather than being yielded, so everything it yields holds something; " +
			"where the call asked for the total, the walk ends without asking for a window that is not there."
	case query.PageChanges:
		doc += "An answer saying nothing changed is still yielded, since it carries the state to go on from, and the walk ends when the server says there is no more."
	}
	doc += "\n\nA failure throws, as it does from the query itself, and leaving the loop early sends no further request."
	shared.WriteComment(buf, "", doc)
}
