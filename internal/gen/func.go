package gen

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

// capabilityConstants maps the capability URIs the runtime package names to
// those names, so that generated code reads as it would if written by hand.
var capabilityConstants = map[string]string{
	spec.CapabilityCore:            "CapabilityCore",
	spec.CapabilityMail:            "CapabilityMail",
	spec.CapabilitySubmission:      "CapabilitySubmission",
	spec.CapabilityVacation:        "CapabilityVacation",
	spec.CapabilityContacts:        "CapabilityContacts",
	spec.CapabilityCalendars:       "CapabilityCalendars",
	spec.CapabilityCalendarsParse:  "CapabilityCalendarsParse",
	spec.CapabilityAvailability:    "CapabilityAvailability",
	spec.CapabilityPrincipals:      "CapabilityPrincipals",
	spec.CapabilityPrincipalsOwner: "CapabilityPrincipalsOwner",
	spec.CapabilitySMIMEVerify:     "CapabilitySMIMEVerify",
	spec.CapabilityBlob:            "CapabilityBlob",
	spec.CapabilityQuota:           "CapabilityQuota",
	spec.CapabilitySieve:           "CapabilitySieve",
	spec.CapabilityMDN:             "CapabilityMDN",
}

// writeFunc writes the function that sends the query and decodes its response.
func (g *QueryGenerator) writeFunc(buf *bytes.Buffer, p *plan) {
	g.writeFuncDoc(buf, p)

	sig := fmt.Sprintf("func %s(ctx context.Context, c *%sClient", p.q.Name, g.Qualifier)
	if p.paramsType != "" {
		sig += fmt.Sprintf(", p %s", p.paramsType)
	}
	if p.q.CreatedIDs {
		sig += fmt.Sprintf(", createdIDs map[%[1]sID]%[1]sID", g.Qualifier)
	}
	fmt.Fprintf(buf, "%s) (*%s, error) {\n", sig, p.returnType)

	g.writeSessionLookups(buf, p)
	g.writeRequest(buf, p)

	buf.WriteString("\tresp, err := c.Do(ctx, req)\n")
	buf.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n\n")

	fmt.Fprintf(buf, "\tvar out %s\n", p.returnType)
	if p.q.Returns != nil {
		fmt.Fprintf(buf, "\tif err := resp.Decode(%q, &out); err != nil {\n\t\treturn nil, err\n\t}\n", p.q.Returns.ID)
	} else {
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "\tif err := resp.Decode(%q, &out.%s); err != nil {\n\t\treturn nil, err\n\t}\n", c.ID, c.Field)
		}
	}
	if p.q.CreatedIDs {
		buf.WriteString("\tout.CreatedIDs = resp.CreatedIDs\n")
	}
	g.writeSetErrorChecks(buf, p)
	buf.WriteString("\treturn &out, nil\n")
	buf.WriteString("}\n")
}

// writeSetErrorChecks writes the code that reports the records the server
// refused. A /set answers 200 and lists what it would not create, update, or
// destroy, so nothing above this notices; the response is returned along with
// the error, since the rest of it did happen.
//
// A call the query does not return is decoded here anyway, for its refusals
// alone. Otherwise naming one call in "_returns" would quietly stop the others
// from being checked.
func (g *QueryGenerator) writeSetErrorChecks(buf *bytes.Buffer, p *plan) {
	type check struct {
		call   *query.Call
		prefix string
		decode string // the type to decode into first, empty where out holds it
		fields []string
	}
	var checks []check
	for _, c := range p.q.Calls {
		fields := g.Spec.SetErrorFields(c.Method.Name)
		if len(fields) == 0 {
			continue
		}
		ch := check{call: c, fields: fields, prefix: "out."}
		switch {
		case p.q.Returns == nil:
			ch.prefix = "out." + c.Field + "."
		case c != p.q.Returns:
			ch.decode = p.calls[c].responseType
			ch.prefix = "" // named below, once the variable exists
		}
		checks = append(checks, ch)
	}
	if len(checks) == 0 {
		return
	}
	buf.WriteString("\n")
	for i := range checks {
		if checks[i].decode == "" {
			continue
		}
		name := fmt.Sprintf("refused%d", i)
		fmt.Fprintf(buf, "\tvar %s %s\n", name, checks[i].decode)
		fmt.Fprintf(buf, "\tif err := resp.Decode(%q, &%s); err != nil {\n\t\treturn nil, err\n\t}\n",
			checks[i].call.ID, name)
		checks[i].prefix = name + "."
	}
	fmt.Fprintf(buf, "\tvar failures %sSetErrors\n", g.Qualifier)
	for _, ch := range checks {
		fmt.Fprintf(buf, "\tfailures.Collect(%q, %q, map[string]map[%[3]sID]%[3]sSetError{\n",
			ch.call.Method.Name, ch.call.ID, g.Qualifier)
		for _, name := range ch.fields {
			fmt.Fprintf(buf, "\t\t%q: %s%s,\n", name, ch.prefix, spec.ExportedName(name))
		}
		buf.WriteString("\t})\n")
	}
	buf.WriteString("\tif err := failures.Err(); err != nil {\n\t\treturn &out, err\n\t}\n")
}

// writeFuncDoc writes the generated function's documentation, using what the
// query says about itself and filling in what it does not.
func (g *QueryGenerator) writeFuncDoc(buf *bytes.Buffer, p *plan) {
	doc := strings.TrimSpace(p.q.Doc)
	if doc == "" {
		doc = fmt.Sprintf("%s sends the JMAP request in %s.", p.q.Name, p.q.Path)
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
		doc += "\n\nIt takes the creation ids of an earlier request and reports its own, so that a reference to something created there still resolves here. " +
			"Pass nil where there is no earlier request."
	}
	shared.WriteComment(buf, "", doc)
}

// writeSessionLookups writes the code that resolves the account ids the query
// left unstated.
func (g *QueryGenerator) writeSessionLookups(buf *bytes.Buffer, p *plan) {
	if len(p.sessionCapabilities) == 0 {
		return
	}
	buf.WriteString("\tsession, err := c.Session(ctx)\n")
	buf.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	for _, capability := range p.sessionCapabilities {
		fmt.Fprintf(buf, "\t%s, err := session.PrimaryAccountID(%s)\n",
			accountIDVar(capability), g.capabilityExpr(capability))
		buf.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	}
	buf.WriteString("\n")
}

// capabilityExpr renders a capability URI as the runtime constant that names
// it, or as a string literal when there is none.
func (g *QueryGenerator) capabilityExpr(uri string) string {
	if name, ok := capabilityConstants[uri]; ok {
		return g.Qualifier + name
	}
	return strconv.Quote(uri)
}

// writeRequest writes the literal JMAP request the function sends.
func (g *QueryGenerator) writeRequest(buf *bytes.Buffer, p *plan) {
	fmt.Fprintf(buf, "\treq := &%sRequest{\n", g.Qualifier)
	buf.WriteString("\t\tUsing: []string{")
	for i, uri := range p.q.Using {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(g.capabilityExpr(uri))
	}
	buf.WriteString("},\n")
	fmt.Fprintf(buf, "\t\tMethodCalls: []%sInvocation{\n", g.Qualifier)
	for _, c := range p.q.Calls {
		g.writeInvocation(buf, p, c)
	}
	buf.WriteString("\t\t},\n")
	if p.q.CreatedIDs {
		buf.WriteString("\t\tCreatedIDs: createdIDs,\n")
	}
	buf.WriteString("\t}\n\n")
}

// writeInvocation writes one method call of the request.
func (g *QueryGenerator) writeInvocation(buf *bytes.Buffer, p *plan, c *query.Call) {
	if c.Comment != "" {
		shared.WriteComment(buf, "\t\t\t", c.Comment)
	}
	fmt.Fprintf(buf, "\t\t\t{Name: %q, CallID: %q, Args: map[string]any{\n", c.Method.Name, c.ID)
	if expr := p.calls[c].accountIDExpr; expr != "" {
		fmt.Fprintf(buf, "\t\t\t\t%q: %s,\n", query.AccountIDArgument, expr)
	}
	for _, field := range c.Args.Fields {
		fmt.Fprintf(buf, "\t\t\t\t%s: %s,\n", g.keyExpr(field), g.expr(field.Value, "\t\t\t\t"))
	}
	buf.WriteString("\t\t\t}},\n")
}

// expr renders one argument value as a Go expression. A subtree that depends on
// no parameter is emitted as the JSON it already is, which keeps the generated
// code close to the query that produced it.
func (g *QueryGenerator) expr(n query.Node, indent string) string {
	switch v := n.(type) {
	case *query.ResultRef:
		return fmt.Sprintf("%sResultReference{ResultOf: %q, Name: %q, Path: %q}",
			g.Qualifier, v.Ref.ResultOf, v.Ref.Name, v.Ref.Path)

	case *query.ParamRef:
		return "p." + v.Param.Field

	case *query.Literal:
		return literalExpr(v.JSON)

	case *query.Object:
		if !v.HasParam() {
			return rawExpr(v.Raw)
		}
		var b strings.Builder
		b.WriteString("map[string]any{\n")
		for _, f := range v.Fields {
			fmt.Fprintf(&b, "%s\t%s: %s,\n", indent, g.keyExpr(f), g.expr(f.Value, indent+"\t"))
		}
		b.WriteString(indent + "}")
		return b.String()

	case *query.Array:
		if !v.HasParam() {
			return rawExpr(v.Raw)
		}
		var b strings.Builder
		b.WriteString("[]any{\n")
		for _, item := range v.Items {
			fmt.Fprintf(&b, "%s\t%s,\n", indent, g.expr(item, indent+"\t"))
		}
		b.WriteString(indent + "}")
		return b.String()
	}
	return "nil"
}

// keyExpr renders an object member name. Most names are constants, but a query
// may build one from parameters, as a patch does when it points at a property
// keyed by an id the caller chooses.
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
		name := "p." + seg.Param.Field
		if seg.Param.GoType(g.Qualifier) != "string" {
			name = "string(" + name + ")"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, " + ")
}

// literalExpr renders a JSON value the query stated outright. A scalar becomes
// the Go value it denotes, which reads better than a fragment of encoded JSON;
// anything composite is kept as raw JSON so that the generated request matches
// the query line for line.
func literalExpr(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "nil"
	}
	switch trimmed[0] {
	case '{', '[':
		return rawExpr(raw)
	case 'n':
		return "nil"
	case 't':
		return "true"
	case 'f':
		return "false"
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return rawExpr(raw)
		}
		return strconv.Quote(s)
	default:
		return string(trimmed)
	}
}

// rawExpr renders a JSON value as a json.RawMessage, which marshals back into
// the request exactly as the query wrote it.
func rawExpr(raw json.RawMessage) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		compact.Reset()
		compact.Write(raw)
	}
	s := compact.String()
	if !strings.ContainsAny(s, "`\r") {
		return "json.RawMessage(`" + s + "`)"
	}
	return "json.RawMessage(" + strconv.Quote(s) + ")"
}

// nodeHasParam reports whether a node depends on a parameter.
func nodeHasParam(n query.Node) bool { return n.HasParam() }

// writeWatch writes the function that follows the changes to the type the
// query watches. What the server pushes is that a type has moved on, not what
// changed, so the loop asks the query; the runtime holds the connection open,
// reopens it when it drops, and keeps asking while the server says there is
// more.
func (g *QueryGenerator) writeWatch(buf *bytes.Buffer, p *plan) {
	if p.watchName == "" {
		return
	}
	watched := p.q.Watches
	state := p.q.WatchState.Field

	g.writeWatchDoc(buf, p)
	fmt.Fprintf(buf, "func %s(ctx context.Context, c *%sClient, p %s, fn func(context.Context, *%s) error, opts ...%[2]sWatchOption) error {\n",
		p.watchName, g.Qualifier, p.paramsType, p.returnType)

	account := g.watchAccount(buf, p)
	fmt.Fprintf(buf, "\treturn c.Watch(ctx, %s, %q, p.%s, func(ctx context.Context, sinceState string) (string, bool, error) {\n",
		account, watched.Method.DataType, state)
	fmt.Fprintf(buf, "\t\tp.%s = sinceState\n", state)
	fmt.Fprintf(buf, "\t\tres, err := %s(ctx, c, p)\n", p.q.Name)
	buf.WriteString("\t\tif err != nil {\n\t\t\treturn \"\", false, err\n\t\t}\n")
	buf.WriteString("\t\tif err := fn(ctx, res); err != nil {\n\t\t\treturn \"\", false, err\n\t\t}\n")
	changes := "res."
	if p.q.Returns == nil {
		changes += watched.Field + "."
	}
	fmt.Fprintf(buf, "\t\treturn %[1]s%[2]s, %[1]s%[3]s, nil\n",
		changes, spec.ExportedName(query.NewStateProperty), spec.ExportedName(query.HasMoreChangesProperty))
	buf.WriteString("\t}, opts...)\n")
	buf.WriteString("}\n")
}

// writeWatchDoc writes the watching function's documentation, which has to say
// what the loop does with the query it is built on: where it starts, what it
// hands back, and when it gives up.
func (g *QueryGenerator) writeWatchDoc(buf *bytes.Buffer, p *plan) {
	dataType := p.q.Watches.Method.DataType
	doc := fmt.Sprintf("%s follows the changes to %s, calling %s whenever the server says there are any and passing the result to fn.",
		p.watchName, dataType, p.q.Name)
	doc += fmt.Sprintf("\n\nIt starts from the %s the parameters carry, which is the state a previous answer left the caller at, and it goes on from the state each answer reports. "+
		"A server that answers with only part of what changed is asked again until it says there is no more.",
		p.q.WatchState.Name)
	doc += "\n\nIt runs until the context ends, which is the error it returns; an error from fn stops it and comes back as it was. " +
		"A dropped connection is not an error: the loop opens another, resuming where it left off, and asks what it missed while there was none."
	shared.WriteComment(buf, "", doc)
}

// watchAccount writes whatever is needed to name the account a watch listens
// for, and returns the expression naming it. The events are keyed by account,
// so a watch has to know which one before it makes any request at all.
func (g *QueryGenerator) watchAccount(buf *bytes.Buffer, p *plan) string {
	watched := p.q.Watches
	if expr := p.calls[watched].accountIDExpr; expr != "" {
		capability := watched.Method.Capability
		if capability == "" {
			capability = spec.CapabilityCore
		}
		buf.WriteString("\tsession, err := c.Session(ctx)\n")
		buf.WriteString("\tif err != nil {\n\t\treturn err\n\t}\n")
		fmt.Fprintf(buf, "\t%s, err := session.PrimaryAccountID(%s)\n", expr, g.capabilityExpr(capability))
		buf.WriteString("\tif err != nil {\n\t\treturn err\n\t}\n\n")
		return expr
	}
	account, _ := watched.Args.Find(query.AccountIDArgument)
	if param, ok := account.(*query.ParamRef); ok {
		return "p." + param.Param.Field
	}
	return fmt.Sprintf("%sID(%s)", g.Qualifier, g.expr(account, "\t"))
}
