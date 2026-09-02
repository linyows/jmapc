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
  "_doc": "ListInboxEmails returns the most recent emails in one mailbox.",
  "methodCalls": [
    ["Email/query", {
      "_comment": "Find the ids of the matching emails, newest first.",
      "accountId": "{{accountId}}",
      "filter": {"inMailbox": "{{mailboxId}}"},
      "sort": [{"property": "receivedAt", "isAscending": false}],
      "limit": "{{limit}}"
    }, "c0"],
    ["Email/get", {
      "_comment": "Then fetch them, without a second round trip.",
      "accountId": "{{accountId}}",
      "#ids": {"resultOf": "c0", "name": "Email/query", "path": "/ids"},
      "properties": ["id", "subject", "from", "receivedAt"]
    }, "c1"]
  ],
  "_returns": "c1"
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

	if got, want := q.Calls[0].Comment, "Find the ids of the matching emails, newest first."; got != want {
		t.Errorf("comment = %q, want %q", got, want)
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
		src:  `{"methodCalls": [["Email/query", {}, "c0"]], "_returns": "c9"}`,
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
	  "_returns": "fetch"
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

// TestCalendarsQuery covers a query against JMAP for Calendars, whose events
// are JSCalendar objects and whose times are in kinds JMAP itself does not
// have.
func TestCalendarsQuery(t *testing.T) {
	q := parse(t, "Agenda"+Extension, `{
	  "methodCalls": [
	    ["CalendarEvent/query", {
	      "filter": {
	        "operator": "AND",
	        "conditions": [
	          {"inCalendar": "{{calendarId}}"},
	          {"after": "{{from}}"},
	          {"before": "{{until}}"}
	        ]
	      },
	      "sort": [{"property": "start"}],
	      "expandRecurrences": true,
	      "timeZone": "{{timeZone}}"
	    }, "search"],
	    ["CalendarEvent/get", {
	      "#ids": {"resultOf": "search", "name": "CalendarEvent/query", "path": "/ids"},
	      "properties": ["id", "title", "start", "duration", "participants"],
	      "reduceParticipants": true
	    }, "fetch"]
	  ],
	  "_returns": "fetch"
	}`)

	want := map[string]string{
		"calendarId": "jmapc.ID",
		"from":       "jmapc.LocalDateTime",
		"until":      "jmapc.LocalDateTime",
		"timeZone":   "jmapc.TimeZoneID",
	}
	for _, p := range q.Params {
		if got := p.GoType("jmapc."); got != want[p.Name] {
			t.Errorf("parameter %q is %s, want %s", p.Name, got, want[p.Name])
		}
	}
}

// TestCalendarTimeTypes checks the types JSCalendar adds, which are easy to get
// wrong by reaching for the ones JMAP already had.
func TestCalendarTimeTypes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "local date-time with a zone",
		src:  `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {"start": "2024-05-01T09:00:00Z"}}}, "c0"]]}`,
		want: `is not a LocalDateTime`,
	}, {
		name: "local date-time with a space",
		src:  `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {"start": "2024-05-01 09:00:00"}}}, "c0"]]}`,
		want: `is not a LocalDateTime`,
	}, {
		name: "duration in months",
		src:  `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {"duration": "P1M"}}}, "c0"]]}`,
		want: `is not a Duration`,
	}, {
		name: "duration in Go syntax",
		src:  `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {"duration": "90m"}}}, "c0"]]}`,
		want: `is not a Duration`,
	}, {
		name: "signed duration on an alert",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"alerts": {"a": {"trigger": {"@type": "OffsetTrigger", "offset": "15m"}}}
		}}}, "c0"]]}`,
		want: `is not a SignedDuration`,
	}, {
		name: "unsortable property",
		src:  `{"methodCalls": [["CalendarEvent/query", {"sort": [{"property": "title"}]}, "c0"]]}`,
		want: `CalendarEvent cannot be sorted by "title"`,
	}, {
		name: "misspelled event property",
		src:  `{"methodCalls": [["CalendarEvent/get", {"properties": ["id", "titel"]}, "c0"]]}`,
		want: `did you mean "title"?`,
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

// TestCalendarTimeTypesAccept checks the forms that should pass, including a
// negative offset, which is how an alert says it fires beforehand.
func TestCalendarTimeTypesAccept(t *testing.T) {
	parse(t, "MakeEvent"+Extension, `{"methodCalls": [
	  ["CalendarEvent/set", {"create": {"e": {
	    "@type": "Event",
	    "uid": "{{uid}}",
	    "title": "{{title}}",
	    "start": "2024-05-01T09:00:00",
	    "duration": "PT1H30M",
	    "timeZone": "Europe/London",
	    "alerts": {
	      "a": {"trigger": {"@type": "OffsetTrigger", "offset": "-PT15M"}},
	      "b": {"trigger": {"@type": "AbsoluteTrigger", "when": "2024-05-01T07:00:00Z"}}
	    },
	    "recurrenceRules": [{"frequency": "weekly", "byDay": [{"day": "mo"}], "count": 10}]
	  }}}, "c0"]
	]}`)
}

// TestRecurrenceOverridesArePatches checks the overrides of a recurring event,
// which are patches to the event keyed by the start of the occurrence they
// change. Both halves are checked: the key is a local date-time, and the value
// is a patch into the event.
func TestRecurrenceOverridesArePatches(t *testing.T) {
	parse(t, "MoveOccurrence"+Extension, `{"methodCalls": [
	  ["CalendarEvent/set", {"update": {"{{eventId}}": {
	    "recurrenceOverrides/2024-05-06T09:00:00/title": "Moved",
	    "recurrenceOverrides/2024-05-13T09:00:00/excluded": true
	  }}}, "c0"]
	]}`)

	got := parseErr(t, `{"methodCalls": [
	  ["CalendarEvent/set", {"update": {"e1": {
	    "recurrenceOverrides/2024-05-06T09:00:00/titel": "Moved"
	  }}}, "c0"]
	]}`)
	if !strings.Contains(got, `did you mean "title"?`) {
		t.Errorf("error was:\n%s\nwant it to suggest title", got)
	}
}

// TestAvailability covers Principal/getAvailability, which belongs to a
// capability of its own.
func TestAvailability(t *testing.T) {
	q := parse(t, "WhenFree"+Extension, `{
	  "methodCalls": [["Principal/getAvailability", {
	    "id": "{{principalId}}",
	    "utcStart": "2024-05-01T00:00:00Z",
	    "utcEnd": "2024-05-08T00:00:00Z",
	    "showDetails": true,
	    "eventProperties": ["title", "start", "duration"]
	  }, "c0"]]
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:principals:availability"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

// TestEnumeratedValuesAreChecked covers properties whose specification fixes
// the values they may take. A misspelling here would otherwise reach the
// server, which is free to ignore a value it does not recognise, so the query
// would appear to work while doing nothing.
func TestEnumeratedValuesAreChecked(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "filter operator",
		src: `{"methodCalls": [["Email/query", {"filter": {
			"operator": "BOTH", "conditions": [{"hasAttachment": true}]
		}}, "c0"]]}`,
		want: `"BOTH" is not one of the values this property takes (AND, OR, NOT)`,
	}, {
		name: "event status",
		src:  `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {"status": "confirmd"}}}, "c0"]]}`,
		want: `did you mean "confirmed"?`,
	}, {
		name: "recurrence frequency",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"recurrenceRules": [{"frequency": "forthnightly"}]
		}}}, "c0"]]}`,
		want: `"forthnightly" is not one of the values this property takes`,
	}, {
		name: "day of the week",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"recurrenceRules": [{"frequency": "weekly", "byDay": [{"day": "monday"}]}]
		}}}, "c0"]]}`,
		want: `"monday" is not one of the values this property takes (mo, tu, we, th, fr, sa, su)`,
	}, {
		name: "participation status",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"participants": {"p": {"roles": {"attendee": true}, "participationStatus": "maybe"}}
		}}}, "c0"]]}`,
		want: `"maybe" is not one of the values this property takes`,
	}, {
		name: "a set's keys are the enumerated values",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"participants": {"p": {"roles": {"organiser": true}}}
		}}}, "c0"]]}`,
		want: `"organiser" is not one of the values this property takes (owner, attendee, optional, informational, chair, contact)`,
	}, {
		name: "contact card kind",
		src:  `{"methodCalls": [["ContactCard/set", {"create": {"c": {"kind": "person"}}}, "c0"]]}`,
		want: `"person" is not one of the values this property takes`,
	}, {
		name: "name component kind",
		src: `{"methodCalls": [["ContactCard/set", {"create": {"c": {
			"name": {"components": [{"kind": "first", "value": "Ada"}]}
		}}}, "c0"]]}`,
		want: `"first" is not one of the values this property takes`,
	}, {
		name: "undo status",
		src:  `{"methodCalls": [["EmailSubmission/set", {"update": {"s1": {"undoStatus": "cancelled"}}}, "c0"]]}`,
		want: `did you mean "canceled"?`,
	}, {
		name: "alert action",
		src: `{"methodCalls": [["CalendarEvent/set", {"create": {"e": {
			"alerts": {"a": {"trigger": {"@type": "AbsoluteTrigger", "when": "2024-05-01T09:00:00Z"}, "action": "notify"}}
		}}}, "c0"]]}`,
		want: `"notify" is not one of the values this property takes (display, email)`,
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

// TestEnumeratedValuesAccept checks that the values a specification does allow
// pass, including through an array and a set.
func TestEnumeratedValuesAccept(t *testing.T) {
	parse(t, "Enumerated"+Extension, `{"methodCalls": [
	  ["CalendarEvent/set", {"create": {"e": {
	    "@type": "Event",
	    "status": "tentative",
	    "privacy": "private",
	    "freeBusyStatus": "free",
	    "recurrenceRules": [{
	      "frequency": "monthly",
	      "byDay": [{"day": "we"}, {"day": "fr"}],
	      "skip": "backward",
	      "firstDayOfWeek": "su"
	    }],
	    "participants": {"p": {
	      "roles": {"owner": true, "attendee": true},
	      "kind": "individual",
	      "participationStatus": "accepted",
	      "scheduleAgent": "client"
	    }},
	    "alerts": {"a": {"trigger": {"@type": "OffsetTrigger", "offset": "-PT5M"}, "action": "email"}}
	  }}}, "c0"],
	  ["ContactCard/set", {"create": {"c": {
	    "kind": "org",
	    "name": {"components": [{"kind": "surname", "value": "Lovelace"}]},
	    "anniversaries": {"a": {"kind": "birth", "date": {"year": 1815}}}
	  }}}, "c1"]
	]}`)
}

// TestOpenSetsAreNotChecked checks that properties whose values a specification
// leaves open still take anything. Rejecting a value the server would have
// accepted is worse than letting a typo through.
func TestOpenSetsAreNotChecked(t *testing.T) {
	parse(t, "OpenSets"+Extension, `{"methodCalls": [
	  ["Mailbox/set", {"create": {"m": {"name": "Receipts", "role": "x-vendor-receipts"}}}, "c0"],
	  ["Email/set", {"create": {"e": {"keywords": {"$seen": true, "anything-at-all": true}}}}, "c1"]
	]}`)
}

// TestPrincipalsQuery covers JMAP Sharing: finding the principals an account
// can be shared with, and reading what has recently been shared.
func TestPrincipalsQuery(t *testing.T) {
	q := parse(t, "FindPeople"+Extension, `{
	  "methodCalls": [
	    ["Principal/query", {
	      "filter": {
	        "operator": "AND",
	        "conditions": [{"text": "{{phrase}}"}, {"type": "individual"}]
	      },
	      "limit": "{{limit}}"
	    }, "search"],
	    ["Principal/get", {
	      "#ids": {"resultOf": "search", "name": "Principal/query", "path": "/ids"},
	      "properties": ["id", "type", "name", "email", "timeZone"]
	    }, "fetch"]
	  ],
	  "_returns": "fetch"
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:principals"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

// TestShareNotificationQuery covers the other half: what has been shared with
// the user, which nothing else would tell them.
func TestShareNotificationQuery(t *testing.T) {
	parse(t, "RecentlyShared"+Extension, `{
	  "methodCalls": [
	    ["ShareNotification/query", {
	      "filter": {"after": "{{since}}", "objectType": "Mailbox"},
	      "sort": [{"property": "created", "isAscending": false}]
	    }, "search"],
	    ["ShareNotification/get", {
	      "#ids": {"resultOf": "search", "name": "ShareNotification/query", "path": "/ids"}
	    }, "fetch"]
	  ]
	}`)
}

// TestPrincipalsChecks covers the mistakes a sharing query can make.
func TestPrincipalsChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "principal type",
		src:  `{"methodCalls": [["Principal/query", {"filter": {"type": "person"}}, "c0"]]}`,
		want: `"person" is not one of the values this property takes (individual, group, resource, location, other)`,
	}, {
		name: "misspelled property",
		src:  `{"methodCalls": [["Principal/get", {"properties": ["id", "emial"]}, "c0"]]}`,
		want: `did you mean "email"?`,
	}, {
		name: "notification sort",
		src:  `{"methodCalls": [["ShareNotification/query", {"sort": [{"property": "name"}]}, "c0"]]}`,
		want: `ShareNotification cannot be sorted by "name"`,
	}, {
		name: "back reference into accounts",
		src: `{"methodCalls": [
			["Principal/get", {}, "c0"],
			["Mailbox/get", {"#ids": {"resultOf": "c0", "name": "Principal/get", "path": "/list/*/acounts"}}, "c1"]
		]}`,
		want: `did you mean "accounts"?`,
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

// TestUnspecifiedSortIsAllowed checks that a type whose specification leaves the
// sortable properties to the server accepts any comparator. Principal is such a
// type, and rejecting a sort the server would have honoured is worse than
// letting one through.
func TestUnspecifiedSortIsAllowed(t *testing.T) {
	parse(t, "SortedPeople"+Extension, `{"methodCalls": [
	  ["Principal/query", {"sort": [{"property": "name"}, {"property": "email"}]}, "c0"]
	]}`)
}

// TestSMIMECapabilityIsTracked covers a capability that adds properties to a
// type belonging to another specification. Nothing in the method names says
// RFC 9219 is involved; only the properties do.
func TestSMIMECapabilityIsTracked(t *testing.T) {
	// Derived from the properties, when the query says nothing.
	q := parse(t, "Signed"+Extension, `{"methodCalls": [
	  ["Email/query", {"filter": {"hasVerifiedSmime": true}}, "c0"],
	  ["Email/get", {
	    "#ids": {"resultOf": "c0", "name": "Email/query", "path": "/ids"},
	    "properties": ["id", "subject", "smimeStatus"]
	  }, "c1"]
	]}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:mail urn:ietf:params:jmap:smimeverify"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}

	// Reported, when the query declares a set that does not cover it.
	got := parseErr(t, `{
	  "using": ["core", "mail"],
	  "methodCalls": [["Email/get", {"ids": ["e1"], "properties": ["id", "smimeStatus"]}, "c0"]]
	}`)
	if !strings.Contains(got, "uses properties from urn:ietf:params:jmap:smimeverify") {
		t.Errorf("error was:\n%s\nwant it to name the undeclared capability", got)
	}

	// A query that touches none of them needs nothing extra.
	plain := parse(t, "Plain"+Extension, `{"methodCalls": [
	  ["Email/get", {"ids": ["e1"], "properties": ["id", "subject"]}, "c0"]
	]}`)
	if strings.Contains(strings.Join(plain.Using, " "), "smimeverify") {
		t.Errorf("Using = %v, want no S/MIME capability", plain.Using)
	}
}

// TestSMIMEStatusValues checks the values the verification status may take,
// which are easy to guess wrong: "verified" on its own is not one of them.
func TestSMIMEStatusValues(t *testing.T) {
	got := parseErr(t, `{"methodCalls": [
	  ["Email/set", {"update": {"e1": {"smimeStatus": "verified"}}}, "c0"]
	]}`)
	if !strings.Contains(got, `"verified" is not one of the values this property takes`) {
		t.Errorf("error was:\n%s", got)
	}
	parse(t, "Statuses"+Extension, `{"methodCalls": [
	  ["Email/query", {"filter": {
	    "operator": "OR",
	    "conditions": [{"hasSmime": true}, {"hasVerifiedSmimeAtDelivery": true}]
	  }}, "c0"]
	]}`)
}

// TestBlobExtension covers RFC 9404, which brings blob creation and reading
// into the API. The point of it is that a blob can be created and used within
// one request, which the upload endpoint of RFC 8620 cannot do.
func TestBlobExtension(t *testing.T) {
	q := parse(t, "AttachNote"+Extension, `{
	  "methodCalls": [
	    ["Blob/upload", {
	      "create": {"note": {"data": [{"data:asText": "{{note}}"}], "type": "text/plain"}}
	    }, "upload"],
	    ["Email/set", {
	      "create": {"draft": {
	        "subject": "{{subject}}",
	        "attachments": [{"blobId": "#note", "type": "text/plain"}]
	      }}
	    }, "draft"]
	  ]
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:blob urn:ietf:params:jmap:mail"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

// TestBlobProperties covers the properties Blob/get takes, which mix fixed
// names with ones the server gives meaning to.
func TestBlobProperties(t *testing.T) {
	// A digest names an algorithm the session says it supports, and "data"
	// asks the server to choose an encoding, so neither is a fixed property.
	parse(t, "Peek"+Extension, `{"methodCalls": [
	  ["Blob/get", {
	    "ids": ["{{blobId}}"],
	    "properties": ["data:asText", "data:asBase64", "data", "size", "digest:sha-256"],
	    "offset": 0,
	    "length": 4096
	  }, "c0"]
	]}`)

	got := parseErr(t, `{"methodCalls": [
	  ["Blob/get", {"ids": ["b1"], "properties": ["data:asTxt"]}, "c0"]
	]}`)
	if !strings.Contains(got, `did you mean "data:asText"?`) {
		t.Errorf("error was:\n%s", got)
	}
}

// TestBlobLookup covers the method that says which records refer to a blob.
func TestBlobLookup(t *testing.T) {
	parse(t, "WhatUses"+Extension, `{"methodCalls": [
	  ["Blob/lookup", {"typeNames": ["Email", "Mailbox"], "ids": ["{{blobId}}"]}, "c0"]
	]}`)

	got := parseErr(t, `{"methodCalls": [
	  ["Blob/lookup", {"typeNames": ["Email"], "ids": ["b1"], "extra": true}, "c0"]
	]}`)
	if !strings.Contains(got, `Blob/lookup has no argument "extra"`) {
		t.Errorf("error was:\n%s", got)
	}
}

// TestQuota covers JMAP Quotas, which a client can only read: there is no
// Quota/set, because nothing about a limit is the client's to decide.
func TestQuota(t *testing.T) {
	q := parse(t, "MailQuota"+Extension, `{
	  "methodCalls": [
	    ["Quota/query", {
	      "filter": {"operator": "AND", "conditions": [
	        {"resourceType": "octets"},
	        {"type": "Mail"}
	      ]},
	      "sort": [{"property": "used", "isAscending": false}]
	    }, "search"],
	    ["Quota/get", {
	      "#ids": {"resultOf": "search", "name": "Quota/query", "path": "/ids"},
	      "properties": ["id", "name", "used", "hardLimit", "warnLimit"]
	    }, "fetch"]
	  ],
	  "_returns": "fetch"
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:quota"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

func TestQuotaChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "no set method",
		src:  `{"methodCalls": [["Quota/set", {"update": {"q1": {"hardLimit": 1000}}}, "c0"]]}`,
		want: `unknown method "Quota/set"`,
	}, {
		name: "resource type",
		src:  `{"methodCalls": [["Quota/query", {"filter": {"resourceType": "bytes"}}, "c0"]]}`,
		want: `"bytes" is not one of the values this property takes (count, octets)`,
	}, {
		name: "scope",
		src:  `{"methodCalls": [["Quota/query", {"filter": {"scope": "user"}}, "c0"]]}`,
		want: `"user" is not one of the values this property takes (account, domain, global)`,
	}, {
		name: "unsortable property",
		src:  `{"methodCalls": [["Quota/query", {"sort": [{"property": "hardLimit"}]}, "c0"]]}`,
		want: `Quota cannot be sorted by "hardLimit"`,
	}, {
		name: "misspelled property",
		src:  `{"methodCalls": [["Quota/get", {"properties": ["id", "hardLimt"]}, "c0"]]}`,
		want: `did you mean "hardLimit"?`,
	}, {
		name: "updatedProperties is on the response, not the arguments",
		src:  `{"methodCalls": [["Quota/changes", {"sinceState": "s", "updatedProperties": ["used"]}, "c0"]]}`,
		want: `Quota/changes has no argument "updatedProperties"`,
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

// TestQuotaChangesReportsUpdatedProperties checks that the response property
// RFC 9425 adds resolves, since a client uses it to avoid refetching a whole
// quota when only its used value moved.
func TestQuotaChangesReportsUpdatedProperties(t *testing.T) {
	parse(t, "QuotaSince"+Extension, `{"methodCalls": [
	  ["Quota/changes", {"sinceState": "{{state}}"}, "c0"],
	  ["Quota/get", {
	    "#ids": {"resultOf": "c0", "name": "Quota/changes", "path": "/updated"},
	    "#properties": {"resultOf": "c0", "name": "Quota/changes", "path": "/updatedProperties"}
	  }, "c1"]
	]}`)
}

// TestCommentArgumentIsNotSent checks the member a query uses to explain a
// call. It has to be recognised and set aside: RFC 8620 requires a server to
// reject an argument it does not know, so leaving it among the arguments would
// make every annotated query fail.
func TestCommentArgumentIsNotSent(t *testing.T) {
	q := parse(t, "Annotated"+Extension, `{"methodCalls": [
	  ["Email/query", {
	    "_comment": "Why this call is here.",
	    "filter": {"inMailbox": "{{mailboxId}}"}
	  }, "c0"]
	]}`)
	if got, want := q.Calls[0].Comment, "Why this call is here."; got != want {
		t.Errorf("comment = %q, want %q", got, want)
	}
	if _, sent := q.Calls[0].Args.Find(CommentArgument); sent {
		t.Error("the comment is among the arguments, and would be sent to the server")
	}

	if got := parseErr(t, `{"methodCalls": [
	  ["Email/query", {"_comment": ["not", "a", "string"]}, "c0"]
	]}`); !strings.Contains(got, "_comment must be a string") {
		t.Errorf("error was:\n%s", got)
	}
}

// TestJSONCommentsAreNotAccepted checks that a query file is plain JSON. It was
// not always: comments were stripped before parsing, which meant a file named
// .json that no other tool could read.
func TestJSONCommentsAreNotAccepted(t *testing.T) {
	got := parseErr(t, `{
	  // Find the emails.
	  "methodCalls": [["Email/query", {}, "c0"]]
	}`)
	if !strings.Contains(got, "invalid character") {
		t.Errorf("error was:\n%s\nwant a JSON syntax error", got)
	}
}

// TestSieveScript covers JMAP for Sieve Scripts. A script's text is a blob
// rather than a property, so installing one takes an upload and a store, and
// the blob extension lets both happen in one request.
func TestSieveScript(t *testing.T) {
	q := parse(t, "InstallScript"+Extension, `{
	  "methodCalls": [
	    ["Blob/upload", {
	      "create": {"text": {"data": [{"data:asText": "{{script}}"}], "type": "application/sieve"}}
	    }, "upload"],
	    ["SieveScript/set", {
	      "create": {"filter": {"name": "{{name}}", "blobId": "#text"}},
	      "onSuccessDeactivateScript": true,
	      "onSuccessActivateScript": "#filter"
	    }, "install"]
	  ],
	  "_returns": "install"
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:blob urn:ietf:params:jmap:sieve"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

// TestSieveValidateTakesABlob checks the method that parses a script without
// storing it, whose only input is a blob the caller uploaded first.
func TestSieveValidateTakesABlob(t *testing.T) {
	parse(t, "CheckScript"+Extension, `{"methodCalls": [
	  ["Blob/upload", {"create": {"draft": {"data": [{"data:asText": "{{script}}"}]}}}, "upload"],
	  ["SieveScript/validate", {
	    "#blobId": {"resultOf": "upload", "name": "Blob/upload", "path": "/created/draft/id"}
	  }, "check"]
	]}`)
}

func TestSieveChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "no changes method",
		src:  `{"methodCalls": [["SieveScript/changes", {"sinceState": "s"}, "c0"]]}`,
		want: `unknown method "SieveScript/changes"`,
	}, {
		name: "misspelled property",
		src:  `{"methodCalls": [["SieveScript/get", {"properties": ["id", "isActiv"]}, "c0"]]}`,
		want: `did you mean "isActive"?`,
	}, {
		name: "unsortable property",
		src:  `{"methodCalls": [["SieveScript/query", {"sort": [{"property": "blobId"}]}, "c0"]]}`,
		want: `SieveScript cannot be sorted by "blobId"`,
	}, {
		name: "activation takes an id, not a name",
		src: `{"methodCalls": [["SieveScript/set", {
			"onSuccessActivateScript": {"name": "vacation"}
		}, "c0"]]}`,
		want: `expected Id|null, found an object`,
	}, {
		name: "validate needs a blob",
		src:  `{"methodCalls": [["SieveScript/validate", {"script": "keep;"}, "c0"]]}`,
		want: `SieveScript/validate has no argument "script"`,
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

// TestMDN covers message disposition notifications. An MDN is not stored: it is
// a message the server composes and sends, so there is no /get and no /set.
func TestMDN(t *testing.T) {
	q := parse(t, "SendReadReceipt"+Extension, `{
	  "methodCalls": [
	    ["MDN/send", {
	      "identityId": "{{identityId}}",
	      "send": {
	        "receipt": {
	          "forEmailId": "{{emailId}}",
	          "subject": "{{subject}}",
	          "disposition": {
	            "actionMode": "manual-action",
	            "sendingMode": "mdn-sent-manually",
	            "type": "displayed"
	          }
	        }
	      },
	      "onSuccessUpdateEmail": {"#receipt": {"keywords/$mdnsent": true}}
	    }, "send"]
	  ],
	  "_returns": "send"
	}`)
	want := "urn:ietf:params:jmap:core urn:ietf:params:jmap:mdn"
	if got := strings.Join(q.Using, " "); got != want {
		t.Errorf("Using = %q, want %q", got, want)
	}
}

func TestMDNChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "no get method",
		src:  `{"methodCalls": [["MDN/get", {"ids": ["m1"]}, "c0"]]}`,
		want: `unknown method "MDN/get"`,
	}, {
		name: "disposition type",
		src: `{"methodCalls": [["MDN/send", {"send": {"r": {
			"disposition": {"actionMode": "manual-action", "sendingMode": "mdn-sent-manually", "type": "read"}
		}}}, "c0"]]}`,
		want: `"read" is not one of the values this property takes (deleted, dispatched, displayed, processed)`,
	}, {
		name: "sending mode",
		src: `{"methodCalls": [["MDN/send", {"send": {"r": {
			"disposition": {"sendingMode": "automatic"}
		}}}, "c0"]]}`,
		want: `"automatic" is not one of the values this property takes`,
	}, {
		name: "misspelled property",
		src:  `{"methodCalls": [["MDN/send", {"send": {"r": {"forEmailID": "e1"}}}, "c0"]]}`,
		want: `did you mean "forEmailId"?`,
	}, {
		// The patch applied on success goes to the Email, not to the MDN, so
		// it is checked against Email's properties.
		name: "patch is against the email",
		src: `{"methodCalls": [["MDN/send", {
			"onSuccessUpdateEmail": {"#r": {"keywrds/$mdnsent": true}}
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

// TestMDNParse covers reading a notification that arrived as a message, which
// is how a client tells the user what became of something they sent.
func TestMDNParse(t *testing.T) {
	parse(t, "ReadReceipt"+Extension, `{"methodCalls": [
	  ["Email/get", {"ids": ["{{emailId}}"], "properties": ["id", "attachments"]}, "c0"],
	  ["MDN/parse", {"blobIds": ["{{blobId}}"]}, "c1"]
	]}`)
}

// TestPushSubscription covers the other form of push, where the server posts to
// a URL the client registers. Neither of its methods takes an accountId: a
// subscription belongs to the credentials that created it rather than to an
// account, which makes these the only methods in the catalogue with no account
// to name.
func TestPushSubscription(t *testing.T) {
	q := parse(t, "RegisterPush"+Extension, `{
	  "methodCalls": [
	    ["PushSubscription/set", {
	      "create": {"device": {
	        "deviceClientId": "{{deviceClientId}}",
	        "url": "{{url}}",
	        "keys": {"p256dh": "{{publicKey}}", "auth": "{{authSecret}}"},
	        "types": ["Email"]
	      }}
	    }, "register"]
	  ],
	  "_returns": "register"
	}`)
	for _, p := range q.Params {
		if p.Name == "accountId" {
			t.Error("the query has an accountId parameter, which these methods do not take")
		}
	}

	if got := parseErr(t, `{"methodCalls": [
	  ["PushSubscription/get", {"accountId": "a1", "ids": null}, "c0"]
	]}`); !strings.Contains(got, `PushSubscription/get has no argument "accountId"`) {
		t.Errorf("error was:\n%s", got)
	}
}

// TestPushSubscriptionChecks covers the mistakes these methods invite, which
// come from their looking like the standard forms without being them.
func TestPushSubscriptionChecks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{{
		name: "no state to compare against",
		src:  `{"methodCalls": [["PushSubscription/set", {"ifInState": "s1"}, "c0"]]}`,
		want: `PushSubscription/set has no argument "ifInState"`,
	}, {
		name: "no changes method",
		src:  `{"methodCalls": [["PushSubscription/changes", {"sinceState": "s"}, "c0"]]}`,
		want: `unknown method "PushSubscription/changes"`,
	}, {
		name: "misspelled property",
		src: `{"methodCalls": [["PushSubscription/set", {
			"create": {"d": {"deviceClientID": "x", "url": "https://example.com/push"}}
		}, "c0"]]}`,
		want: `did you mean "deviceClientId"?`,
	}, {
		name: "keys are an object, not a string",
		src: `{"methodCalls": [["PushSubscription/set", {
			"create": {"d": {"keys": "p256dh"}}
		}, "c0"]]}`,
		want: `expected PushSubscriptionKeys|null, found a string`,
	}, {
		name: "patch into a subscription",
		src: `{"methodCalls": [["PushSubscription/set", {
			"update": {"s1": {"verificationCod": "abc"}}
		}, "c0"]]}`,
		want: `did you mean "verificationCode"?`,
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

// TestHeaderFieldProperties covers the properties naming one header field of a
// message. They are not members of the Email type — a message may carry any
// field at all — but the form asked for decides the type, so they are checked
// and typed rather than waved through.
func TestHeaderFieldProperties(t *testing.T) {
	parse(t, "Headers"+Extension, `{"methodCalls": [
	  ["Email/get", {"ids": ["e1"], "properties": [
	    "id",
	    "header:List-Id",
	    "header:List-Id:asText",
	    "header:From:asAddresses",
	    "header:To:asGroupedAddresses",
	    "header:Message-ID:asMessageIds",
	    "header:Delivery-Date:asDate",
	    "header:List-Post:asURLs",
	    "header:Received:all",
	    "header:Resent-To:asAddresses:all"
	  ]}, "c0"]
	]}`)

	tests := []struct{ src, want string }{{
		`{"methodCalls": [["Email/get", {"properties": ["header:Subject:asWords"]}, "c0"]]}`,
		"asks for the asWords form",
	}, {
		`{"methodCalls": [["Email/get", {"properties": ["header:"]}, "c0"]]}`,
		"names no header field",
	}, {
		`{"methodCalls": [["Email/get", {"properties": ["header:Subject:asText:all:more"]}, "c0"]]}`,
		"more suffixes than the form and :all",
	}}
	for _, tt := range tests {
		if got := parseErr(t, tt.src); !strings.Contains(got, tt.want) {
			t.Errorf("error was:\n%s\nwant it to mention: %s", got, tt.want)
		}
	}
}

// TestBodyProperties covers the argument that narrows the body parts rather
// than the records. It reaches places properties cannot: a part's sub-parts,
// and their sub-parts, all the way down.
func TestBodyProperties(t *testing.T) {
	q := parse(t, "Bodies"+Extension, `{"methodCalls": [
	  ["Email/get", {
	    "ids": ["e1"],
	    "properties": ["id", "bodyStructure", "textBody"],
	    "bodyProperties": ["partId", "type", "size", "subParts"]
	  }, "c0"]
	]}`)
	if got, want := strings.Join(q.Calls[0].NestedProperties, ","), "partId,type,size,subParts"; got != want {
		t.Errorf("bodyProperties = %q, want %q", got, want)
	}

	if got := parseErr(t, `{"methodCalls": [
	  ["Email/get", {"bodyProperties": ["partId", "siz"]}, "c0"]
	]}`); !strings.Contains(got, `EmailBodyPart has no property "siz"`) {
		t.Errorf("error was:\n%s", got)
	}

	// Email/parse narrows body parts the same way.
	parse(t, "Parsed"+Extension, `{"methodCalls": [
	  ["Email/parse", {"blobIds": ["b1"], "bodyProperties": ["partId", "type"]}, "c0"]
	]}`)
}

// TestCreatedIDsCarry covers a query that takes the creation ids of an earlier
// request and reports its own. RFC 8620 has this for proxies, which split one
// request across several servers and need the references to still resolve.
func TestCreatedIDsCarry(t *testing.T) {
	q := parse(t, "CreateAndFile"+Extension, `{
	  "_createdIds": true,
	  "methodCalls": [
	    ["Mailbox/set", {"create": {"box": {"name": "{{name}}", "parentId": null}}}, "make"],
	    ["Email/set", {"update": {"{{emailId}}": {"mailboxIds/#box": true}}}, "file"]
	  ]
	}`)
	if !q.CreatedIDs {
		t.Error("the query does not carry creation ids")
	}

	// The ids belong to the request rather than to any one call, so there is
	// nowhere to put them in a query that returns a single response.
	got := parseErr(t, `{
	  "_createdIds": true,
	  "_returns": "c0",
	  "methodCalls": [["Mailbox/get", {}, "c0"]]
	}`)
	if !strings.Contains(got, "returns every response") {
		t.Errorf("error was:\n%s", got)
	}

	// A query that says nothing about them carries none.
	plain := parse(t, "Plain"+Extension, `{"methodCalls": [["Mailbox/get", {}, "c0"]]}`)
	if plain.CreatedIDs {
		t.Error("a query that did not ask for creation ids carries them")
	}
}
