package rust

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

const (
	// lineWidth is the column rustfmt wraps at, and so the width a signature
	// has to fit in before it is broken up.
	lineWidth = 100
	// chainWidth is the width rustfmt keeps a method chain within before it
	// puts each call on a line of its own.
	chainWidth = 60
	// attrWidth is the width rustfmt keeps the arguments of an attribute
	// within before it puts each on a line of its own.
	attrWidth = 70
)

// writeFunc writes the function that sends the query and reads its response.
func (g *QueryGenerator) writeFunc(buf *bytes.Buffer, p *plan) {
	g.writeFuncDoc(buf, p)
	g.writeSignature(buf, p)

	for _, capability := range p.sessionCapabilities {
		chain := fmt.Sprintf("client.primary_account_id(%s).await?", quote(capability))
		if len(chain) < chainWidth {
			fmt.Fprintf(buf, "    let %s = %s;\n", accountIDVar(capability), chain)
			continue
		}
		fmt.Fprintf(buf, "    let %s = client\n        .primary_account_id(%s)\n        .await?;\n",
			accountIDVar(capability), quote(capability))
	}
	if len(p.sessionCapabilities) > 0 {
		buf.WriteString("\n")
	}

	g.writeRequest(buf, p)

	buf.WriteString("    let res = client.request(&req).await?;\n\n")

	// Where nothing can be refused there is nothing to hold the response for,
	// so it is returned as it is read.
	checks := g.setErrorChecks(p)
	if p.q.Returns != nil {
		if len(checks) == 0 {
			fmt.Fprintf(buf, "    decode::<%s>(&req, &res, %s)\n}\n", p.returnType, quote(p.q.Returns.ID))
			return
		}
		fmt.Fprintf(buf, "    let out = decode::<%s>(&req, &res, %s)?;\n",
			p.returnType, quote(p.q.Returns.ID))
	} else {
		fmt.Fprintf(buf, "    let out = %s {\n", p.resultType)
		for _, c := range p.q.Calls {
			fmt.Fprintf(buf, "        %s: decode::<%s>(&req, &res, %s)?,\n",
				spec.RustFieldName(c.Field), p.calls[c].responseType, quote(c.ID))
		}
		if p.q.CreatedIDs {
			buf.WriteString("        created_ids: res.created_ids.clone().unwrap_or_default(),\n")
		}
		buf.WriteString("    };\n")
	}
	g.writeSetErrorChecks(buf, checks)
	buf.WriteString("\n    Ok(out)\n}\n")
}

// writeSignature writes the function's signature, on one line where it fits and
// broken up where it does not, as rustfmt would.
func (g *QueryGenerator) writeSignature(buf *bytes.Buffer, p *plan) {
	args := []string{"client: &Client<T>"}
	if p.paramsType != "" {
		args = append(args, "p: "+p.paramsType)
	}
	if p.q.CreatedIDs {
		args = append(args, "created_ids: Option<BTreeMap<Id, Id>>")
	}
	head := fmt.Sprintf("pub async fn %s<T: Transport>(", p.funcName)
	tail := fmt.Sprintf(") -> Result<%s, Error> {", p.returnType)
	if line := head + strings.Join(args, ", ") + tail; len(line) <= lineWidth {
		buf.WriteString(line + "\n")
		return
	}
	buf.WriteString(head + "\n")
	for _, arg := range args {
		fmt.Fprintf(buf, "    %s,\n", arg)
	}
	buf.WriteString(tail + "\n")
}

// setErrorCheck is one method call whose response reports records the server
// refused.
type setErrorCheck struct {
	call   *query.Call
	object string // the expression holding the response
	decode string // the type to read it from, empty where out holds it
	groups []setErrorGroup
}

// setErrorGroup is one response property the refusals arrive in.
type setErrorGroup struct {
	kind     string
	field    string
	optional bool
}

// setErrorChecks finds the calls whose responses report per-record failures.
// A call the query does not return is read for its refusals alone, since
// otherwise naming one call in "_returns" would quietly stop the others from
// being checked.
func (g *QueryGenerator) setErrorChecks(p *plan) []setErrorCheck {
	var checks []setErrorCheck
	for i, c := range p.q.Calls {
		fields := g.Spec.SetErrorFields(c.Method.Name)
		if len(fields) == 0 {
			continue
		}
		ch := setErrorCheck{call: c, object: "out"}
		resp, err := g.Spec.ResponseOf(c.Method.Name)
		if err != nil {
			continue
		}
		for _, name := range fields {
			group := setErrorGroup{kind: name, field: spec.RustFieldName(name)}
			if f, ok := resp.Field(name); ok {
				group.optional = f.ParsedType().Nullable
			}
			ch.groups = append(ch.groups, group)
		}
		switch {
		case p.q.Returns == nil:
			ch.object = "out." + spec.RustFieldName(c.Field)
		case c != p.q.Returns:
			ch.object = fmt.Sprintf("refused%d", i)
			ch.decode = p.calls[c].responseType
		}
		checks = append(checks, ch)
	}
	return checks
}

// writeSetErrorChecks writes the code that fails where the server refused a
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
		fmt.Fprintf(buf, "    let %s = decode::<%s>(&req, &res, %s)?;\n",
			ch.object, ch.decode, quote(ch.call.ID))
	}
	buf.WriteString("    let mut failures: Vec<SetFailure> = Vec::new();\n")
	for _, ch := range checks {
		entries := make([]string, len(ch.groups))
		for i, group := range ch.groups {
			access := fmt.Sprintf("%s.%s", ch.object, group.field)
			if group.optional {
				access += ".as_ref()"
			} else {
				access = "Some(&" + access + ")"
			}
			entries[i] = fmt.Sprintf("(%s, %s)", quote(group.kind), access)
		}
		fmt.Fprintf(buf, "    collect_set_errors(\n        %s,\n        %s,\n",
			quote(ch.call.Method.Name), quote(ch.call.ID))
		if one := "        &[" + strings.Join(entries, ", ") + "],"; len(entries) == 1 && len(one) <= lineWidth {
			buf.WriteString(one + "\n")
		} else {
			buf.WriteString("        &[\n")
			for _, entry := range entries {
				fmt.Fprintf(buf, "            %s,\n", entry)
			}
			buf.WriteString("        ],\n")
		}
		buf.WriteString("        &mut failures,\n    );\n")
	}
	buf.WriteString("    if !failures.is_empty() {\n")
	buf.WriteString("        return Err(Error::Set(SetErrors::new(failures, out)));\n")
	buf.WriteString("    }\n")
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
	writeDoc(buf, "", doc)
}

// writeRequest writes the request the function sends.
func (g *QueryGenerator) writeRequest(buf *bytes.Buffer, p *plan) {
	buf.WriteString("    let req = Request {\n")
	g.writeUsing(buf, p)

	// A list of one collapses onto the line that names it, which is how
	// rustfmt lays out a vec! whose single element is itself a block. A call
	// with a comment keeps its own line, since the comment belongs above it.
	single := len(p.q.Calls) == 1 && p.q.Calls[0].Comment == ""
	if single {
		buf.WriteString("        method_calls: vec![")
		g.writeInvocation(buf, p, p.q.Calls[0], "        ", true)
	} else {
		buf.WriteString("        method_calls: vec![\n")
		for _, c := range p.q.Calls {
			g.writeInvocation(buf, p, c, "            ", false)
		}
		buf.WriteString("        ],\n")
	}

	if p.q.CreatedIDs {
		buf.WriteString("        created_ids,\n")
	} else {
		buf.WriteString("        created_ids: None,\n")
	}
	buf.WriteString("    };\n\n")
}

// writeUsing writes the capabilities the request declares.
func (g *QueryGenerator) writeUsing(buf *bytes.Buffer, p *plan) {
	uris := make([]string, len(p.q.Using))
	for i, uri := range p.q.Using {
		uris[i] = quote(uri) + ".to_string()"
	}
	if line := "        using: vec![" + strings.Join(uris, ", ") + "],"; len(line) <= lineWidth {
		buf.WriteString(line + "\n")
		return
	}
	buf.WriteString("        using: vec![\n")
	for _, uri := range uris {
		fmt.Fprintf(buf, "            %s,\n", uri)
	}
	buf.WriteString("        ],\n")
}

// writeInvocation writes one method call of the request. indent is where the
// Invocation itself sits; collapsed says that the line it starts on has already
// been opened by the vec! holding it, and that it closes that vec! too.
func (g *QueryGenerator) writeInvocation(buf *bytes.Buffer, p *plan, c *query.Call, indent string, collapsed bool) {
	if !collapsed {
		if c.Comment != "" {
			shared.WriteComment(buf, indent, c.Comment)
		}
		buf.WriteString(indent)
	}
	buf.WriteString("Invocation(\n")

	args := indent + "    "
	members := args + "    "
	fmt.Fprintf(buf, "%s%s.to_string(),\n", args, quote(c.Method.Name))

	accountID := p.calls[c].accountIDVar
	if accountID == "" && len(c.Args.Fields) == 0 {
		fmt.Fprintf(buf, "%sjson!({}),\n", args)
	} else {
		fmt.Fprintf(buf, "%sjson!({\n", args)
		if accountID != "" {
			fmt.Fprintf(buf, "%s%s: %s,\n", members, quote(query.AccountIDArgument), accountID)
		}
		for _, field := range c.Args.Fields {
			fmt.Fprintf(buf, "%s%s: %s,\n", members, g.keyExpr(field), g.expr(field.Value, members))
		}
		fmt.Fprintf(buf, "%s}),\n", args)
	}
	fmt.Fprintf(buf, "%s%s.to_string(),\n", args, quote(c.ID))
	if collapsed {
		fmt.Fprintf(buf, "%s)],\n", indent)
		return
	}
	fmt.Fprintf(buf, "%s),\n", indent)
}

// keyExpr renders an argument name, which a query may build from parameters. A
// name the query states outright is a string literal; one it builds is a Rust
// expression, which the json! macro takes in parentheses.
func (g *QueryGenerator) keyExpr(f query.ObjectField) string {
	if len(f.KeySegments) == 0 {
		return quote(f.Key)
	}
	if len(f.KeySegments) == 1 && f.KeySegments[0].Param != nil {
		return "(" + paramExpr(f.KeySegments[0].Param) + ".to_string())"
	}
	var format strings.Builder
	var args []string
	for _, seg := range f.KeySegments {
		if seg.Param == nil {
			format.WriteString(strings.ReplaceAll(strings.ReplaceAll(seg.Text, "{", "{{"), "}", "}}"))
			continue
		}
		format.WriteString("{}")
		args = append(args, paramExpr(seg.Param))
	}
	return fmt.Sprintf("(format!(%s, %s))", quote(format.String()), strings.Join(args, ", "))
}

// paramExpr names the field of the parameters struct a parameter reads from.
func paramExpr(p *query.Param) string {
	return "p." + spec.RustFieldName(p.Field)
}

// expr renders one argument value.
func (g *QueryGenerator) expr(n query.Node, indent string) string {
	switch v := n.(type) {
	case *query.ResultRef:
		return fmt.Sprintf("{%q: %s, %q: %s, %q: %s}",
			"resultOf", quote(v.Ref.ResultOf), "name", quote(v.Ref.Name), "path", quote(v.Ref.Path))

	case *query.ParamRef:
		return paramExpr(v.Param)

	case *query.Literal:
		return literalExpr(v.JSON)

	case *query.Object:
		if !v.HasParam() {
			return literalExpr(v.Raw)
		}
		var b strings.Builder
		b.WriteString("{\n")
		for _, f := range v.Fields {
			fmt.Fprintf(&b, "%s    %s: %s,\n", indent, g.keyExpr(f), g.expr(f.Value, indent+"    "))
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
			fmt.Fprintf(&b, "%s    %s,\n", indent, g.expr(item, indent+"    "))
		}
		b.WriteString(indent + "]")
		return b.String()
	}
	return "null"
}

// writePages writes the walk over the whole of what one request returns a
// part of. Rust has no stream to hand back without a crate to define one, and
// the generated code takes nothing but serde, so the walk is a value that
// remembers where it is and a method that asks for the next part.
func (g *QueryGenerator) writePages(buf *bytes.Buffer, p *plan) {
	if p.pagesType == "" {
		return
	}
	start := spec.RustFieldName(p.q.PageStart.Field)
	startType := p.q.PageStart.ValueType().RustType()

	buf.WriteString("\n")
	g.writePagesDoc(buf, p, "///")
	fmt.Fprintf(buf, "pub struct %s {\n", p.pagesType)
	fmt.Fprintf(buf, "    params: %s,\n", p.paramsType)
	fmt.Fprintf(buf, "    start: %s,\n", startType)
	buf.WriteString("    done: bool,\n")
	buf.WriteString("}\n\n")

	shared.WriteCommentMarker(buf, "", "///", fmt.Sprintf(
		"%s starts a walk from the %s the parameters carry. Nothing is sent until the first call to next.",
		p.pagesFunc, p.q.PageStart.Name))
	fmt.Fprintf(buf, "pub fn %s(p: %s) -> %s {\n", p.pagesFunc, p.paramsType, p.pagesType)
	fmt.Fprintf(buf, "    %s {\n", p.pagesType)
	fmt.Fprintf(buf, "        start: p.%s.clone(),\n", start)
	buf.WriteString("        params: p,\n")
	buf.WriteString("        done: false,\n")
	buf.WriteString("    }\n}\n\n")

	fmt.Fprintf(buf, "impl %s {\n", p.pagesType)
	shared.WriteCommentMarker(buf, "    ", "///",
		"next asks for the part after the one before, and answers with None where there is none left.")
	head := "    pub async fn next<T: Transport>(&mut self, client: &Client<T>)"
	tail := fmt.Sprintf(" -> Result<Option<%s>, Error> {", p.returnType)
	if len(head+tail) <= lineWidth {
		buf.WriteString(head + tail + "\n")
	} else {
		buf.WriteString("    pub async fn next<T: Transport>(\n        &mut self,\n        client: &Client<T>,\n")
		fmt.Fprintf(buf, "    )%s\n", tail)
	}
	buf.WriteString("        if self.done {\n            return Ok(None);\n        }\n")
	fmt.Fprintf(buf, "        self.params.%s = self.start.clone();\n", start)
	fmt.Fprintf(buf, "        let res = %s(client, self.params.clone()).await?;\n", p.funcName)

	window := "&res"
	if p.q.Returns == nil {
		window = "&res." + spec.RustFieldName(p.q.Pages.Field)
	}
	fmt.Fprintf(buf, "        let window = %s;\n", window)

	switch p.q.PageKind {
	case query.PageQuery:
		ids := spec.RustFieldName(spec.ExportedName(query.IDsProperty))
		// A window with nothing in it is the end of the list rather than a
		// part of it, and handing it back would make every caller check.
		fmt.Fprintf(buf, "        if window.%s.is_empty() {\n            self.done = true;\n            return Ok(None);\n        }\n", ids)
		fmt.Fprintf(buf, "        self.start = window.%s as %s + window.%s.len() as %s;\n",
			spec.RustFieldName(spec.ExportedName(query.PositionArgument)), startType, ids, startType)
		// Where the call asked for the total, the end is known without asking
		// for a part that is not there.
		fmt.Fprintf(buf, "        if window.%[1]s > 0 && self.start as u64 >= window.%[1]s {\n            self.done = true;\n        }\n",
			spec.RustFieldName(spec.ExportedName(query.TotalProperty)))

	case query.PageChanges:
		// An answer saying nothing changed still carries the state to go on
		// from, so it is worth handing back.
		fmt.Fprintf(buf, "        if window.%s {\n", spec.RustFieldName(spec.ExportedName(query.HasMoreChangesProperty)))
		fmt.Fprintf(buf, "            self.start = window.%s.clone();\n", spec.RustFieldName(spec.ExportedName(query.NewStateProperty)))
		buf.WriteString("        } else {\n            self.done = true;\n        }\n")
	}

	buf.WriteString("        Ok(Some(res))\n")
	buf.WriteString("    }\n}\n")
}

// writePagesDoc writes the walk's documentation, on whichever item carries it.
func (g *QueryGenerator) writePagesDoc(buf *bytes.Buffer, p *plan, marker string) {
	doc := fmt.Sprintf("%s walks the whole of what %s returns one part of, calling it again for each part until there is none left.",
		p.pagesType, p.funcName)
	switch p.q.PageKind {
	case query.PageQuery:
		doc += "\n\nA window with nothing in it ends the walk rather than being handed back, so every part it answers with holds something; " +
			"where the call asked for the total, the walk ends without asking for a window that is not there."
	case query.PageChanges:
		doc += "\n\nAn answer saying nothing changed is still handed back, since it carries the state to go on from, and the walk ends when the server says there is no more."
	}
	shared.WriteCommentMarker(buf, "", marker, doc)
}
