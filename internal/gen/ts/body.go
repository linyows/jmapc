package ts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

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

	buf.WriteString("  const res = await client.request(req)\n\n")
	if p.q.Returns != nil {
		fmt.Fprintf(buf, "  return decode<%s>(req, res, %s)\n",
			p.returnType, strconv.Quote(p.q.Returns.ID))
	} else {
		fmt.Fprintf(buf, "  return {\n")
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "    %s: decode<%s>(req, res, %s),\n",
				tsMemberName(c.Field), p.calls[c].responseType, strconv.Quote(c.ID))
		}
		if p.q.CreatedIDs {
			buf.WriteString("    createdIds: res.createdIds ?? {},\n")
		}
		buf.WriteString("  }\n")
	}
	buf.WriteString("}\n")
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
		joinMethods(methods), roundTripPhrase(len(p.q.Calls)))
	if p.q.Returns != nil {
		doc += fmt.Sprintf(" It returns the response to the %s call.", p.q.Returns.Method.Name)
	}
	if len(p.sessionCapabilities) > 0 {
		doc += "\n\nThe query does not say which account to use, so the primary account of the session is used, which costs a session lookup on first use."
	}
	if p.q.CreatedIDs {
		doc += "\n\nIt takes the creation ids of an earlier request and reports its own, so that a reference to something created there still resolves here."
	}
	writeComment(buf, "", doc)
}

// joinMethods renders a list of method names for prose.
func joinMethods(names []string) string {
	switch len(names) {
	case 0:
		return "no calls"
	case 1:
		return "one " + names[0] + " call"
	case 2:
		return names[0] + " and " + names[1] + " calls"
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1] + " calls"
	}
}

// roundTripPhrase describes what batching the calls buys.
func roundTripPhrase(n int) string {
	if n <= 1 {
		return "the server is asked once"
	}
	return fmt.Sprintf("%d dependent calls cost one round trip", n)
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
		writeComment(buf, "      ", c.Comment)
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

// headerPropertyDoc describes a property naming one header field of a message.
func headerPropertyDoc(h *spec.HeaderProperty) string {
	which := "The " + h.Name + " header field"
	if h.All {
		which = "Every " + h.Name + " header field in the message, in the order they appear"
	} else {
		which += ", or the last of them where the message has several"
	}
	switch h.Form {
	case "", "asRaw":
		return which + ", as it appears in the message."
	case "asText":
		return which + ", decoded and unfolded into text."
	case "asAddresses":
		return which + ", parsed as a list of addresses."
	case "asGroupedAddresses":
		return which + ", parsed as a list of addresses, keeping the groups they were written in."
	case "asMessageIds":
		return which + ", parsed as message ids, without their angle brackets."
	case "asDate":
		return which + ", parsed as a date."
	case "asURLs":
		return which + ", parsed as a list of URLs."
	}
	return which + "."
}

// dynamicPropertyDoc describes a property whose meaning comes from the server.
func dynamicPropertyDoc(name string) string {
	switch {
	case strings.HasPrefix(name, "digest:"):
		return "The digest of the blob under the " + strings.TrimPrefix(name, "digest:") + " algorithm, as base64."
	case name == "data":
		return "The blob's octets. The server returns them under data:asText or " +
			"data:asBase64, whichever suits what they hold, so this property itself does not come back."
	}
	return "The " + name + " property, whose meaning the server decides."
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) { sort.Strings(s) }
