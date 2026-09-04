package request

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/internal/query"
	"github.com/linyows/jmapc/internal/spec"
)

// parse checks a query the way the generator does, so that a test builds a
// request from the same thing a run does.
func parse(t *testing.T, name, src string) *query.Query {
	t.Helper()
	q, err := query.NewParser(spec.Standard()).Parse(name+query.Extension, []byte(src))
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return q
}

// values reads the parameters of a query as a caller would give them on a
// command line.
func values(t *testing.T, q *query.Query, pairs map[string]string) map[string]Value {
	t.Helper()
	out := make(map[string]Value, len(pairs))
	for _, p := range q.Params {
		text, ok := pairs[p.Name]
		if !ok {
			continue
		}
		v, err := ParseValue(p, text)
		if err != nil {
			t.Fatalf("parameter %s: %v", p.Name, err)
		}
		out[p.Name] = v
	}
	return out
}

// account answers with one account id, whatever capability is asked about.
func account(id jmapc.ID) Accounts {
	return func(string) (jmapc.ID, error) { return id, nil }
}

// build renders a request as the JSON it goes out as.
func build(t *testing.T, q *query.Query, vals map[string]Value, accounts Accounts) string {
	t.Helper()
	req, err := Build(spec.Standard(), q, vals, accounts, nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	return string(out)
}

// TestBuild checks a request carrying every kind of value a query holds: a
// literal subtree, a parameter, a member name built from one, and a back
// reference to an earlier call.
func TestBuild(t *testing.T) {
	q := parse(t, "Chain", `{
	  "methodCalls": [
	    ["Email/query", {"filter": {"inMailbox": "{{mailboxId}}"}, "limit": "{{limit}}",
	                     "sort": [{"property": "receivedAt", "isAscending": false}]}, "search"],
	    ["Email/get", {"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"},
	                   "properties": ["id", "subject"]}, "fetch"],
	    ["Email/set", {"update": {"{{mailboxId}}": {"keywords/$seen": true}}}, "mark"]
	  ]
	}`)
	got := build(t, q, values(t, q, map[string]string{"mailboxId": "mbx1", "limit": "5"}), account("a1"))

	for _, want := range []string{
		// The parameters take the shape their types call for: an id is a
		// string and a limit is a number, though both were typed as text.
		`"inMailbox":"mbx1"`,
		`"limit":5`,
		// A subtree depending on no parameter goes out as it was written.
		`"sort":[{"property":"receivedAt","isAscending":false}]`,
		// A back reference is a constant in the request; the server fills it in.
		`"#ids":{"resultOf":"search","name":"Email/query","path":"/ids"}`,
		// A parameter may name the record to change, rather than a value.
		`"update":{"mbx1":{"keywords/$seen":true}}`,
		// The account id the query left out is filled in from the session.
		`"accountId":"a1"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the request does not contain %s:\n%s", want, got)
		}
	}
}

// TestBuildKeepsTheAccountTheQueryStates checks that a query naming the account
// is left alone, session or no session.
func TestBuildKeepsTheAccountTheQueryStates(t *testing.T) {
	q := parse(t, "Named", `{
	  "methodCalls": [["Mailbox/get", {"accountId": "a2", "ids": null}, "all"]]
	}`)
	accounts := func(string) (jmapc.ID, error) {
		t.Error("the session was asked for an account the query states")
		return "", nil
	}
	if got := build(t, q, nil, accounts); !strings.Contains(got, `"accountId":"a2"`) {
		t.Errorf("the request does not carry the account the query names:\n%s", got)
	}
}

// TestBuildWithoutASession checks that a query leaving the account to the
// session says so, rather than sending a request with no account in it.
func TestBuildWithoutASession(t *testing.T) {
	q := parse(t, "Unnamed", `{"methodCalls": [["Mailbox/get", {"ids": null}, "all"]]}`)
	_, err := Build(spec.Standard(), q, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no session to ask") {
		t.Fatalf("err = %v, want the missing session reported", err)
	}
}

// TestBuildReportsEveryParameterProblemAtOnce checks that a caller who mistyped
// one parameter name does not have to run again to learn about the next.
func TestBuildReportsEveryParameterProblemAtOnce(t *testing.T) {
	q := parse(t, "Two", `{
	  "methodCalls": [["Mailbox/query", {"filter": {"name": "{{name}}"}, "limit": "{{limit}}"}, "q"]]
	}`)
	_, err := Build(spec.Standard(), q, map[string]Value{
		"nmae": {Text: "Work", JSON: json.RawMessage(`"Work"`)},
	}, account("a1"), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"name (String)", "limit (UnsignedInt)", `has no parameter "nmae"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s:\n%v", want, err)
		}
	}
}

// TestBuildCarriesCreatedIDs checks the ids that travel from one request to the
// next, which is what lets a reference to something created earlier resolve.
func TestBuildCarriesCreatedIDs(t *testing.T) {
	q := parse(t, "File", `{
	  "_createdIds": true,
	  "methodCalls": [["Email/set", {"update": {"{{emailId}}": {"mailboxIds/#box": true}}}, "file"]]
	}`)
	vals := values(t, q, map[string]string{"emailId": "m1"})
	req, err := Build(spec.Standard(), q, vals, account("a1"), map[jmapc.ID]jmapc.ID{"box": "mbx9"})
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	if req.CreatedIDs["box"] != "mbx9" {
		t.Errorf("createdIds = %v", req.CreatedIDs)
	}
}

// TestBuildLeavesOutAnOptionalParameter checks the argument a query lets the
// caller leave out: no value for it means the argument is not in the request,
// which is a different request from one sending null.
func TestBuildLeavesOutAnOptionalParameter(t *testing.T) {
	q := parse(t, "GetChanges", `{
	  "methodCalls": [["Email/changes", {"sinceState": "{{sinceState}}", "maxChanges": "{{maxChanges?}}"}, "changes"]]
	}`)

	without := build(t, q, values(t, q, map[string]string{"sinceState": "s1"}), account("a1"))
	if strings.Contains(without, "maxChanges") {
		t.Errorf("the request carries maxChanges though it was not given:\n%s", without)
	}
	if !strings.Contains(without, `"sinceState":"s1"`) {
		t.Errorf("the request lost an argument that was given:\n%s", without)
	}

	with := build(t, q, values(t, q, map[string]string{"sinceState": "s1", "maxChanges": "25"}), account("a1"))
	if !strings.Contains(with, `"maxChanges":25`) {
		t.Errorf("the request does not carry the maxChanges that was given:\n%s", with)
	}
}
