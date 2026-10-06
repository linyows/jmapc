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
			{"name": "Note", "properties": [{"name": "author", "type": "Nonesuch"}]}
		]}`,
		want: `refers to the type "Nonesuch", which nothing defines`,
	}, {
		name: "arguments for a method the type does not have",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["get"],
			 "arguments": {"query": [{"name": "x", "type": "String"}]}}
		]}`,
		want: "adds arguments to Note/query, which it does not define",
	}, {
		name: "type named after a type JMAP already has",
		src:  `{"capability": "urn:x:y", "types": [{"name": "String", "properties": []}]}`,
		want: `"String" is the name of a type JMAP already has`,
	}, {
		name: "type name that is not a name",
		src:  `{"capability": "urn:x:y", "types": [{"name": "a b/c", "properties": []}]}`,
		want: `"a b/c" is not a type name`,
	}, {
		name: "type name that does not begin with a capital",
		src:  `{"capability": "urn:x:y", "types": [{"name": "email", "properties": []}]}`,
		want: `"email" is not a type name`,
	}, {
		name: "type that is a type JMAP already has but for case",
		src:  `{"capability": "urn:x:y", "types": [{"name": "EMAIL", "properties": []}]}`,
		want: `which is the type "Email" but for case`,
	}, {
		name: "type that is a primitive but for case",
		src:  `{"capability": "urn:x:y", "types": [{"name": "UtcDate", "properties": []}]}`,
		want: `"UtcDate" is the name of a type JMAP already has`,
	}, {
		name: "two types differing only in case",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": []}, {"name": "NOTE", "properties": []}
		]}`,
		want: `both define the type "NOTE"`,
	}, {
		name: "property defined twice",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": [
			{"name": "title", "type": "String"}, {"name": "title", "type": "String"}
		]}]}`,
		want: `Note defines "title" twice`,
	}, {
		name: "two properties a generator writes as one",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": [
			{"name": "noteId", "type": "Id"}, {"name": "NoteId", "type": "Id"}
		]}]}`,
		want: `Note defines "noteId" and "NoteId", which a generator writes as one name`,
	}, {
		name: "argument a standard method already has but for case",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["get"],
			 "arguments": {"get": [{"name": "AccountId", "type": "Id"}]}}
		]}`,
		want: `Note/get already has the argument "accountId"`,
	}, {
		name: "argument a standard method already has",
		src: `{"capability": "urn:x:y", "types": [
			{"name": "Note", "properties": [], "methods": ["get"],
			 "arguments": {"get": [{"name": "accountId", "type": "Id"}]}}
		]}`,
		want: `Note/get already has the argument "accountId"`,
	}, {
		name: "patches to a type nothing defines",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": [
			{"name": "edits", "type": "PatchObject", "patchTarget": "Nonesuch"}
		]}]}`,
		want: `Note.edits patches the type "Nonesuch", which nothing defines`,
	}, {
		name: "sorts a type nothing defines",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": [
			{"name": "order", "type": "Comparator[]", "sortTarget": "Nonesuch"}
		]}]}`,
		want: `Note.order sorts the type "Nonesuch", which nothing defines`,
	}, {
		name: "method name without a type",
		src:  `{"capability": "urn:x:y", "types": [], "methods": [{"name": "summarise"}]}`,
		want: `"summarise" is not a method name`,
	}, {
		name: "method name with two slashes",
		src:  `{"capability": "urn:x:y", "types": [], "methods": [{"name": "Note/get/extra"}]}`,
		want: `"Note/get/extra" is not a method name`,
	}, {
		name: "method name with something other than letters and digits",
		src:  `{"capability": "urn:x:y", "types": [], "methods": [{"name": "Note/sum-up"}]}`,
		want: `"Note/sum-up" is not a method name`,
	}, {
		name: "method over a type nothing defines",
		src:  `{"capability": "urn:x:y", "types": [], "methods": [{"name": "Note/summarise", "dataType": "Note"}]}`,
		want: `Note/summarise works on the type "Note", which nothing defines`,
	}, {
		name: "method selecting properties through an argument it does not have",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": []}],
			"methods": [{"name": "Note/summarise", "dataType": "Note", "properties": "fields"}]}`,
		want: `Note/summarise selects properties through "fields", which is not one of its arguments`,
	}, {
		name: "method selecting properties of no type",
		src: `{"capability": "urn:x:y", "types": [],
			"methods": [{"name": "Note/summarise", "properties": "fields",
			             "arguments": [{"name": "fields", "type": "String[]"}]}]}`,
		want: `names no dataType for them to be properties of`,
	}, {
		name: "method returning records in a property its response does not have",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": []}],
			"methods": [{"name": "Note/summarise", "dataType": "Note", "resultProperty": "list"}]}`,
		want: `Note/summarise returns its records in "list", which its response does not have`,
	}, {
		name: "method returning the id of records it does not narrow",
		src: `{"capability": "urn:x:y", "types": [{"name": "Note", "properties": []}],
			"methods": [{"name": "Note/summarise", "dataType": "Note", "returnsId": true}]}`,
		want: `Note/summarise says it returns the id of every record, and needs properties and resultProperty`,
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

// TestExtendTakesDynamicProperties checks a vendor type naming properties a
// /get may ask for beyond its fields, which the catalogue then accepts of it
// as it does a header field of an Email.
func TestExtendTakesDynamicProperties(t *testing.T) {
	s, err := extend(t, `{"capability": "urn:x:y", "types": [
		{"name": "Note", "properties": [{"name": "id", "type": "Id"}], "dynamic": ["meta:"]}
	]}`)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}
	note, _ := s.Object("Note")
	if !note.AcceptsDynamic("meta:colour") || note.AcceptsDynamic("header:Subject") {
		t.Errorf("Note accepts meta:colour %v, header:Subject %v; want true and false",
			note.AcceptsDynamic("meta:colour"), note.AcceptsDynamic("header:Subject"))
	}
	if _, err := extend(t, `{"capability": "urn:x:y", "types": [
		{"name": "Note", "properties": [{"name": "id", "type": "Id"}], "dynamic": ["id"]}
	]}`); err == nil || !strings.Contains(err.Error(), "has a property of that name") {
		t.Errorf("a dynamic property named as a field: %v, want it refused", err)
	}
}
