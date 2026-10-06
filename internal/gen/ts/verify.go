package ts

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
)

// VerifyFileName is the module the verify function is generated into.
const VerifyFileName = "verify.ts"

// RuntimeFileNames are the modules the runtime and the data model are written
// to beside the requests. A request whose module would have one of these names
// is refused rather than overwritten.
var RuntimeFileNames = []string{"client.ts", "types.ts"}

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
	buf.WriteString("import type { Client } from \"./client.js\"\n\n")
	buf.WriteString("// verify checks every request in this directory against the session, and throws\n")
	buf.WriteString("// VerifyErrors with everything that would stop one of them from being sent.\n")
	buf.WriteString("// Call it as the program starts, to learn then rather than when a request is\n")
	buf.WriteString("// first sent. See Client.verify for what it checks.\n")
	buf.WriteString("export async function verify(client: Client): Promise<void> {\n")
	buf.WriteString("  await client.verify([\n")
	for _, p := range plans {
		buf.WriteString("    {\n")
		fmt.Fprintf(&buf, "      name: %s,\n", quote(p.q.Name))
		fmt.Fprintf(&buf, "      using: [%s],\n", quoteAll(p.q.Using))
		fmt.Fprintf(&buf, "      calls: %d,\n", len(p.q.Calls))
		fmt.Fprintf(&buf, "      primaryAccounts: [%s],\n", quoteAll(p.sessionCapabilities))
		buf.WriteString("    },\n")
	}
	buf.WriteString("  ])\n}\n")
	return buf.Bytes()
}

// quoteAll renders strings as the elements of an array literal.
func quoteAll(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return strings.Join(quoted, ", ")
}
