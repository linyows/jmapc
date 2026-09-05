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
	g.writeArgVars(buf, p)
	g.writeRequest(buf, p)

	buf.WriteString("\tresp, err := c.Do(ctx, req)\n")
	// A method call the server would not run does not fail the request: the
	// calls around it may still have answered, and err carries what went
	// wrong, so the response is read and returned with it.
	buf.WriteString("\tif resp == nil {\n\t\treturn nil, err\n\t}\n\n")

	fmt.Fprintf(buf, "\tvar out %s\n", p.returnType)
	if p.q.Returns != nil {
		// The one call this returns is the whole of the answer, so a failure
		// there leaves nothing to return. What went wrong is already in err,
		// where the server reported it.
		fmt.Fprintf(buf, "\tif e := resp.Decode(%q, &out); e != nil {\n", p.q.Returns.ID)
		buf.WriteString("\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		buf.WriteString("\t\treturn nil, e\n\t}\n")
	} else {
		// A call the server would not run leaves its result at the zero value
		// rather than taking the others with it, since err already says which
		// calls those were. Anything else wrong with the response is still
		// worth reporting on its own.
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "\tif e := resp.Decode(%q, &out.%s); e != nil && err == nil {\n\t\treturn nil, e\n\t}\n", c.ID, c.Field)
		}
	}
	if p.q.CreatedIDs {
		buf.WriteString("\tout.CreatedIDs = resp.CreatedIDs\n")
	}
	g.writeSetErrorChecks(buf, p)
	buf.WriteString("\treturn &out, err\n")
	buf.WriteString("}\n")
}

// writeSetErrorChecks writes the code that reports the records the server
// refused. A /set answers 200 and lists what it would not create, update, or
// destroy, so nothing above this notices; the response is returned along with
// the error, since the rest of it did happen.
//
// A call the query does not return is decoded here anyway, for its refusals
// alone. Otherwise naming one call in "_returns" would silently exempt the
// others from the check.
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
	// Both levels can fail at once: one call refused a record while another
	// was not run at all, and a caller reading only one of the two would miss
	// the other.
	buf.WriteString("\tif e := failures.Err(); e != nil {\n\t\treturn &out, errors.Join(err, e)\n\t}\n")
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
		doc += "\n\n" + shared.PrimaryAccountPhrase(p.sessionCapabilities)
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

// writeArgVars writes the argument objects that cannot be stated as one
// literal, because a member of each is only there when the caller supplied it.
func (g *QueryGenerator) writeArgVars(buf *bytes.Buffer, p *plan) {
	for _, c := range p.q.Calls {
		if !c.HasOptionalArgs() {
			continue
		}
		name := argsVar(c)
		fmt.Fprintf(buf, "\t%s := map[string]any{\n", name)
		if expr := p.calls[c].accountIDExpr; expr != "" {
			fmt.Fprintf(buf, "\t\t%q: %s,\n", query.AccountIDArgument, expr)
		}
		for _, field := range c.Args.Fields {
			if field.OptionalParam() != nil {
				continue
			}
			fmt.Fprintf(buf, "\t\t%s: %s,\n", g.keyExpr(field), g.expr(field.Value, "\t\t"))
		}
		buf.WriteString("\t}\n")
		for _, field := range c.Args.Fields {
			param := field.OptionalParam()
			if param == nil {
				continue
			}
			held := "p." + param.Field
			value := held
			if strings.HasPrefix(param.GoType(g.Qualifier), "*") {
				value = "*" + held
			}
			fmt.Fprintf(buf, "\tif %s != nil {\n\t\t%s[%s] = %s\n\t}\n", held, name, g.keyExpr(field), value)
		}
		buf.WriteString("\n")
	}
}

// argsVar names the variable holding a call's arguments, for a call whose
// arguments are built rather than stated.
func argsVar(c *query.Call) string {
	field := c.Field
	return strings.ToLower(field[:1]) + field[1:] + "Args"
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
	if c.HasOptionalArgs() {
		fmt.Fprintf(buf, "\t\t\t{Name: %q, CallID: %q, Args: %s},\n", c.Method.Name, c.ID, argsVar(c))
		return
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
// query watches. The server pushes only that a type has changed, not what
// changed, so the loop calls the query; the runtime holds the connection open,
// reopens it when it drops, and repeats the call while the server reports more
// changes.
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

// writeWatchDoc writes the watching function's documentation, which has to
// state what the loop does with the query it is built on: where it starts,
// what it returns, and when it stops.
func (g *QueryGenerator) writeWatchDoc(buf *bytes.Buffer, p *plan) {
	dataType := p.q.Watches.Method.DataType
	doc := fmt.Sprintf("%s follows the changes to %s, calling %s whenever the server reports any and passing the result to fn.",
		p.watchName, dataType, p.q.Name)
	doc += fmt.Sprintf("\n\nIt starts from the %s the parameters carry, which is the state a previous answer reported, and it continues from the state each answer reports. "+
		"A server that returns only part of what changed is called again until it reports no more.",
		p.q.WatchState.Name)
	doc += "\n\nIt runs until the context ends, which is the error it returns; an error from fn stops it and is returned unchanged. " +
		"A dropped connection is not an error: the loop opens another, resuming from the last event delivered, and requests the changes made while no connection was open."
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

// writePages writes the function that walks the whole of what one request
// returns a window of. A /query answers with the window the caller asked for
// and says where it sits; a /changes answers with as much as the server cares
// to and says whether there is more. Either way the next request is worked out
// from the last answer, which is what the loop does.
func (g *QueryGenerator) writePages(buf *bytes.Buffer, p *plan) {
	if p.pagesName == "" {
		return
	}
	g.writePagesDoc(buf, p)
	fmt.Fprintf(buf, "func %s(ctx context.Context, c *%sClient, p %s) iter.Seq2[*%s, error] {\n",
		p.pagesName, g.Qualifier, p.paramsType, p.returnType)
	fmt.Fprintf(buf, "\treturn func(yield func(*%s, error) bool) {\n", p.returnType)

	start := p.q.PageStart.Field
	fmt.Fprintf(buf, "\t\tstart := p.%s\n", start)
	buf.WriteString("\t\tfor {\n")
	fmt.Fprintf(buf, "\t\t\tp.%s = start\n", start)
	fmt.Fprintf(buf, "\t\t\tres, err := %s(ctx, c, p)\n", p.q.Name)
	buf.WriteString("\t\t\tif err != nil {\n\t\t\t\tyield(nil, err)\n\t\t\t\treturn\n\t\t\t}\n")

	window := "res"
	if p.q.Returns == nil {
		window = "&res." + p.q.Pages.Field
	}
	fmt.Fprintf(buf, "\t\t\twindow := %s\n", window)

	switch p.q.PageKind {
	case query.PageQuery:
		ids := spec.ExportedName(query.IDsProperty)
		// A window with nothing in it is the end of the list rather than a
		// page of it, and handing it back would make every caller check.
		fmt.Fprintf(buf, "\t\t\tif len(window.%s) == 0 {\n\t\t\t\treturn\n\t\t\t}\n", ids)
		buf.WriteString("\t\t\tif !yield(res, nil) {\n\t\t\t\treturn\n\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\tstart = %[1]sInt(window.%[2]s) + %[1]sInt(len(window.%[3]s))\n",
			g.Qualifier, spec.ExportedName(query.PositionArgument), ids)
		// Where the call asked for the total, the end is known without asking
		// for a window that is not there.
		fmt.Fprintf(buf, "\t\t\tif window.%[1]s > 0 && %[2]sUnsignedInt(start) >= window.%[1]s {\n\t\t\t\treturn\n\t\t\t}\n",
			spec.ExportedName(query.TotalProperty), g.Qualifier)

	case query.PageChanges:
		// An answer saying nothing changed still carries the state to go on
		// from, so it is worth handing back.
		buf.WriteString("\t\t\tif !yield(res, nil) {\n\t\t\t\treturn\n\t\t\t}\n")
		fmt.Fprintf(buf, "\t\t\tif !window.%s {\n\t\t\t\treturn\n\t\t\t}\n",
			spec.ExportedName(query.HasMoreChangesProperty))
		fmt.Fprintf(buf, "\t\t\tstart = window.%s\n", spec.ExportedName(query.NewStateProperty))
	}

	buf.WriteString("\t\t}\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")
}

// writePagesDoc writes the pager's documentation, which has to state where the
// loop starts, what each step returns, and what ends it.
func (g *QueryGenerator) writePagesDoc(buf *bytes.Buffer, p *plan) {
	doc := fmt.Sprintf("%s walks the whole of what %s returns one part of, calling it again for each part until none is left.",
		p.pagesName, p.q.Name)
	doc += fmt.Sprintf("\n\nIt starts from the %s the parameters carry and derives the next one from each answer. ",
		p.q.PageStart.Name)
	switch p.q.PageKind {
	case query.PageQuery:
		doc += "An empty window ends the walk instead of being yielded, so every result yielded holds at least one record; " +
			"where the call asked for the total, the walk ends without requesting a window past it."
	case query.PageChanges:
		doc += "An answer reporting no changes is still yielded, since it carries the state to continue from, and the walk ends when the server reports no more."
	}
	doc += "\n\nAn error ends the walk and is yielded with a nil result, so a range over it checks the error on each iteration. " +
		"Breaking out of the range stops it, and sends no further request."
	shared.WriteComment(buf, "", doc)
}
