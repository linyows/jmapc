package jmapc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// searchRequest is a request carrying a search phrase, a back reference and a
// property list, as a list of messages sends.
const searchRequest = `{
  "using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
  "methodCalls": [
    ["Email/query", {"accountId": "a1", "filter": {"text": "quarterly invoice", "inMailbox": "mb1"}, "sort": [{"property": "receivedAt", "isAscending": false}], "limit": 10}, "search"],
    ["Email/get", {"accountId": "a1", "#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids"}, "properties": ["id", "subject", "from"]}, "fetch"]
  ]
}`

// searchResponse is what it answers with: the subjects and addresses of two
// messages, and the ids and states around them.
const searchResponse = `{
  "methodResponses": [
    ["Email/query", {"accountId": "a1", "queryState": "q1", "ids": ["e1", "e2"], "position": 0}, "search"],
    ["Email/get", {"accountId": "a1", "state": "s9", "list": [
      {"id": "e1", "threadId": "t1", "subject": "Invoice for March", "from": [{"name": "Alice", "email": "alice@example.com"}], "mailboxIds": {"mb1": true}},
      {"id": "e2", "threadId": "t2", "subject": "Re: invoice", "from": [{"name": "Bob", "email": "bob@example.com"}], "mailboxIds": {"mb1": true}}
    ], "notFound": ["e3"]}, "fetch"],
    ["error", {"type": "invalidArguments", "description": "no such mailbox: Private"}, "other"]
  ],
  "sessionState": "sess1"
}`

func TestRedactContentKeepsTheShapeAndWithholdsTheContent(t *testing.T) {
	redact := RedactContent()
	for name, tt := range map[string]struct {
		body     string
		kept     []string
		withheld []string
	}{
		"request": {
			body: searchRequest,
			kept: []string{
				`"Email/query"`, `"search"`, `"fetch"`, `"urn:ietf:params:jmap:mail"`,
				`"accountId":"a1"`, `"inMailbox":"mb1"`, `"property":"receivedAt"`,
				`"isAscending":false`, `"limit":10`, `"properties":["id","subject","from"]`,
				`"#ids":{"name":"Email/query","path":"/ids","resultOf":"search"}`,
			},
			withheld: []string{"quarterly invoice"},
		},
		"response": {
			body: searchResponse,
			kept: []string{
				`"queryState":"q1"`, `"ids":["e1","e2"]`, `"state":"s9"`, `"id":"e1"`,
				`"threadId":"t1"`, `"mb1":true`, `"notFound":["e3"]`, `"sessionState":"sess1"`,
				`"type":"invalidArguments"`, `"position":0`,
			},
			withheld: []string{"Invoice for March", "Re: invoice", "Alice", "alice@example.com", "bob@example.com", "Private"},
		},
	} {
		got := string(redact(json.RawMessage(tt.body)))
		for _, want := range tt.kept {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the redacted body has no %s:\n%s", name, want, got)
			}
		}
		for _, secret := range tt.withheld {
			if strings.Contains(got, secret) {
				t.Errorf("%s: the redacted body still says %q:\n%s", name, secret, got)
			}
		}
		if !strings.Contains(got, Redacted) {
			t.Errorf("%s: nothing was withheld:\n%s", name, got)
		}
	}
}

// TestRedactContentKeepsTheKeysItIsGiven checks that a caller can name a key
// whose values it wants to see, as a program that logs which mailbox a message
// was filed in by name would.
func TestRedactContentKeepsTheKeysItIsGiven(t *testing.T) {
	got := string(RedactContent("subject")(json.RawMessage(searchResponse)))
	if !strings.Contains(got, "Invoice for March") {
		t.Errorf("the subject was withheld though it was asked for:\n%s", got)
	}
	if strings.Contains(got, "alice@example.com") {
		t.Errorf("an address was kept though only the subject was asked for:\n%s", got)
	}
}

func TestRedactContentWithholdsWhatIsNotJSON(t *testing.T) {
	for _, body := range []string{
		`subject: Invoice for March`,
		// One JSON value and something after it is not JSON either, and the
		// value before it is not handed on.
		`{"id":"e1"} {"subject":"Invoice for March"}`,
		`{"id":"e1"} trailing`,
	} {
		if got := string(RedactContent()(json.RawMessage(body))); got != `"`+Redacted+`"` {
			t.Errorf("%s came out as %s, want it withheld whole", body, got)
		}
	}
	if got := string(RedactContent()(json.RawMessage("{\"id\":\"e1\"}\n  "))); got != `{"id":"e1"}` {
		t.Errorf("a body with trailing white space came out as %s", got)
	}
}

// TestRedactContentKeepsOnlyWhatABackReferenceIs checks that a "#" key keeps
// the call id, method name and path of a back reference, and withholds
// anything else written under it, as it would anywhere.
func TestRedactContentKeepsOnlyWhatABackReferenceIs(t *testing.T) {
	got := string(RedactContent()(json.RawMessage(`{"#ids": {"resultOf": "search", "name": "Email/query", "path": "/ids", "note": "quarterly invoice"}}`)))
	for _, want := range []string{`"resultOf":"search"`, `"name":"Email/query"`, `"path":"/ids"`, `"note":"` + Redacted + `"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the back reference has no %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "quarterly invoice") {
		t.Errorf("a member a back reference does not define was kept:\n%s", got)
	}
}

func TestKeepBodiesKeepsEverything(t *testing.T) {
	if got := string(KeepBodies(json.RawMessage(searchResponse))); got != searchResponse {
		t.Errorf("KeepBodies changed the body:\n%s", got)
	}
}

// TestObserverSeesTheBodiesOnlyWhenAskedTo checks that bodies reach the
// observer through Redact, and not at all without it.
func TestObserverSeesTheBodiesOnlyWhenAskedTo(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(searchRequest), &req); err != nil {
		t.Fatalf("decoding the request: %v", err)
	}
	for _, redact := range []func(json.RawMessage) json.RawMessage{nil, RedactContent()} {
		ts := newTestServer(t)
		ts.sessionHandler = `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
		  "accounts": {"a1": {"name": "someone", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "a1"},
		  "username": "someone", "apiUrl": "` + ts.URL + `/api", "state": "sess1"
		}`
		ts.apiHandler = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, searchResponse) }
		rec := &recorder{}
		o := rec.observer()
		o.Redact = redact
		c := ts.client(WithObserver(o))
		if _, err := c.Do(context.Background(), &req); err == nil {
			t.Fatal("Do returned no error for the response's method error")
		}
		sent, answered := rec.requests[0].Body, rec.responses[0].Body
		if redact == nil {
			if sent != nil || answered != nil {
				t.Errorf("bodies reached an observer without Redact: %s, %s", sent, answered)
			}
			continue
		}
		if !strings.Contains(string(sent), `"Email/query"`) || strings.Contains(string(sent), "quarterly invoice") {
			t.Errorf("the request body is not the redacted request:\n%s", sent)
		}
		if !strings.Contains(string(answered), `"id":"e1"`) || strings.Contains(string(answered), "Invoice for March") {
			t.Errorf("the response body is not the redacted response:\n%s", answered)
		}
	}
}

func TestSlogObserverWritesTheBodies(t *testing.T) {
	ts := newTestServer(t)
	var buf bytes.Buffer
	o := SlogObserver(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	o.Redact = RedactContent()
	c := ts.client(WithObserver(o))
	if _, err := c.Do(context.Background(), &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: "Core/echo", CallID: "e", Args: map[string]any{"note": "private"}}},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	log := buf.String()
	if !strings.Contains(log, `"request_body"`) || !strings.Contains(log, `"response_body"`) {
		t.Errorf("the log has no bodies:\n%s", log)
	}
	if strings.Contains(log, "private") {
		t.Errorf("the log holds what Redact withholds:\n%s", log)
	}
}

// TestObserverSeesASplitGetAsTheCallerDoes checks the bodies of a /get sent in
// several parts: the request as the caller made it, with every id, and the
// response as the caller received it, with the records of every part.
func TestObserverSeesASplitGetAsTheCallerDoes(t *testing.T) {
	ts := newSplitServer(t, 2, 16)
	rec := &recorder{}
	o := rec.observer()
	o.Redact = KeepBodies
	c := ts.client(WithSplitGets(), WithObserver(o))
	if _, err := c.Do(context.Background(), get("m1", "m2", "m3", "m4", "m5")); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(ts.sent()) != 3 {
		t.Fatalf("the server was asked %d times, want the /get split in three", len(ts.sent()))
	}
	if len(rec.requests) != 1 || len(rec.responses) != 1 {
		t.Fatalf("the observer saw %d requests and %d responses, want one of each", len(rec.requests), len(rec.responses))
	}
	sent, answered := string(rec.requests[0].Body), string(rec.responses[0].Body)
	if !strings.Contains(sent, `"ids":["m1","m2","m3","m4","m5"]`) {
		t.Errorf("the request body is not the request the caller made:\n%s", sent)
	}
	for _, id := range []string{"m1", "m3", "m5"} {
		if !strings.Contains(answered, `{"id":"`+id+`"}`) {
			t.Errorf("the response body has no record %s from its part:\n%s", id, answered)
		}
	}
}
