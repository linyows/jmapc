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

// TestPatchObjectIsChecked covers the members of a PatchObject, whose keys are
// JSON pointers into the record being patched. Nothing in the type of a
// PatchObject says what it patches, so the catalogue records that on the
// argument carrying it; without that, a misspelled pointer would reach the
// server as a property nothing reads.
func TestPatchObjectIsChecked(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "misspelled property",
		src:  `{"methodCalls": [["Email/set", {"update": {"e1": {"mailboxIDs/m1": true}}}, "c0"]]}`,
		want: `Email has no property "mailboxIDs"`,
	}, {
		name: "wrong value type",
		src:  `{"methodCalls": [["Email/set", {"update": {"e1": {"keywords/$seen": "yes"}}}, "c0"]]}`,
		want: `expected Boolean|null, found a string`,
	}, {
		name: "pointer into a scalar",
		src:  `{"methodCalls": [["Email/set", {"update": {"e1": {"subject/x": "a"}}}, "c0"]]}`,
		want: `cannot look inside String|null`,
	}, {
		name: "misspelled property in a submission side effect",
		src: `{"methodCalls": [["EmailSubmission/set", {
			"onSuccessUpdateEmail": {"#send": {"keywrds/$draft": null}}
		}, "c0"]]}`,
		want: `Email has no property "keywrds"`,
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

// TestPatchObjectAllowsRemoval checks that null is accepted anywhere in a patch,
// because in a patch it means "remove this" rather than "set this to null".
func TestPatchObjectAllowsRemoval(t *testing.T) {
	parse(t, "Unfile"+Extension, `{"methodCalls": [
	  ["Email/set", {"update": {"{{emailId}}": {
	    "mailboxIds/{{fromMailboxId}}": null,
	    "mailboxIds/{{toMailboxId}}": true,
	    "keywords/$seen": null
	  }}}, "c0"]
	]}`)
}

// TestPatchPointerParametersAreTyped checks that a parameter standing in for a
// segment of a patch pointer takes the type that segment selects by, so that it
// agrees with the same parameter used elsewhere.
func TestPatchPointerParametersAreTyped(t *testing.T) {
	q := parse(t, "Refile"+Extension, `{"methodCalls": [
	  ["Email/set", {
	    "create": {"draft": {"mailboxIds": {"{{mailboxId}}": true}}},
	    "update": {"e1": {"mailboxIds/{{mailboxId}}": true}}
	  }, "c0"]
	]}`)
	if len(q.Params) != 1 {
		t.Fatalf("got %d parameters, want 1", len(q.Params))
	}
	if got := q.Params[0].GoType("jmapc."); got != "jmapc.ID" {
		t.Errorf("mailboxId is %s, want jmapc.ID", got)
	}
}

// TestPatchPointerParameterAloneIsAString checks that a parameter naming a
// property, where nothing says what that property is, still generates something
// usable rather than failing.
func TestPatchPointerParameterAloneIsAString(t *testing.T) {
	q := parse(t, "SetKeyword"+Extension, `{"methodCalls": [
	  ["Email/set", {"update": {"e1": {"keywords/{{keyword}}": true}}}, "c0"]
	]}`)
	if got := q.Params[0].GoType("jmapc."); got != "string" {
		t.Errorf("keyword is %s, want string", got)
	}
}

// TestEchoTakesAnyArguments checks that Core/echo accepts whatever it is given,
// since its whole purpose is to hand it back.
func TestEchoTakesAnyArguments(t *testing.T) {
	parse(t, "Ping"+Extension, `{"methodCalls": [
	  ["Core/echo", {"anything": [1, "two", {"three": true}], "value": "{{value}}"}, "c0"]
	]}`)
}

// TestSubmissionQuery covers a query that spans two capabilities, checking that
// both are declared.
func TestSubmissionQuery(t *testing.T) {
	q := parse(t, "Send"+Extension, `{"methodCalls": [
	  ["Email/set", {"create": {"draft": {"subject": "{{subject}}"}}}, "write"],
	  ["EmailSubmission/set", {"create": {"send": {
	    "emailId": "#draft", "identityId": "{{identityId}}"
	  }}}, "send"]
	]}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:mail urn:ietf:params:jmap:submission"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

// TestSortIsChecked covers the sort order of a /query. A server is only obliged
// to sort on the properties its specification names, so sorting on anything
// else is a query that would fail against a conforming server.
func TestSortIsChecked(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "unsortable property",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"property": "preview"}]}, "c0"]]}`,
		want: `Email cannot be sorted by "preview"`,
	}, {
		name: "misspelled property",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"property": "recievedAt"}]}, "c0"]]}`,
		want: `did you mean "receivedAt"?`,
	}, {
		name: "wrong type's property",
		src:  `{"methodCalls": [["Mailbox/query", {"sort": [{"property": "receivedAt"}]}, "c0"]]}`,
		want: `Mailbox cannot be sorted by "receivedAt"`,
	}, {
		name: "missing property",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"isAscending": false}]}, "c0"]]}`,
		want: `a comparator must say which property to sort by`,
	}, {
		name: "keyword sort without its keyword",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"property": "hasKeyword"}]}, "c0"]]}`,
		want: `sorting by "hasKeyword" needs the comparator to also set "keyword"`,
	}, {
		name: "keyword on a sort that does not take one",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"property": "subject", "keyword": "$seen"}]}, "c0"]]}`,
		want: `a comparator has no member "keyword"`,
	}, {
		name: "unknown comparator member",
		src:  `{"methodCalls": [["Email/query", {"sort": [{"property": "subject", "ascending": true}]}, "c0"]]}`,
		want: `a comparator has no member "ascending"`,
	}, {
		name: "queryChanges sorts too",
		src:  `{"methodCalls": [["Email/queryChanges", {"sinceQueryState": "s", "sort": [{"property": "nope"}]}, "c0"]]}`,
		want: `Email cannot be sorted by "nope"`,
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

// TestSortAccepts checks the comparators that should pass, including the
// keyword form and a property the caller supplies.
func TestSortAccepts(t *testing.T) {
	parse(t, "Sorted"+Extension, `{"methodCalls": [
	  ["Email/query", {"sort": [
	    {"property": "hasKeyword", "keyword": "$flagged", "isAscending": false},
	    {"property": "receivedAt", "isAscending": false},
	    {"property": "subject", "collation": "i;ascii-casemap"}
	  ]}, "c0"],
	  ["Email/query", {"sort": [{"property": "{{sortBy}}", "keyword": "{{keyword}}"}]}, "c1"]
	]}`)
}

// TestFilterConditionIsPreferredOverOperator checks that a filter holding
// nothing but a misspelled condition is reported as a condition, not as a
// FilterOperator missing its operator. Both shapes are possible in that
// position, and only one of them is what the author meant.
func TestFilterConditionIsPreferredOverOperator(t *testing.T) {
	tests := []struct{ src, want string }{{
		`{"methodCalls": [["Email/query", {"filter": {"inMailboxx": "abc"}}, "c0"]]}`,
		`EmailFilterCondition has no property "inMailboxx"`,
	}, {
		`{"methodCalls": [["ContactCard/query", {"filter": {"inAddressBok": "abc"}}, "c0"]]}`,
		`ContactCardFilterCondition has no property "inAddressBok"`,
	}}
	for _, tt := range tests {
		got := parseErr(t, tt.src)
		if !strings.Contains(got, tt.want) {
			t.Errorf("error was:\n%s\nwant it to mention: %s", got, tt.want)
		}
	}
}

// TestContactsQuery covers a query against JMAP for Contacts, whose cards are
// JSContact objects rather than anything JMAP defines itself.
func TestContactsQuery(t *testing.T) {
	q := parse(t, "SearchContacts"+Extension, `{
	  "methodCalls": [
	    ["ContactCard/query", {
	      "filter": {
	        "operator": "AND",
	        "conditions": [
	          {"inAddressBook": "{{addressBookId}}"},
	          {"text": "{{phrase}}"},
	          {"kind": "individual"}
	        ]
	      },
	      "sort": [{"property": "name/surname"}, {"property": "name/given"}]
	    }, "search"],
	    ["ContactCard/get", {
	      "#ids": {"resultOf": "search", "name": "ContactCard/query", "path": "/ids"},
	      "properties": ["id", "uid", "name", "emails", "phones"]
	    }, "fetch"]
	  ],
	  "returns": "fetch"
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:contacts"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
	if got := q.Params[0].GoType("jmapc."); got != "jmapc.ID" {
		t.Errorf("addressBookId is %s, want jmapc.ID", got)
	}
}

// TestContactsChecks covers the mistakes a contacts query can make.
func TestContactsChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "misspelled card property",
		src:  `{"methodCalls": [["ContactCard/get", {"properties": ["id", "nickname"]}, "c0"]]}`,
		want: `did you mean "nicknames"?`,
	}, {
		name: "unsortable property",
		src:  `{"methodCalls": [["ContactCard/query", {"sort": [{"property": "email"}]}, "c0"]]}`,
		want: `ContactCard cannot be sorted by "email"`,
	}, {
		name: "address book has no query",
		src:  `{"methodCalls": [["AddressBook/query", {}, "c0"]]}`,
		want: `unknown method "AddressBook/query"`,
	}, {
		name: "patch into a card",
		src: `{"methodCalls": [["ContactCard/set", {
			"update": {"c1": {"nicknames/n1/nme": "Bob"}}
		}, "c0"]]}`,
		want: `ContactNickname has no property "nme"`,
	}, {
		name: "localizations patch the card itself",
		src: `{"methodCalls": [["ContactCard/set", {
			"create": {"c": {"localizations": {"de": {"titles/t1/nme": "Ingenieur"}}}}
		}, "c0"]]}`,
		want: `ContactTitle has no property "nme"`,
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
