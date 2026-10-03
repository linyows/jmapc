package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"path"
	"strconv"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
)

// VerifyFileName is the file the package's Verify function is generated into.
const VerifyFileName = "verify_gen.go"

// VerifyFunc is the name of the generated function that checks every request
// in the package against the session. A request cannot take it, since its
// function would be generated under the same name.
const VerifyFunc = "Verify"

// verifyFile writes Verify, with what each request needs of the server as far
// as the request file says: the capabilities it declares, how many calls it
// makes, and the capabilities whose primary account fills in an account it
// leaves out.
func (g *RequestGenerator) verifyFile(plans []*plan) ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s\n", shared.GeneratedBanner)
	fmt.Fprintf(&buf, "// Source: %s\n\n", requestsDir(plans))
	fmt.Fprintf(&buf, "package %s\n\n", g.Package)
	buf.WriteString("import (\n\t\"context\"\n\n\t\"github.com/linyows/jmapc\"\n)\n\n")

	fmt.Fprintf(&buf, "// %s checks every request in this package against the session, and\n", VerifyFunc)
	buf.WriteString("// reports everything that would stop one of them from being sent. Call it as\n")
	buf.WriteString("// the program starts, to learn then rather than when a request is first sent.\n")
	buf.WriteString("// See jmapc.Client.Verify for what it checks.\n")
	fmt.Fprintf(&buf, "func %s(ctx context.Context, c *%sClient) error {\n", VerifyFunc, g.Qualifier)
	buf.WriteString("\treturn c.Verify(ctx, requestNeeds...)\n}\n\n")

	buf.WriteString("// requestNeeds is what each request in this package needs of the server.\n")
	fmt.Fprintf(&buf, "var requestNeeds = []%sRequestNeeds{\n", g.Qualifier)
	for _, p := range plans {
		fmt.Fprintf(&buf, "\t{\n\t\tName: %s,\n", strconv.Quote(p.q.Name))
		fmt.Fprintf(&buf, "\t\tUsing: []string{%s},\n", g.capabilityList(p.q.Using))
		fmt.Fprintf(&buf, "\t\tCalls: %d,\n", len(p.q.Calls))
		if len(p.sessionCapabilities) > 0 {
			fmt.Fprintf(&buf, "\t\tPrimaryAccounts: []string{%s},\n", g.capabilityList(p.sessionCapabilities))
		}
		buf.WriteString("\t},\n")
	}
	buf.WriteString("}\n")

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gen: formatting %s: %w\n%s", VerifyFileName, err, buf.String())
	}
	return src, nil
}

// capabilityList renders capability URIs as the elements of a []string.
func (g *RequestGenerator) capabilityList(uris []string) string {
	exprs := make([]string, len(uris))
	for i, uri := range uris {
		exprs[i] = g.capabilityExpr(uri)
	}
	return strings.Join(exprs, ", ")
}

// requestsDir names the directory the requests were read from, for the Source
// line. The requests of one package come from one directory.
func requestsDir(plans []*plan) string {
	if len(plans) == 0 {
		return ""
	}
	return path.Dir(strings.ReplaceAll(plans[0].q.Path, `\`, "/"))
}
