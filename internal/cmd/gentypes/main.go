// Command gentypes writes the Go declarations for the JMAP data types into the
// jmapc runtime package. Both the runtime types and the code generated for a
// query come from the same catalogue, so they cannot drift apart.
package main

import (
	"fmt"
	"os"

	"github.com/linyows/jmapc/internal/gen"
	"github.com/linyows/jmapc/internal/spec"
)

// output is the file the declarations are written to, relative to the package
// the generator is run from.
const output = "types_gen.go"

func main() {
	g := &gen.TypeGenerator{
		Spec:    spec.Standard(),
		Package: "jmapc",
		// These are written by hand: PatchObject and SetError carry behaviour
		// beyond their shape, and Account is part of the session object, which
		// is not generated.
		Skip: map[string]bool{"PatchObject": true, "SetError": true, "Account": true},
	}
	src, err := g.Generate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gentypes: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(output, src, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gentypes: %v\n", err)
		os.Exit(1)
	}
}
