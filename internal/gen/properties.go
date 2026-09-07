package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
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
	var imports []string
	if bytes.Contains(body, []byte("json.")) {
		imports = append(imports, "encoding/json")
	}
	runtime := bytes.Contains(body, []byte(g.Qualifier))
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
	shared.WriteComment(buf, "", propertySetDoc(set))
	fmt.Fprintf(buf, "type %s struct {\n", set.Name)
	if set.Extends != nil {
		shared.WriteComment(buf, "\t", fmt.Sprintf(
			"The properties of %s, which this set adds to.", set.Extends.Name))
		fmt.Fprintf(buf, "\t%s\n", set.Extends.Name)
	}
	for i, name := range setProperties(set, dataType) {
		if i > 0 || set.Extends != nil {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, dataType, name, "", "")
	}
	buf.WriteString("}\n\n")
}

// setProperties returns the properties the type for a set declares itself: its
// own, and the id that a /get answers with whether or not it was asked for.
//
// The id belongs to the set that declares no base, since that is the type every
// other one embeds, and only to a set narrowing a type that has one: a set
// narrowing the body parts of an Email describes something a record holds
// rather than a record.
func setProperties(set *request.PropertySet, dataType *spec.Object) []string {
	if set.Extends != nil {
		return set.Own
	}
	if _, hasID := dataType.Field("id"); !hasID {
		return set.Own
	}
	return shared.RecordProperties(set.Own)
}

// propertySetDoc is the comment the type for a set carries: what the author
// said it is for, and what asking for it gets.
func propertySetDoc(set *request.PropertySet) string {
	asked := fmt.Sprintf("%s holds the properties of %s that a call asking for @%s receives.",
		set.Name, set.Type, set.Name)
	if set.Doc == "" {
		return asked
	}
	return set.Doc + "\n\n" + asked
}
