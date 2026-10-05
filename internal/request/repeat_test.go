package request

import (
	"strings"
	"testing"
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
		"get":                         false,
		"urn:example:Note/frobnicate": false,
	} {
		if got := readsOnly(method); got != want {
			t.Errorf("readsOnly(%q) = %v, want %v", method, got, want)
		}
	}
}
