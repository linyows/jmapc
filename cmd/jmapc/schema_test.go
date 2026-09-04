package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSchema checks the command that describes the query files: it writes a
// schema an editor can load, covering the vendor extensions the configuration
// names as well as the specifications jmapc knows.
func TestSchema(t *testing.T) {
	out, _, err := capture(t, []string{"schema"})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	if doc["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Errorf("the schema does not say which dialect it is written in: %v", doc["$schema"])
	}
	if !strings.Contains(out, `"Email/query"`) {
		t.Error("the schema does not know Email/query")
	}
}

// TestSchemaToFile checks the schema written to disk, which is where an editor
// reads it from.
func TestSchemaToFile(t *testing.T) {
	dir := workspace(t, map[string]string{
		"schema/notes.json": `{
		  "capability": "urn:example:params:jmap:notes",
		  "types": [{
		    "name": "Note",
		    "doc": "Note is a scrap of text the user keeps.",
		    "properties": [{"name": "id", "type": "Id", "doc": "The id of the note."}],
		    "methods": ["get"]
		  }]
		}`,
	})
	path := filepath.Join(dir, "jmapc.schema.json")

	if _, _, err := capture(t, []string{"schema", "-schema", filepath.Join(dir, "schema/notes.json"), "-out", path}); err != nil {
		t.Fatalf("schema: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	if !strings.Contains(string(written), `"Note/get"`) {
		t.Error("the schema does not describe the vendor extension")
	}
}
