package ts

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// PropertiesModule is the module the types for the named sets of properties are
// written into, without its extension. They are not a request's to declare: six
// requests may ask for one set, and the type they answer with is the same type.
const PropertiesModule = "properties"

// PropertiesFileName is the file that module lives in.
const PropertiesFileName = PropertiesModule + ".ts"

// propertiesFile writes the module holding the types for the named sets.
func (g *RequestGenerator) propertiesFile() []byte {
	var body bytes.Buffer
	for _, set := range g.Properties.Sets {
		g.writePropertySet(&body, set)
	}

	var buf bytes.Buffer
	writeHeader(&buf, g.Properties.Path)
	g.writePropertyImports(&buf, body.String())
	buf.Write(body.Bytes())
	return buf.Bytes()
}

// writePropertyImports names what the module takes from the data model, which
// is the type of every property the sets hold.
func (g *RequestGenerator) writePropertyImports(buf *bytes.Buffer, body string) {
	names := map[string]bool{}
	for _, set := range g.Properties.Sets {
		dataType, ok := g.Spec.Object(set.Type)
		if !ok {
			continue
		}
		for _, name := range shared.SetProperties(set, dataType) {
			field, known := dataType.Field(name)
			if !known {
				if header, err := spec.ParseHeaderProperty(name); err == nil && header != nil {
					collectTypeNames(spec.MustParseType(header.Type), names)
				}
				continue
			}
			collectTypeNames(field.ParsedType(), names)
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	if len(sorted) == 0 {
		return
	}
	fmt.Fprintf(buf, "import type { %s } from \"./types.js\"\n\n", strings.Join(sorted, ", "))
}

// writePropertySet writes the type for one named set. A set that extends
// another extends the interface, so that what is written for the base takes a
// record of the derived set as it stands.
func (g *RequestGenerator) writePropertySet(buf *bytes.Buffer, set *request.PropertySet) {
	dataType, ok := g.Spec.Object(set.Type)
	if !ok {
		return
	}
	shared.WriteComment(buf, "", shared.SetDoc(set, set.Name))
	if set.Extends != nil {
		fmt.Fprintf(buf, "export interface %s extends %s {\n", set.Name, set.Extends.Name)
	} else {
		fmt.Fprintf(buf, "export interface %s {\n", set.Name)
	}
	for i, name := range shared.SetProperties(set, dataType) {
		if i > 0 {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, dataType, set.Own, name, "", "")
	}
	buf.WriteString("}\n\n")
}
