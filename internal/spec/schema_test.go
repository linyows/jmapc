package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// notesSchema is a vendor extension of the kind a real server offers: a type of
// its own, the standard methods over it, and one method that is not standard.
const notesSchema = `{
  "capability": "urn:example:params:jmap:notes",
  "types": [
    {
      "name": "Note",
      "doc": "Note is a scrap of text the user keeps.",
      "properties": [
        {"name": "id", "type": "Id", "serverSet": true, "immutable": true, "doc": "The id of the note."},
        {"name": "title", "type": "String", "doc": "The note's title."},
        {"name": "body", "type": "String", "doc": "The note's text."},
        {"name": "createdAt", "type": "UTCDate", "serverSet": true, "doc": "When the note was created."},
        {"name": "mailboxIds", "type": "Id[Boolean]", "doc": "The mailboxes the note is filed in."}
      ],
      "methods": ["get", "changes", "set", "query"],
      "sort": [
        {"name": "createdAt", "doc": "Sorts by when the note was created."},
        {"name": "title", "doc": "Sorts by title."}
      ]
    },
    {
      "name": "NoteFilterCondition",
      "doc": "NoteFilterCondition is a condition a note must satisfy to match a Note/query.",
      "properties": [
        {"name": "text", "type": "String", "doc": "Matches notes containing this text."},
        {"name": "before", "type": "UTCDate", "doc": "Matches notes created before this time."}
      ]
    }
  ],
  "methods": [
    {
      "name": "Note/pin",
      "doc": "Pins notes to the top of the list.",
      "dataType": "Note",
      "arguments": [
        {"name": "accountId", "type": "Id", "doc": "The account to operate on."},
        {"name": "ids", "type": "Id[]", "doc": "The notes to pin."}
      ],
      "response": [
        {"name": "accountId", "type": "Id", "doc": "The account operated on."},
        {"name": "pinned", "type": "Id[]", "doc": "The notes that were pinned."}
      ]
    }
  ]
}`

// extend parses a schema and adds it to a fresh standard catalogue.
func extend(t *testing.T, src string) (*Spec, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("writing the schema: %v", err)
	}
	sc, err := LoadSchema(path)
	if err != nil {
		return nil, err
	}
	s := Standard()
	return s, s.Extend(sc)
}

func TestExtend(t *testing.T) {
	s, err := extend(t, notesSchema)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}

	// The type and its standard methods are registered under the schema's
	// capability.
	for _, name := range []string{"Note/get", "Note/changes", "Note/set", "Note/query", "Note/pin"} {
		m, ok := s.Method(name)
		if !ok {
			t.Errorf("method %q was not registered", name)
			continue
		}
		if m.Capability != "urn:example:params:jmap:notes" {
			t.Errorf("%s needs %q, want the schema's capability", name, m.Capability)
		}
	}
	if _, ok := s.Method("Note/copy"); ok {
		t.Error("Note/copy was registered, but the schema did not ask for it")
	}

	// The standard methods have the shape the specification gives them, so a
	// back reference into one resolves just as it would for Email.
	got, err := s.ResolvePath("Note/query", "/ids")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if got.String() != "Id[]" {
		t.Errorf("Note/query /ids = %s, want Id[]", got)
	}
	got, err = s.ResolvePath("Note/get", "/list/*/title")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if got.String() != "String[]" {
		t.Errorf("Note/get /list/*/title = %s, want String[]", got)
	}

	// The sortable properties came across.
	note, _ := s.Object("Note")
	if _, ok := note.SortProperty("createdAt"); !ok {
		t.Error("Note is not sortable by createdAt")
	}
	if _, ok := note.SortProperty("body"); ok {
		t.Error("Note is sortable by body, which the schema did not list")
	}

	// A /set over the new type knows what its patches apply to.
	args, err := s.ArgumentsOf("Note/set")
	if err != nil {
		t.Fatalf("Note/set: %v", err)
	}
	update, ok := args.Field("update")
	if !ok {
		t.Fatal("Note/set has no update argument")
	}
	if update.PatchTarget != "Note" {
		t.Errorf("update patches %q, want Note", update.PatchTarget)
	}
}

func TestExtendErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "no capability",
		src:  `{"types": []}`,
		want: "does not say which capability",
	}, {
		name: "capability is not a URI",
		src:  `{"capability": "notes", "types": []}`,
		want: `"notes" is not a capability URI`,
	}, {
		name: "type already exists",
		src:  `{"capability": "urn:x:y", "types": [{"name": "Email", "properties": []}]}`,
		want: `the type "Email", which already exists`,
	}, {
		name: "method already exists",
		src:  `{"capability": "urn:x:y", "types": [], "methods": [{"name": "Email/get"}]}`,
		want: `the method "Email/get" already exists`,
	}, {
		name: "two types with one name",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": []}, {"name": "Note", "properties": []}
		]}`,
		want: `both define the type "Note"`,
	}, {
		name: "unknown standard method",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["fetch"]}
		]}`,
		want: `"fetch" is not a standard method`,
	}, {
		name: "query without a filter condition",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["query"]}
		]}`,
		want: "must also define NoteFilterCondition",
	}, {
		name: "property with no type",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [{"name": "title"}]}
		]}`,
		want: "Note.title has no type",
	}, {
		name: "malformed type expression",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [{"name": "title", "type": "String["}]}
		]}`,
		want: "Note.title",
	}, {
		name: "reference to a type nothing defines",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [{"name": "author", "type": "Person"}]}
		]}`,
		want: `refers to the type "Person", which nothing defines`,
	}, {
		name: "arguments for a method the type does not have",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["get"],
			 "arguments": {"query": [{"name": "x", "type": "String"}]}}
		]}`,
		want: "adds arguments to Note/query, which it does not define",
	}, {
		name: "unknown member in the schema",
		src:  `{"capability": "urn:x:y", "typs": []}`,
		want: `unknown field "typs"`,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extend(t, tt.src)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error was:\n%v\nwant it to mention: %s", err, tt.want)
			}
		})
	}
}

// TestExtendLeavesStandardAlone checks that extending one catalogue does not
// reach into another, since each call to Standard builds its own.
func TestExtendLeavesStandardAlone(t *testing.T) {
	if _, err := extend(t, notesSchema); err != nil {
		t.Fatalf("Extend: %v", err)
	}
	if _, ok := Standard().Method("Note/get"); ok {
		t.Error("a schema loaded into one catalogue turned up in another")
	}
}

// TestExtendAddsArguments checks the arguments a schema adds to a standard
// method it asked for.
func TestExtendAddsArguments(t *testing.T) {
	s, err := extend(t, `{
	  "capability": "urn:example:params:jmap:notes",
	  "types": [{
	    "name": "Note",
	    "properties": [{"name": "id", "type": "Id", "doc": "The id."}],
	    "methods": ["get"],
	    "arguments": {"get": [{"name": "includeArchived", "type": "Boolean", "default": "false",
	                           "doc": "Whether to include archived notes."}]}
	  }]
	}`)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}
	args, err := s.ArgumentsOf("Note/get")
	if err != nil {
		t.Fatalf("Note/get: %v", err)
	}
	if _, ok := args.Field("includeArchived"); !ok {
		t.Errorf("Note/get has no includeArchived argument (has %v)", args.PropertyNames())
	}
}
