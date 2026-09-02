// Package example exercises the generated client against a stub JMAP server.
// It is the end-to-end check that a query written by hand turns into the
// request the specification says it should, and that the response decodes into
// the types the query asked for.
package example

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/example/jmapq"
)

// accountID is the account the stub server reports as primary for mail.
const accountID = jmapc.ID("acct1")

// stub is a JMAP server that serves one session and answers one request with a
// canned response, recording what it was asked.
type stub struct {
	t *testing.T
	// response is the JSON body returned to an API request.
	response string
	// got holds the decoded request the client sent.
	got map[string]any
	// raw holds the request body as it arrived.
	raw []byte
}

func (s *stub) start() *httptest.Server {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			s.t.Errorf("Authorization = %q, want %q", got, "Bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"capabilities": map[string]any{
				jmapc.CapabilityCore:       map[string]any{"maxCallsInRequest": 16},
				jmapc.CapabilityMail:       map[string]any{},
				jmapc.CapabilitySubmission: map[string]any{},
			},
			"accounts": map[string]any{
				string(accountID): map[string]any{"name": "someone@example.com", "isPersonal": true},
			},
			"primaryAccounts": map[string]any{
				jmapc.CapabilityMail:       string(accountID),
				jmapc.CapabilitySubmission: string(accountID),
			},
			"username": "someone@example.com",
			"apiUrl":   srv.URL + "/jmap/api/",
			"state":    "session-1",
		})
	})
	mux.HandleFunc("/jmap/api/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Fatalf("reading request body: %v", err)
		}
		s.raw = body
		if err := json.Unmarshal(body, &s.got); err != nil {
			s.t.Fatalf("request body is not JSON: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(s.response))
	})
	s.t.Cleanup(srv.Close)
	return srv
}

// client returns a client pointed at the stub.
func (s *stub) client() *jmapc.Client {
	srv := s.start()
	return jmapc.New(srv.URL+"/.well-known/jmap", jmapc.WithBearerToken("token"))
}

// call returns the method call the request made under the given call id.
func (s *stub) call(t *testing.T, callID string) (name string, args map[string]any) {
	t.Helper()
	calls, ok := s.got["methodCalls"].([]any)
	if !ok {
		t.Fatalf("request has no methodCalls:\n%s", s.raw)
	}
	for _, c := range calls {
		parts, ok := c.([]any)
		if !ok || len(parts) != 3 {
			t.Fatalf("malformed method call %v", c)
		}
		if parts[2] == callID {
			return parts[0].(string), parts[1].(map[string]any)
		}
	}
	t.Fatalf("request has no call %q:\n%s", callID, s.raw)
	return "", nil
}

func TestListInboxEmails(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Email/query", {"accountId": "acct1", "queryState": "q1", "canCalculateChanges": true,
	                     "position": 0, "ids": ["e1", "e2"]}, "search"],
	    ["Email/get", {"accountId": "acct1", "state": "s1", "notFound": [], "list": [
	      {"id": "e1", "threadId": "t1", "subject": "Hello", "receivedAt": "2024-05-01T09:00:00Z",
	       "preview": "Hi there", "hasAttachment": false,
	       "from": [{"name": "Ann", "email": "ann@example.com"}]},
	      {"id": "e2", "threadId": "t2", "subject": null, "receivedAt": "2024-04-30T08:00:00Z",
	       "preview": "", "hasAttachment": true, "from": null}
	    ]}, "fetch"]
	  ]
	}`}

	got, err := jmapq.ListInboxEmails(context.Background(), s.client(), jmapq.ListInboxEmailsParams{
		MailboxID: "mbx1",
		Limit:     25,
	})
	if err != nil {
		t.Fatalf("ListInboxEmails: %v", err)
	}

	// The request should carry both calls, with the account id filled in from
	// the session and the ids passed by back reference rather than by value.
	name, args := s.call(t, "search")
	if name != "Email/query" {
		t.Errorf("first call is %q, want Email/query", name)
	}
	if args["accountId"] != string(accountID) {
		t.Errorf("accountId = %v, want %v", args["accountId"], accountID)
	}
	if args["limit"] != float64(25) {
		t.Errorf("limit = %v, want 25", args["limit"])
	}
	filter, ok := args["filter"].(map[string]any)
	if !ok || filter["inMailbox"] != "mbx1" {
		t.Errorf("filter = %v, want inMailbox mbx1", args["filter"])
	}

	name, args = s.call(t, "fetch")
	if name != "Email/get" {
		t.Errorf("second call is %q, want Email/get", name)
	}
	if _, byValue := args["ids"]; byValue {
		t.Error("the ids were sent by value; they should come from the back reference")
	}
	ref, ok := args["#ids"].(map[string]any)
	if !ok {
		t.Fatalf("#ids = %v, want a result reference", args["#ids"])
	}
	if ref["resultOf"] != "search" || ref["name"] != "Email/query" || ref["path"] != "/ids" {
		t.Errorf("#ids = %v, want a reference to /ids of the search call", ref)
	}

	// The response should decode into the narrowed record type.
	if len(got.List) != 2 {
		t.Fatalf("got %d emails, want 2", len(got.List))
	}
	first := got.List[0]
	if first.ID != "e1" || first.Subject == nil || *first.Subject != "Hello" {
		t.Errorf("first email = %+v, want e1 with subject Hello", first)
	}
	if len(first.From) != 1 || first.From[0].Email != "ann@example.com" {
		t.Errorf("first email from = %v", first.From)
	}
	if !first.ReceivedAt.Time.Equal(mustTime(t, "2024-05-01T09:00:00Z")) {
		t.Errorf("receivedAt = %v", first.ReceivedAt)
	}
	if got.List[1].Subject != nil {
		t.Errorf("second subject = %v, want nil for a null subject", *got.List[1].Subject)
	}
}

func TestMarkEmailRead(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Email/set", {"accountId": "acct1", "oldState": "s1", "newState": "s2",
	                   "updated": {"e1": null}}, "mark"]
	  ]
	}`}

	got, err := jmapq.MarkEmailRead(context.Background(), s.client(), jmapq.MarkEmailReadParams{EmailID: "e1"})
	if err != nil {
		t.Fatalf("MarkEmailRead: %v", err)
	}

	_, args := s.call(t, "mark")
	update, ok := args["update"].(map[string]any)
	if !ok {
		t.Fatalf("update = %v, want an object keyed by email id", args["update"])
	}
	patch, ok := update["e1"].(map[string]any)
	if !ok {
		t.Fatalf("update has no entry for e1: %v", update)
	}
	if patch["keywords/$seen"] != true {
		t.Errorf("patch = %v, want keywords/$seen set to true", patch)
	}
	if _, updated := got.Updated["e1"]; !updated {
		t.Errorf("Updated = %v, want an entry for e1", got.Updated)
	}
}

func TestSyncEmails(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Email/changes", {"accountId": "acct1", "oldState": "s1", "newState": "s2",
	                       "hasMoreChanges": false, "created": ["e3"], "updated": ["e1"], "destroyed": []}, "changes"],
	    ["Email/get", {"accountId": "acct1", "state": "s2", "notFound": [], "list": [
	      {"id": "e3", "threadId": "t3", "subject": "New", "receivedAt": "2024-05-02T09:00:00Z",
	       "mailboxIds": {"mbx1": true}, "keywords": {}}
	    ]}, "created"],
	    ["Email/get", {"accountId": "acct1", "state": "s2", "notFound": [], "list": [
	      {"id": "e1", "mailboxIds": {"mbx2": true}, "keywords": {"$seen": true}}
	    ]}, "updated"]
	  ]
	}`}

	got, err := jmapq.SyncEmails(context.Background(), s.client(), jmapq.SyncEmailsParams{SinceState: "s1"})
	if err != nil {
		t.Fatalf("SyncEmails: %v", err)
	}

	// Two /get calls read different paths out of the same /changes result.
	_, created := s.call(t, "created")
	if ref := created["#ids"].(map[string]any); ref["path"] != "/created" {
		t.Errorf("created call reads %v, want /created", ref["path"])
	}
	_, updated := s.call(t, "updated")
	if ref := updated["#ids"].(map[string]any); ref["path"] != "/updated" {
		t.Errorf("updated call reads %v, want /updated", ref["path"])
	}

	if got.EmailChanges.NewState != "s2" {
		t.Errorf("newState = %q, want s2", got.EmailChanges.NewState)
	}
	if len(got.EmailGet.List) != 1 || got.EmailGet.List[0].ID != "e3" {
		t.Errorf("created emails = %+v, want e3", got.EmailGet.List)
	}
	if seen := got.EmailGet2.List[0].Keywords["$seen"]; !seen {
		t.Errorf("updated email keywords = %v, want $seen", got.EmailGet2.List[0].Keywords)
	}
}

func TestCreateMailbox(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Mailbox/set", {"accountId": "acct1", "newState": "m2",
	                     "created": {"new": {"id": "mbx9", "sortOrder": 0, "totalEmails": 0,
	                                          "unreadEmails": 0, "totalThreads": 0, "unreadThreads": 0}}}, "create"]
	  ]
	}`}

	got, err := jmapq.CreateMailbox(context.Background(), s.client(), jmapq.CreateMailboxParams{Name: "Receipts"})
	if err != nil {
		t.Fatalf("CreateMailbox: %v", err)
	}

	_, args := s.call(t, "create")
	create := args["create"].(map[string]any)
	mailbox := create["new"].(map[string]any)
	if mailbox["name"] != "Receipts" {
		t.Errorf("name = %v, want Receipts", mailbox["name"])
	}
	if parent, present := mailbox["parentId"]; !present || parent != nil {
		t.Errorf("parentId = %v (present %v), want an explicit null", parent, present)
	}
	if mailbox["isSubscribed"] != true {
		t.Errorf("isSubscribed = %v, want true", mailbox["isSubscribed"])
	}
	if got.Created["new"].ID != "mbx9" {
		t.Errorf("created mailbox = %+v, want id mbx9", got.Created["new"])
	}
}

// TestMethodError checks that a method-level error reaches the caller as one,
// naming the method that failed rather than the literal "error" the wire format
// carries.
func TestMethodError(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["error", {"type": "unsupportedFilter", "description": "cannot filter on that"}, "search"],
	    ["Email/get", {"accountId": "acct1", "state": "s1", "list": [], "notFound": []}, "fetch"]
	  ]
	}`}

	_, err := jmapq.ListInboxEmails(context.Background(), s.client(), jmapq.ListInboxEmailsParams{MailboxID: "mbx1"})
	if err == nil {
		t.Fatal("expected an error")
	}
	var errs jmapc.MethodErrors
	if !asMethodErrors(err, &errs) {
		t.Fatalf("error is %T (%v), want jmapc.MethodErrors", err, err)
	}
	if len(errs) != 1 {
		t.Fatalf("got %d method errors, want 1", len(errs))
	}
	if errs[0].Type != jmapc.ErrUnsupportedFilter {
		t.Errorf("error type = %q, want %q", errs[0].Type, jmapc.ErrUnsupportedFilter)
	}
	if errs[0].MethodName != "Email/query" {
		t.Errorf("error names method %q, want Email/query", errs[0].MethodName)
	}
	if errs[0].CallID != "search" {
		t.Errorf("error names call %q, want search", errs[0].CallID)
	}
}

// TestPreflightRejectsUnknownCapability checks that a request the session shows
// the server cannot serve fails before it is sent.
func TestPreflightRejectsUnknownCapability(t *testing.T) {
	s := &stub{t: t, response: `{"sessionState": "session-1", "methodResponses": []}`}
	srv := s.start()
	c := jmapc.New(srv.URL+"/.well-known/jmap", jmapc.WithBearerToken("token"))

	_, err := c.Do(context.Background(), &jmapc.Request{
		Using: []string{"urn:ietf:params:jmap:calendars"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if s.raw != nil {
		t.Errorf("the request was sent anyway:\n%s", s.raw)
	}
	var reqErr *jmapc.RequestError
	if !asRequestError(err, &reqErr) {
		t.Fatalf("error is %T (%v), want *jmapc.RequestError", err, err)
	}
	if reqErr.Type != jmapc.ErrTypeUnknownCapability {
		t.Errorf("error type = %q, want %q", reqErr.Type, jmapc.ErrTypeUnknownCapability)
	}
}

// TestSendEmail covers the request JMAP exists for: writing a message,
// submitting it, and filing it under Sent all in one go, with each step
// referring to what the one before it created.
func TestSendEmail(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Email/set", {"accountId": "acct1", "newState": "s2",
	                   "created": {"draft": {"id": "e9", "blobId": "b9", "threadId": "t9", "size": 412}}}, "write"],
	    ["EmailSubmission/set", {"accountId": "acct1", "newState": "sub2",
	                             "created": {"send": {"id": "sub9", "threadId": "t9",
	                                                  "sendAt": "2024-05-01T09:00:00Z",
	                                                  "undoStatus": "final"}}}, "send"]
	  ]
	}`}

	got, err := jmapq.SendEmail(context.Background(), s.client(), jmapq.SendEmailParams{
		DraftsMailboxID: "drafts",
		SentMailboxID:   "sent",
		IdentityID:      "id1",
		FromAddress:     "me@example.com",
		ToAddress:       "you@example.com",
		Subject:         "Lunch",
		Body:            "Thursday?",
	})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	// The draft names the mailbox the caller chose, as a key rather than a
	// value.
	_, write := s.call(t, "write")
	draft := write["create"].(map[string]any)["draft"].(map[string]any)
	mailboxes := draft["mailboxIds"].(map[string]any)
	if mailboxes["drafts"] != true {
		t.Errorf("mailboxIds = %v, want the drafts mailbox", mailboxes)
	}
	if draft["subject"] != "Lunch" {
		t.Errorf("subject = %v, want Lunch", draft["subject"])
	}
	body := draft["bodyValues"].(map[string]any)["text"].(map[string]any)
	if body["value"] != "Thursday?" {
		t.Errorf("body = %v, want Thursday?", body)
	}

	// The submission refers to the email by its creation id, which only works
	// because both calls are in the same request.
	_, send := s.call(t, "send")
	submission := send["create"].(map[string]any)["send"].(map[string]any)
	if submission["emailId"] != "#draft" {
		t.Errorf("emailId = %v, want the creation id #draft", submission["emailId"])
	}

	// The patch applied on success moves the message between mailboxes, with
	// both ids substituted into the JSON pointers.
	patch := send["onSuccessUpdateEmail"].(map[string]any)["#send"].(map[string]any)
	if v, present := patch["mailboxIds/drafts"]; !present || v != nil {
		t.Errorf("patch removes drafts as %v (present %v), want an explicit null", v, present)
	}
	if patch["mailboxIds/sent"] != true {
		t.Errorf("patch = %v, want the sent mailbox added", patch)
	}
	if v, present := patch["keywords/$draft"]; !present || v != nil {
		t.Errorf("patch removes $draft as %v (present %v), want an explicit null", v, present)
	}

	if got.Created["send"].UndoStatus != "final" {
		t.Errorf("submission = %+v, want undoStatus final", got.Created["send"])
	}
}
