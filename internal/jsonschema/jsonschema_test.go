package jsonschema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// generate returns the schema for the standard catalogue, decoded.
func generate(t *testing.T) map[string]any {
	t.Helper()
	out, err := (&Generator{Spec: spec.Standard()}).Generate()
	if err != nil {
		t.Fatalf("generating the schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	return doc
}

// definitions returns the schema's definitions.
func definitions(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	defs, ok := doc["definitions"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no definitions")
	}
	return defs
}

// TestEveryMethodIsDescribed checks that the schema covers the catalogue: a
// method it does not name is one an editor would report as a mistake.
func TestEveryMethodIsDescribed(t *testing.T) {
	catalogue := spec.Standard()
	doc := generate(t)
	defs := definitions(t, doc)

	names := map[string]bool{}
	enum, ok := defs[methodDef].(map[string]any)["enum"].([]any)
	if !ok {
		t.Fatal("the method names are not an enum")
	}
	for _, name := range enum {
		names[name.(string)] = true
	}
	for _, m := range catalogue.Methods() {
		if !names[m.Name] {
			t.Errorf("the schema does not know the method %s", m.Name)
		}
		if _, described := defs[m.Arguments]; !described {
			t.Errorf("the schema does not describe %s, the arguments of %s", m.Arguments, m.Name)
		}
	}
	if len(names) != len(catalogue.Methods()) {
		t.Errorf("the schema names %d methods, the catalogue has %d", len(names), len(catalogue.Methods()))
	}
}

// TestEveryReferenceResolves checks that no definition points at one that was
// never written, which a validator would refuse to load the schema over.
func TestEveryReferenceResolves(t *testing.T) {
	out, err := (&Generator{Spec: spec.Standard()}).Generate()
	if err != nil {
		t.Fatalf("generating the schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	defs := definitions(t, doc)

	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if target, ok := v["$ref"].(string); ok {
				name := strings.TrimPrefix(target, "#/definitions/")
				if name == target {
					t.Errorf("the reference %q does not point into the definitions", target)
				} else if _, defined := defs[name]; !defined {
					t.Errorf("the reference %q points at nothing", target)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
}

// TestFilterOperatorsNest checks the one thing the data model cannot say. The
// conditions of an AND are Any there, because what they may hold depends on the
// type being queried; the schema names itself instead, so that a condition
// inside an operator is checked like one outside it.
func TestFilterOperatorsNest(t *testing.T) {
	defs := definitions(t, generate(t))
	filter, ok := defs["EmailFilter"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no EmailFilter")
	}
	if !strings.Contains(render(t, filter), `"#/definitions/EmailFilterCondition"`) {
		t.Errorf("an EmailFilter does not admit an EmailFilterCondition:\n%s", render(t, filter))
	}
	operator, ok := defs["EmailFilterOperator"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no EmailFilterOperator")
	}
	conditions := operator["properties"].(map[string]any)["conditions"]
	if !strings.Contains(render(t, conditions), `"#/definitions/EmailFilter"`) {
		t.Errorf("the conditions of an operator are not filters:\n%s", render(t, conditions))
	}
}

// TestComparatorsKnowWhatTheySort checks that the sort order of a /query is
// described against the type being queried, so that an editor offers the
// properties that type can actually be sorted by.
func TestComparatorsKnowWhatTheySort(t *testing.T) {
	defs := definitions(t, generate(t))
	comparator, ok := defs["EmailComparator"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no EmailComparator")
	}
	props := comparator["properties"].(map[string]any)
	rendered := render(t, props["property"])
	for _, want := range []string{`"receivedAt"`, `"hasKeyword"`} {
		if !strings.Contains(rendered, want) {
			t.Errorf("an Email cannot be sorted by %s:\n%s", want, rendered)
		}
	}
	// A comparator like hasKeyword takes a member of its own, which the
	// generic Comparator has no room for.
	if _, ok := props["keyword"]; !ok {
		t.Errorf("the extra member hasKeyword takes is missing: %v", props)
	}
}

// TestPropertyNamesAreOffered checks the argument that selects properties of a
// record, which is the one place a plain array of strings can be completed from
// the data model.
func TestPropertyNamesAreOffered(t *testing.T) {
	defs := definitions(t, generate(t))
	args, ok := defs["EmailGetArguments"].(map[string]any)
	if !ok {
		t.Fatal("the schema has no EmailGetArguments")
	}
	props := args["properties"].(map[string]any)
	rendered := render(t, props["properties"])
	if !strings.Contains(rendered, `"subject"`) {
		t.Errorf("the properties of an Email are not offered:\n%s", rendered)
	}
	// A property naming a header field belongs to no type, so the names the
	// data model fixes cannot be the whole of what is allowed.
	if !strings.Contains(rendered, "header:") {
		t.Errorf("a header property would be reported as a mistake:\n%s", rendered)
	}
}

// TestVendorExtensionsAreDescribed checks that a server's own types are
// described like any other, since a query against them is checked like any
// other.
func TestVendorExtensionsAreDescribed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.json")
	if err := os.WriteFile(path, []byte(`{
	  "capability": "urn:example:params:jmap:notes",
	  "types": [
	    {
	      "name": "Note",
	      "doc": "Note is a scrap of text the user keeps.",
	      "properties": [
	        {"name": "id", "type": "Id", "serverSet": true, "doc": "The id of the note."},
	        {"name": "title", "type": "String", "doc": "The note's title."}
	      ],
	      "methods": ["get", "set"]
	    }
	  ]
	}`), 0o644); err != nil {
		t.Fatalf("writing the schema file: %v", err)
	}
	extension, err := spec.LoadSchema(path)
	if err != nil {
		t.Fatalf("loading the schema file: %v", err)
	}
	catalogue := spec.Standard()
	if err := catalogue.Extend(extension); err != nil {
		t.Fatalf("extending the catalogue: %v", err)
	}
	out, err := (&Generator{Spec: catalogue}).Generate()
	if err != nil {
		t.Fatalf("generating the schema: %v", err)
	}
	for _, want := range []string{`"Note/get"`, "NoteGetArguments", "The note's title."} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("the schema does not describe %s", want)
		}
	}
}

// TestGenerationIsDeterministic checks that generating twice gives the same
// bytes, so that a schema kept in a repository never shows up as a spurious
// diff.
func TestGenerationIsDeterministic(t *testing.T) {
	first, err := (&Generator{Spec: spec.Standard()}).Generate()
	if err != nil {
		t.Fatalf("generating the schema: %v", err)
	}
	second, err := (&Generator{Spec: spec.Standard()}).Generate()
	if err != nil {
		t.Fatalf("generating the schema: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two runs produced different schemas")
	}
}

// render returns a value as JSON, for a test that wants to say what it does not
// contain.
func render(t *testing.T, v any) string {
	t.Helper()
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	return string(out)
}
