package rust

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
)

// VerifyModule is the module the verify function is generated into.
const VerifyModule = "verify"

// RuntimeModules are the modules the runtime and the data model are written to
// beside the requests, and mod.rs, which declares them all. A request whose
// module would have one of these names is refused rather than overwritten.
var RuntimeModules = []string{"client", "types", "mod"}

// verifyFile writes verify, with what each request needs of the server as far
// as the request file says: the capabilities it declares, how many calls it
// makes, and the capabilities whose primary account fills in an account it
// leaves out.
func (g *RequestGenerator) verifyFile(plans []*plan) []byte {
	paths := make([]string, len(plans))
	for i, p := range plans {
		paths[i] = p.q.Path
	}
	var buf bytes.Buffer
	writeHeader(&buf, shared.RequestsDir(paths))
	buf.WriteString("use super::client::{Client, Error, RequestNeeds, Transport};\n\n")
	writeDoc(&buf, "", "verify checks every request in this directory against the session, and "+
		"fails with Error::Verify holding everything that would stop one of them from being sent. "+
		"Call it as the program starts, to learn then rather than when a request is first sent. "+
		"See Client::verify for what it checks.")
	buf.WriteString("pub async fn verify<T: Transport>(client: &Client<T>) -> Result<(), Error> {\n")
	buf.WriteString("    client.verify(&NEEDS).await\n}\n\n")
	writeDoc(&buf, "", "What each request in this directory needs of the server.")
	fmt.Fprintf(&buf, "const NEEDS: [RequestNeeds; %d] = [\n", len(plans))
	for _, p := range plans {
		buf.WriteString("    RequestNeeds {\n")
		fmt.Fprintf(&buf, "        name: %s,\n", quote(p.q.Name))
		writeStrSlice(&buf, "using", p.q.Using)
		fmt.Fprintf(&buf, "        calls: %d,\n", len(p.q.Calls))
		writeStrSlice(&buf, "primary_accounts", p.sessionCapabilities)
		buf.WriteString("    },\n")
	}
	buf.WriteString("];\n")
	return finish(&buf)
}

// arrayWidth is the widest the elements of an array literal may be, separators
// included and brackets not, for rustfmt to keep it on one line: its
// array_width, 60% of the line width.
const arrayWidth = lineWidth * 60 / 100

// writeStrSlice writes a field holding a slice of string literals, on one line
// where rustfmt would keep it there and one element to a line where not.
func writeStrSlice(buf *bytes.Buffer, field string, values []string) {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	elements := strings.Join(quoted, ", ")
	if line := "        " + field + ": &[" + elements + "],"; len(elements) <= arrayWidth && len(line) <= lineWidth {
		buf.WriteString(line + "\n")
		return
	}
	fmt.Fprintf(buf, "        %s: &[\n", field)
	for _, q := range quoted {
		fmt.Fprintf(buf, "            %s,\n", q)
	}
	buf.WriteString("        ],\n")
}
