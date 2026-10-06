package rust

import (
	"bytes"
	"fmt"

	"github.com/linyows/jmapc/internal/gen/shared"
	"github.com/linyows/jmapc/internal/request"
	"github.com/linyows/jmapc/internal/spec"
)

// PropertiesModule is the module the types for the named sets of properties are
// written into. They are not a request's to declare: six requests may ask for
// one set, and the type they answer with is the same type.
const PropertiesModule = "properties"

// PropertiesFileName is the file that module lives in.
const PropertiesFileName = PropertiesModule + ".rs"

// propertiesFile writes the module holding the types for the named sets.
func (g *RequestGenerator) propertiesFile() []byte {
	var body bytes.Buffer
	for _, set := range g.Properties.Sets {
		g.writePropertySet(&body, set)
	}

	var buf bytes.Buffer
	writeHeader(&buf, g.Properties.Path)
	g.writePropertyUses(&buf, body.String())
	buf.Write(body.Bytes())
	return finish(&buf)
}

// writePropertyUses names what the module takes from the data model, which is
// the type of every property the sets hold.
func (g *RequestGenerator) writePropertyUses(buf *bytes.Buffer, body string) {
	imports := &imports{types: map[string]bool{}}
	for _, set := range g.Properties.Sets {
		dataType, ok := g.Spec.Object(set.Type)
		if !ok {
			continue
		}
		for _, name := range shared.SetProperties(set, dataType) {
			field, known := dataType.Field(name)
			if !known {
				if header, err := spec.ParseHeaderProperty(name); err == nil && header != nil {
					imports.collect(spec.MustParseType(header.Type))
				}
				continue
			}
			imports.collect(field.ParsedType())
		}
	}

	code := codeOf(body)
	if names(code, "BTreeMap") {
		buf.WriteString("use std::collections::BTreeMap;\n\n")
	}
	buf.WriteString("use serde::{Deserialize, Serialize};\n\n")

	used := make([]string, 0, len(imports.types))
	for name := range imports.types {
		if names(code, name) {
			used = append(used, name)
		}
	}
	if len(used) > 0 {
		sortUse(used)
		writeUse(buf, "super::types", used)
	}
	buf.WriteString("\n")
}

// writePropertySet writes the struct for one named set. A set that extends
// another holds it as a flattened field, so that the properties of the base
// still read and write as members of the record itself.
func (g *RequestGenerator) writePropertySet(buf *bytes.Buffer, set *request.PropertySet) {
	dataType, ok := g.Spec.Object(set.Type)
	if !ok {
		return
	}
	writeDoc(buf, "", shared.SetDoc(set, spec.RustTypeName(set.Name)))
	writeDerive(buf)
	buf.WriteString("#[serde(rename_all = \"camelCase\")]\n")
	fmt.Fprintf(buf, "pub struct %s {\n", spec.RustTypeName(set.Name))
	if set.Extends != nil {
		writeDoc(buf, "    ", fmt.Sprintf("The properties of %s, which this set adds to.", set.Extends.Name))
		buf.WriteString("    #[serde(flatten)]\n")
		fmt.Fprintf(buf, "    pub %s: %s,\n", spec.RustFieldName(set.Extends.Name), spec.RustTypeName(set.Extends.Name))
	}
	for i, name := range shared.SetProperties(set, dataType) {
		if i > 0 || set.Extends != nil {
			buf.WriteString("\n")
		}
		g.writeRecordField(buf, dataType, set.Own, name, "", "")
	}
	buf.WriteString("}\n\n")
}
