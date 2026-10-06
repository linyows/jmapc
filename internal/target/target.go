// Package target generates the whole of a client in one of the languages jmapc
// writes: the file of each request, and, where the language has no runtime
// package to import, the data model and the runtime beside them.
package target

import (
	"fmt"

	"github.com/linyows/jmapc/internal/gen"
	"github.com/linyows/jmapc/internal/gen/rust"
	"github.com/linyows/jmapc/internal/gen/ts"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// The languages a client is generated in.
const (
	Go         = "go"
	TypeScript = "typescript"
	Rust       = "rust"
)

// Languages lists them, in the order a message names them.
var Languages = []string{Go, TypeScript, Rust}

// handWritten are the types the Rust and TypeScript runtimes write by hand
// rather than take from the data model. A PatchObject is a shape rather than a
// record.
var handWritten = map[string]bool{"PatchObject": true}

// Client describes the client to generate.
type Client struct {
	// Lang is the language, one of Go, TypeScript and Rust.
	Lang string
	// Package names the generated Go package, and means nothing elsewhere.
	Package string
	// Spec is the catalogue the requests were checked against.
	Spec *spec.Spec
	// Requests are the requests to generate a function for each of.
	Requests []*request.Request
	// Properties are the sets of properties the requests may ask for.
	Properties *request.PropertySets
}

// Generate returns the files of the client, keyed by their names within the
// output directory.
func (c *Client) Generate() (map[string][]byte, error) {
	switch c.Lang {
	case Go:
		return (&gen.RequestGenerator{
			Spec:       c.Spec,
			Package:    c.Package,
			Qualifier:  "jmapc.",
			Requests:   c.Requests,
			Properties: c.Properties,
		}).Generate()
	case Rust:
		files, err := (&rust.RequestGenerator{Spec: c.Spec, Requests: c.Requests, Properties: c.Properties}).Generate()
		if err != nil {
			return nil, err
		}
		return withRuntime(files, "rs",
			(&rust.TypeGenerator{Spec: c.Spec, Skip: handWritten}).Generate,
			(&rust.ClientGenerator{}).Generate)
	case TypeScript:
		files, err := (&ts.RequestGenerator{Spec: c.Spec, Requests: c.Requests, Properties: c.Properties}).Generate()
		if err != nil {
			return nil, err
		}
		return withRuntime(files, "ts",
			(&ts.TypeGenerator{Spec: c.Spec, Skip: handWritten}).Generate,
			(&ts.ClientGenerator{}).Generate)
	}
	return nil, fmt.Errorf("cannot generate %q; the languages are %s, %s and %s", c.Lang, Go, TypeScript, Rust)
}

// withRuntime adds the data model and the runtime to the files of the
// requests, as types and client with the language's extension.
func withRuntime(files map[string][]byte, ext string, types, client func() ([]byte, error)) (map[string][]byte, error) {
	t, err := types()
	if err != nil {
		return nil, err
	}
	r, err := client()
	if err != nil {
		return nil, err
	}
	files["types."+ext] = t
	files["client."+ext] = r
	return files, nil
}
