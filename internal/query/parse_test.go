package query

import (
	"strings"
	"testing"

	"github.com/linyows/jmapc/internal/spec"
)

// parse checks src as the query named by path and fails the test if it does not
// parse cleanly.
func parse(t *testing.T, path, src string) *Query {
	t.Helper()
	q, err := NewParser(spec.Standard()).Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("parsing %s:\n%v", path, err)
	}
	return q
}

// parseErr checks src and returns the error, failing the test if there is none.
func parseErr(t *testing.T, src string) string {
	t.Helper()
	_, err := NewParser(spec.Standard()).Parse("Q"+Extension, []byte(src))
	if err == nil {
		t.Fatal("expected an error, got none")
	}
	return err.Error()
}

const listInboxEmails = `{
  "doc": "ListInboxEmails returns the most recent emails in one mailbox.",
  "methodCalls": [
    // Find the ids of the matching emails, newest first.
    ["Email/query", {
      "accountId": "{{accountId}}",
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "c0"],
    /* Then fetch them, without a second round trip. */
    ["Email/get", {
      "accountId": "{{accountId}}",
      "#ids": {"resultOf": "c0", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "c1"]
  ],
  "returns": "c1"
}`

func TestParse(t *testing.T) {
	q := parse(t, "ListInboxEmails"+Extension, listInboxEmails)

	if q.Name != "ListInboxEmails" {
		t.Errorf("Name = %q, want %q", q.Name, "ListInboxEmails")
	}
	if len(q.Calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(q.Calls))
	}
	if q.Returns != q.Calls[1] {
		t.Errorf("Returns = %v, want the Email/get call", q.Returns)
	}
	if got, want := strings.Join(q.Using, " "), spec.CapabilityCore+" "+spec.CapabilityMail; got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}

	wantParams := []struct{ name, goName, goType string }{
		{"accountId", "AccountID", "jmapc.ID"},
		{"mailboxId", "MailboxID", "jmapc.ID"},
		{"limit", "Limit", "jmapc.UnsignedInt"},
	}
	if len(q.Params) != len(wantParams) {
		t.Fatalf("got %d parameters, want %d", len(q.Params), len(wantParams))
	}
	for i, want := range wantParams {
		p := q.Params[i]
		if p.Name != want.name || p.GoName != want.goName || p.GoType("jmapc.") != want.goType {
			t.Errorf("parameter %d = %s %s %s, want %s %s %s",
				i, p.Name, p.GoName, p.GoType("jmapc."), want.name, want.goName, want.goType)
		}
	}

	get := q.Calls[1]
	if got, want := strings.Join(get.Properties, ","), "id,subject,from,receivedAt"; got != want {
		t.Errorf("properties = %q, want %q", got, want)
	}
	if get.GoField != "EmailGet" {
		t.Errorf("GoField = %q, want %q", get.GoField, "EmailGet")
	}
	ref, ok := get.Args.Find("#ids")
	if !ok {
		t.Fatal("the Email/get call has no #ids back reference")
	}
	if _, ok := ref.(*ResultRef); !ok {
		t.Errorf("#ids is %T, want a *ResultRef", ref)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "unknown method",
		src:  `{"methodCalls": [["Email/quiry", {}, "c0"]]}`,
		want: `unknown method "Email/quiry"`,
	}, {
		name: "unknown argument",
		src:  `{"methodCalls": [["Email/query", {"limmit": 10}, "c0"]]}`,
		want: `Email/query has no argument "limmit"`,
	}, {
		name: "argument type",
		src:  `{"methodCalls": [["Email/query", {"limit": "ten"}, "c0"]]}`,
		want: `expected UnsignedInt|null, found a string`,
	}, {
		name: "negative unsigned",
		src:  `{"methodCalls": [["Email/query", {"limit": -1}, "c0"]]}`,
		want: `found the negative number -1`,
	}, {
		name: "unknown filter condition",
		src:  `{"methodCalls": [["Email/query", {"filter": {"inMailboxx": "abc"}}, "c0"]]}`,
		want: `has no property "inMailboxx"`,
	}, {
		name: "filter condition type",
		src:  `{"methodCalls": [["Email/query", {"filter": {"hasAttachment": "yes"}}, "c0"]]}`,
		want: `expected Boolean, found a string`,
	}, {
		name: "nested filter operator",
		src: `{"methodCalls": [["Email/query", {"filter": {
			"operator": "AND",
			"conditions": [{"inMailbox": "abc"}, {"hasAttachmnt": true}]
		}}, "c0"]]}`,
		want: `has no property "hasAttachmnt"`,
	}, {
		name: "unknown property",
		src:  `{"methodCalls": [["Email/get", {"ids": ["a"], "properties": ["id", "subjekt"]}, "c0"]]}`,
		want: `Email has no property "subjekt"`,
	}, {
		name: "back reference to a later call",
		src: `{"methodCalls": [
			["Email/get", {"#ids": {"resultOf": "c1", "name": "Email/query", "path": "/ids"}}, "c0"],
			["Email/query", {}, "c1"]
		]}`,
		want: `no earlier method call has the id "c1"`,
	}, {
		name: "back reference names the wrong method",
		src: `{"methodCalls": [
			["Email/query", {}, "c0"],
			["Email/get", {"#ids": {"resultOf": "c0", "name": "Email/get", "path": "/ids"}}, "c1"]
		]}`,
		want: `the referenced call is Email/query, but the reference names Email/get`,
	}, {
		name: "back reference path does not exist",
		src: `{"methodCalls": [
			["Email/query", {}, "c0"],
			["Email/get", {"#ids": {"resultOf": "c0", "name": "Email/query", "path": "/idz"}}, "c1"]
		]}`,
		want: `has no property "idz"`,
	}, {
		name: "back reference has the wrong type",
		src: `{"methodCalls": [
			["Email/query", {}, "c0"],
			["Email/query", {"#filter": {"resultOf": "c0", "name": "Email/query", "path": "/ids"}}, "c1"]
		]}`,
		want: `expects FilterOperator|EmailFilterCondition|null`,
	}, {
		name: "back reference selects a scalar for a list",
		src: `{"methodCalls": [
			["Email/query", {}, "c0"],
			["Email/get", {"#ids": {"resultOf": "c0", "name": "Email/query", "path": "/queryState"}}, "c1"]
		]}`,
		want: `expects Id[]|null`,
	}, {
		name: "duplicate call id",
		src:  `{"methodCalls": [["Email/query", {}, "c0"], ["Email/get", {}, "c0"]]}`,
		want: `call id "c0" is already used`,
	}, {
		name: "argument given twice",
		src: `{"methodCalls": [
			["Email/query", {}, "c0"],
			["Email/get", {"ids": ["a"], "#ids": {"resultOf": "c0", "name": "Email/query", "path": "/ids"}}, "c1"]
		]}`,
		want: `argument "ids" is already set`,
	}, {
		name: "parameter used with two types",
		src:  `{"methodCalls": [["Email/query", {"accountId": "{{x}}", "limit": "{{x}}"}, "c0"]]}`,
		want: `parameter "x" is Id where it is first used`,
	}, {
		name: "parameter inside a string",
		src:  `{"methodCalls": [["Email/query", {"accountId": "acct-{{x}}"}, "c0"]]}`,
		want: `a parameter cannot be embedded in a larger string`,
	}, {
		name: "missing capability",
		src:  `{"using": ["core"], "methodCalls": [["Email/query", {}, "c0"]]}`,
		want: `needs the capability urn:ietf:params:jmap:mail`,
	}, {
		name: "unknown returns",
		src:  `{"methodCalls": [["Email/query", {}, "c0"]], "returns": "c9"}`,
		want: `no method call has the id "c9"`,
	}, {
		name: "invalid id literal",
		src:  `{"methodCalls": [["Email/query", {"accountId": "not a valid id"}, "c0"]]}`,
		want: `is not a valid id`,
	}, {
		name: "bad date",
		src:  `{"methodCalls": [["Email/query", {"filter": {"before": "yesterday"}}, "c0"]]}`,
		want: `is not a UTCDate`,
	}, {
		name: "null where a value is required",
		src:  `{"methodCalls": [["Email/query", {"accountId": null}, "c0"]]}`,
		want: `Id may not be null`,
	}, {
		name: "unknown top-level member",
		src:  `{"methodCall": []}`,
		want: `unknown field "methodCall"`,
	}, {
		name: "no method calls",
		src:  `{"methodCalls": []}`,
		want: `the query makes no method calls`,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseErr(t, tt.src)
			if !strings.Contains(got, tt.want) {
				t.Errorf("error was:\n%s\nwant it to mention: %s", got, tt.want)
			}
		})
	}
}

// TestParseSuggests checks that a misspelling is met with a suggestion, because
// a generator that only says "no" makes for slow going.
func TestParseSuggests(t *testing.T) {
	got := parseErr(t, `{"methodCalls": [["Email/query", {"limmit": 10}, "c0"]]}`)
	if !strings.Contains(got, `did you mean "limit"?`) {
		t.Errorf("error was:\n%s\nwant it to suggest limit", got)
	}
}

// TestParseAcceptsCreationIDs checks that a /set call may refer to a record it
// creates in the same request.
func TestParseAcceptsCreationIDs(t *testing.T) {
	parse(t, "FileEmail"+Extension, `{
      "methodCalls": [
        ["Mailbox/set", {
          "accountId": "{{accountId}}",
          "create": {"folder": {"name": "{{name}}", "parentId": null}}
        }, "c0"],
        ["Email/set", {
          "accountId": "{{accountId}}",
          "update": {"{{emailId}}": {"mailboxIds/#folder": true}}
        }, "c1"]
      ]
    }`)
}
