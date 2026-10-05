package request

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// TestRepeats checks which calls are found to repeat an earlier one: the same
// method with the same arguments, with nothing between that could change the
// answer.
func TestRepeats(t *testing.T) {
	for _, tc := range []struct {
		name  string
		calls string
		// want lists each repeat as "later=earlier".
		want []string
	}{
		{
			name: "the same call twice",
			calls: `["Mailbox/get", {"ids": null}, "a"],
			        ["Mailbox/get", {"ids": null}, "b"]`,
			want: []string{"b=a"},
		},
		{
			name: "the order of members and a comment do not count",
			calls: `["Email/query", {"filter": {"inMailbox": "{{box}}"}, "limit": 10}, "a"],
			        ["Email/query", {"_comment": "Again.", "limit": 10, "filter": {"inMailbox": "{{ box }}"}}, "b"]`,
			want: []string{"b=a"},
		},
		{
			name: "a third repeat names the first",
			calls: `["Mailbox/get", {"ids": null}, "a"],
			        ["Mailbox/get", {"ids": null}, "b"],
			        ["Mailbox/get", {"ids": null}, "c"]`,
			want: []string{"b=a", "c=a"},
		},
		{
			name: "a query and its get, written twice",
			calls: `["Email/query", {"limit": 10}, "q1"],
			        ["Email/get", {"#ids": {"resultOf": "q1", "name": "Email/query", "path": "/ids"}}, "g1"],
			        ["Email/query", {"limit": 10}, "q2"],
			        ["Email/get", {"#ids": {"resultOf": "q2", "name": "Email/query", "path": "/ids"}}, "g2"]`,
			want: []string{"q2=q1", "g2=g1"},
		},
		{
			name: "a call that only reads between them",
			calls: `["Mailbox/get", {"ids": null}, "a"],
			        ["Core/echo", {"hello": true}, "echo"],
			        ["Mailbox/get", {"ids": null}, "b"]`,
			want: []string{"b=a"},
		},
		{
			name: "a write between them",
			calls: `["Mailbox/get", {"ids": null}, "a"],
			        ["Mailbox/set", {"create": {"box": {"name": "x"}}}, "make"],
			        ["Mailbox/get", {"ids": null}, "b"]`,
		},
		{
			name: "the same write twice",
			calls: `["Mailbox/set", {"create": {"box": {"name": "x"}}}, "a"],
			        ["Mailbox/set", {"create": {"box2": {"name": "x"}}}, "b"],
			        ["Mailbox/set", {"create": {"box2": {"name": "x"}}}, "c"]`,
		},
		{
			name: "parameters of different names",
			calls: `["Email/query", {"filter": {"inMailbox": "{{one}}"}}, "a"],
			        ["Email/query", {"filter": {"inMailbox": "{{two}}"}}, "b"]`,
		},
		{
			name: "other arguments",
			calls: `["Email/query", {"limit": 10}, "a"],
			        ["Email/query", {"limit": 20}, "b"]`,
		},
		{
			name: "integers a float64 cannot tell apart",
			calls: `["Core/echo", {"payload": [9007199254740992]}, "a"],
			        ["Core/echo", {"payload": [9007199254740993]}, "b"]`,
		},
		{
			name: "what looks like a reference in a value stated outright",
			calls: `["Mailbox/get", {"ids": null}, "a"],
			        ["Mailbox/get", {"ids": null}, "b"],
			        ["Core/echo", {"payload": {"resultOf": "a"}}, "c"],
			        ["Core/echo", {"payload": {"resultOf": "b"}}, "d"]`,
			want: []string{"b=a"},
		},
		{
			name: "what looks like a parameter in a value stated outright",
			calls: `["Core/echo", {"payload": {"text": "{{id}}"}}, "a"],
			        ["Core/echo", {"payload": {"text": "{{ id }}"}}, "b"]`,
		},
		{
			name: "references to different calls",
			calls: `["Email/query", {"limit": 10}, "q1"],
			        ["Email/query", {"limit": 20}, "q2"],
			        ["Email/get", {"#ids": {"resultOf": "q1", "name": "Email/query", "path": "/ids"}}, "g1"],
			        ["Email/get", {"#ids": {"resultOf": "q2", "name": "Email/query", "path": "/ids"}}, "g2"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := parse(t, "Repeats"+Extension, `{"methodCalls": [`+tc.calls+`]}`)
			var got []string
			for _, r := range q.Repeats {
				got = append(got, r.Call.ID+"="+r.Same.ID)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("repeats are %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReadsOnly checks which methods are taken as only reading. One that is
// not known to, a vendor's own included, is taken as one that may change data.
func TestReadsOnly(t *testing.T) {
	for method, want := range map[string]bool{
		"Email/get":                   true,
		"Email/queryChanges":          true,
		"SieveScript/validate":        true,
		"Principal/getAvailability":   true,
		"Email/set":                   false,
		"Email/import":                false,
		"Blob/upload":                 false,
		"MDN/send":                    false,
		"Note/pin":                    false,
		"Note/get":                    false,
		"Note/echo":                   false,
		"get":                         false,
		"urn:example:Note/frobnicate": false,
	} {
		if got := readsOnly(method); got != want {
			t.Errorf("readsOnly(%q) = %v, want %v", method, got, want)
		}
	}
}

// TestRepeatsAcrossAVendorMethod checks that a vendor's method is taken as one
// that may change data, whatever its name ends in: its Note/echo need not be
// Core/echo, nor its Note/get a /get as RFC 8620 defines one.
func TestRepeatsAcrossAVendorMethod(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, []byte(`{
	  "capability": "urn:example:params:jmap:notes",
	  "types": [{
	    "name": "Note",
	    "doc": "Note is a scrap of text the user keeps.",
	    "properties": [{"name": "id", "type": "Id", "doc": "The id of the note."}],
	    "methods": ["get"]
	  }],
	  "methods": [
	    {"name": "Note/echo", "doc": "Echoes, and touches every note.", "dataType": "Note",
	     "arguments": [{"name": "accountId", "type": "Id", "doc": "The account to operate on."}],
	     "response": [{"name": "accountId", "type": "Id", "doc": "The account operated on."}]}
	  ]
	}`), 0o644); err != nil {
		t.Fatalf("writing the schema: %v", err)
	}
	sc, err := spec.LoadSchema(path)
	if err != nil {
		t.Fatalf("loading the schema: %v", err)
	}
	catalogue := spec.Standard()
	if err := catalogue.Extend(sc); err != nil {
		t.Fatalf("extending the catalogue: %v", err)
	}
	q, err := NewParser(catalogue).Parse("Notes"+Extension, []byte(`{"methodCalls": [
	  ["Mailbox/get", {"ids": null}, "a"],
	  ["Note/echo", {}, "touch"],
	  ["Mailbox/get", {"ids": null}, "b"],
	  ["Note/get", {"ids": null}, "c"],
	  ["Note/get", {"ids": null}, "d"]
	]}`))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	for _, r := range q.Repeats {
		t.Errorf("%s is reported to repeat %s across a vendor's method", r.Call.ID, r.Same.ID)
	}
}
