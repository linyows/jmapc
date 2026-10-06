package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
)

// PropertiesFileName is the file the types for the named sets of properties are
// generated into. They are not a request's to declare: six requests may ask for
// one set, and the type they answer with is the same type.
const PropertiesFileName = "properties_gen.go"

// propertiesFile writes the types for the named sets of properties.
func (g *RequestGenerator) propertiesFile() ([]byte, error) {
	var body bytes.Buffer
	for _, set := range g.Properties.Sets {
		g.writePropertySet(&body, set)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s\n", shared.GeneratedBanner)
	fmt.Fprintf(&buf, "// Source: %s\n\n", strings.ReplaceAll(g.Properties.Path, `\`, "/"))
	fmt.Fprintf(&buf, "package %s\n\n", g.Package)
	g.writePropertyImports(&buf, body.Bytes())
	buf.Write(body.Bytes())

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gen: formatting %s: %w\n%s", g.Properties.Path, err, buf.String())
	}
	return src, nil
}

// writePropertyImports writes the import block of the file the sets are
// generated into, which needs less than a request does: there is no function
// here, only the types the records take.
func (g *RequestGenerator) writePropertyImports(buf *bytes.Buffer, body []byte) {
	used := packagesUsed(body)
	var imports []string
	if used["json"] {
		imports = append(imports, "encoding/json")
	}
	runtime := used[strings.TrimSuffix(g.Qualifier, ".")]
	if len(imports) == 0 && !runtime {
		return
	}
	buf.WriteString("import (\n")
	for _, path := range imports {
		fmt.Fprintf(buf, "\t%q\n", path)
	}
	if runtime {
		if len(imports) > 0 {
			buf.WriteString("\n")
		}
		fmt.Fprintf(buf, "\t%q\n", "github.com/linyows/jmapc")
	}
	buf.WriteString(")\n\n")
}

// writePropertySet writes the type for one named set. A set that extends
// another embeds it, so that a function written for the base takes a record of
// the derived set without anything being copied from one struct to another.
func (g *RequestGenerator) writePropertySet(buf *bytes.Buffer, set *request.PropertySet) {
	dataType, ok := g.Spec.Object(set.Type)
	if !ok {
		return
	}
	shared.WriteComment(buf, "", shared.SetDoc(set, set.Name))
	fmt.Fprintf(buf, "type %s struct {\n", set.Name)
	if set.Extends != nil {
		shared.WriteComment(buf, "\t", fmt.Sprintf(
			"The properties of %s, which this set adds to.", set.Extends.Name))
		fmt.Fprintf(buf, "\t%s\n", set.Extends.Name)
	}
	for i, name := range shared.SetProperties(set, dataType) {
		if i > 0 || set.Extends != nil {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, dataType, name, "", "")
	}
	buf.WriteString("}\n\n")
}
