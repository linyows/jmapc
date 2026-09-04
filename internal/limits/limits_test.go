package limits

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// session decodes a session object written the way a server serves one.
func session(t *testing.T, src string) *jmapc.Session {
	t.Helper()
	var s jmapc.Session
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		t.Fatalf("the session is not JSON: %v", err)
	}
	return &s
}

// generous is a server that would accept anything these tests ask of it, so
// that a check reporting something is reporting the one thing a case put
// wrong.
const generous = `{
  "capabilities": {
    "urn:ietf:params:jmap:core": {
      "maxCallsInRequest": 16, "maxObjectsInGet": 500, "maxObjectsInSet": 500,
      "maxSizeRequest": 10000000, "collationAlgorithms": ["i;ascii-casemap", "i;unicode-casemap"]
    },
    "urn:ietf:params:jmap:mail": {},
    "urn:ietf:params:jmap:contacts": {}
  },
  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
  "primaryAccounts": {
    "urn:ietf:params:jmap:core": "a1",
    "urn:ietf:params:jmap:mail": "a1",
    "urn:ietf:params:jmap:contacts": "a1"
  },
  "username": "someone",
  "apiUrl": "https://example.com/api",
  "state": "s1"
}`

// check parses a query and reports what the server would refuse about it.
func check(t *testing.T, sessionSrc, src string) string {
	t.Helper()
	catalogue := spec.Standard()
	q, err := query.NewParser(catalogue).Parse("Q"+query.Extension, []byte(src))
	if err != nil {
		t.Fatalf("checking the query:\n%v", err)
	}
	err = Check(catalogue, session(t, sessionSrc), q)
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestAccepted checks that a query a server would take is reported as nothing
// at all, which is what makes the rest of these worth reading.
func TestAccepted(t *testing.T) {
	got := check(t, generous, `{
	  "methodCalls": [
	    ["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"},
	                     "sort": [{"property": "receivedAt", "collation": "i;ascii-casemap"}]}, "search"],
	    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}}, "fetch"]
	  ]
	}`)
	if got != "" {
		t.Errorf("the server was said to refuse a query it would take:\n%s", got)
	}
}

// TestRefused covers each thing a session says that the specifications do not.
func TestRefused(t *testing.T) {
	cases := []struct{ name, session, query, want string }{
		{
			"a capability the server does not have",
			`{"capabilities": {"urn:ietf:params:jmap:core": {}}, "accounts": {}, "primaryAccounts": {}, "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"accountId": "a1", "ids": null}, "c0"]]}`,
			"the server does not advertise urn:ietf:params:jmap:mail",
		},
		{
			"an account the session cannot fill in",
			`{"capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {}, "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"ids": null}, "c0"]]}`,
			"the query leaves the account to the session",
		},
		{
			"an account the session does not hold",
			generous,
			`{"methodCalls": [["Email/get", {"accountId": "a9", "ids": null}, "c0"]]}`,
			`the session has no account "a9"`,
		},
		{
			"an account that does not do mail",
			`{"capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone", "accountCapabilities": {"urn:ietf:params:jmap:contacts": {}}}},
			  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"}, "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"ids": null}, "c0"]]}`,
			`the account "a1" does not support urn:ietf:params:jmap:mail`,
		},
		{
			"more calls than the server takes",
			`{"capabilities": {"urn:ietf:params:jmap:core": {"maxCallsInRequest": 1}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
			  "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"ids": ["m1"]}, "c0"], ["Mailbox/get", {"ids": null}, "c1"]]}`,
			"the query makes 2 calls, and the server takes 1 in one request",
		},
		{
			"more records than the server returns",
			`{"capabilities": {"urn:ietf:params:jmap:core": {"maxObjectsInGet": 2}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
			  "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"ids": ["m1", "m2", "m3"]}, "c0"]]}`,
			"the call asks for 3 records, and the server returns 2 from one call",
		},
		{
			"more changes than the server makes at once",
			`{"capabilities": {"urn:ietf:params:jmap:core": {"maxObjectsInSet": 1}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
			  "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/set", {"update": {"m1": {"keywords/$seen": true}, "m2": {"keywords/$seen": true}},
			                                 "destroy": ["m3"]}, "c0"]]}`,
			"the call changes 3 records, and the server changes 1 in one call",
		},
		{
			"a collation the server does not compare with",
			generous,
			`{"methodCalls": [["Email/query", {"sort": [{"property": "subject", "collation": "i;octet"}]}, "c0"]]}`,
			`the server does not offer the collation "i;octet"`,
		},
		{
			"a request larger than the server accepts",
			`{"capabilities": {"urn:ietf:params:jmap:core": {"maxSizeRequest": 64}, "urn:ietf:params:jmap:mail": {}},
			  "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
			  "apiUrl": "u", "state": "s"}`,
			`{"methodCalls": [["Email/get", {"ids": ["m1"], "properties": ["id", "subject", "from", "receivedAt"]}, "c0"]]}`,
			"and the server accepts 64",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := check(t, c.session, c.query)
			if !strings.Contains(got, c.want) {
				t.Errorf("got  %s\nwant it to mention %q", got, c.want)
			}
		})
	}
}

// TestUnknowableIsNotRefused checks what a query leaves to its caller. How many
// records they pass, and which account they name, is not known here, and
// guessing at it would report a query that is fine.
func TestUnknowableIsNotRefused(t *testing.T) {
	small := `{"capabilities": {"urn:ietf:params:jmap:core": {"maxObjectsInGet": 1, "maxObjectsInSet": 1},
	            "urn:ietf:params:jmap:mail": {}},
	           "accounts": {"a1": {"name": "someone"}}, "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
	           "apiUrl": "u", "state": "s"}`
	got := check(t, small, `{
	  "methodCalls": [["Email/get", {"accountId": "{{accountId}}", "ids": "{{ids}}"}, "c0"]]
	}`)
	if got != "" {
		t.Errorf("a query the server may well accept was refused:\n%s", got)
	}
}

// TestServerWithoutCore checks the one session that cannot be checked against:
// every server has to advertise the core capability, and one that does not is
// not describing itself.
func TestServerWithoutCore(t *testing.T) {
	got := check(t, `{"capabilities": {}, "accounts": {}, "primaryAccounts": {}, "apiUrl": "u", "state": "s"}`,
		`{"methodCalls": [["Core/echo", {}, "c0"]]}`)
	if !strings.Contains(got, "urn:ietf:params:jmap:core") {
		t.Errorf("got %s, want the missing core capability reported", got)
	}
}
