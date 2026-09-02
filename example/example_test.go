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
	"time"

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
				jmapc.CapabilityContacts:   map[string]any{},
				jmapc.CapabilityCalendars:  map[string]any{},
				jmapc.CapabilityPrincipals: map[string]any{},
			},
			"accounts": map[string]any{
				string(accountID): map[string]any{"name": "someone@example.com", "isPersonal": true},
			},
			"primaryAccounts": map[string]any{
				jmapc.CapabilityMail:       string(accountID),
				jmapc.CapabilitySubmission: string(accountID),
				jmapc.CapabilityContacts:   string(accountID),
				jmapc.CapabilityCalendars:  string(accountID),
				jmapc.CapabilityPrincipals: string(accountID),
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
		Using: []string{"urn:ietf:params:jmap:sieve"},
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

// TestSearchContacts covers a query against JMAP for Contacts, where a card is
// a JSContact object rather than anything JMAP defines itself, and where one
// request answers two unrelated questions at once.
func TestSearchContacts(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["AddressBook/get", {"accountId": "acct1", "state": "b1", "notFound": [], "list": [
	      {"id": "book1", "name": "Personal", "isDefault": true},
	      {"id": "book2", "name": "Work", "isDefault": false}
	    ]}, "books"],
	    ["ContactCard/query", {"accountId": "acct1", "queryState": "q1",
	                           "canCalculateChanges": true, "position": 0, "ids": ["card1"]}, "search"],
	    ["ContactCard/get", {"accountId": "acct1", "state": "c1", "notFound": [], "list": [
	      {"id": "card1", "uid": "urn:uuid:1234",
	       "name": {"components": [
	         {"kind": "given", "value": "Ada"},
	         {"kind": "surname", "value": "Lovelace"}
	       ], "isOrdered": true},
	       "emails": {"work": {"address": "ada@example.com", "pref": 1}},
	       "phones": {"mobile": {"number": "tel:+1-555-0100"}},
	       "organizations": {"employer": {"name": "Analytical Engines"}}}
	    ]}, "fetch"]
	  ]
	}`}

	got, err := jmapq.SearchContacts(context.Background(), s.client(), jmapq.SearchContactsParams{
		AddressBookID: "book2",
		Phrase:        "lovelace",
		Limit:         20,
	})
	if err != nil {
		t.Fatalf("SearchContacts: %v", err)
	}

	// The sort names properties the specification allows sorting cards by.
	_, search := s.call(t, "search")
	sort := search["sort"].([]any)
	if len(sort) != 2 {
		t.Fatalf("sort has %d terms, want 2", len(sort))
	}
	if p := sort[0].(map[string]any)["property"]; p != "name/surname" {
		t.Errorf("first sort term is %v, want name/surname", p)
	}

	// Three calls, three results, all decoded.
	if len(got.AddressBookGet.List) != 2 {
		t.Fatalf("got %d address books, want 2", len(got.AddressBookGet.List))
	}
	if got.AddressBookGet.List[0].Name != "Personal" || !got.AddressBookGet.List[0].IsDefault {
		t.Errorf("first address book = %+v", got.AddressBookGet.List[0])
	}
	if len(got.ContactCardQuery.IDs) != 1 {
		t.Fatalf("got %d matching cards, want 1", len(got.ContactCardQuery.IDs))
	}

	card := got.ContactCardGet.List[0]
	if card.UID != "urn:uuid:1234" {
		t.Errorf("uid = %q", card.UID)
	}
	// A JSContact name is a list of parts, not a string.
	if len(card.Name.Components) != 2 {
		t.Fatalf("name has %d components, want 2", len(card.Name.Components))
	}
	if card.Name.Components[1].Kind != "surname" || card.Name.Components[1].Value != "Lovelace" {
		t.Errorf("second name component = %+v", card.Name.Components[1])
	}
	if addr := card.Emails["work"].Address; addr != "ada@example.com" {
		t.Errorf("work email = %q", addr)
	}
	if org := card.Organizations["employer"].Name; org != "Analytical Engines" {
		t.Errorf("organization = %q", org)
	}
}

// TestCreateContact checks that the nested shape of a JSContact card survives
// the trip through the generated code.
func TestCreateContact(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["ContactCard/set", {"accountId": "acct1", "newState": "c2",
	                         "created": {"card": {"id": "card9"}}}, "create"]
	  ]
	}`}

	got, err := jmapq.CreateContact(context.Background(), s.client(), jmapq.CreateContactParams{
		AddressBookID: "book1",
		UID:           "urn:uuid:5678",
		GivenName:     "Grace",
		Surname:       "Hopper",
		EmailAddress:  "grace@example.com",
		PhoneNumber:   "tel:+1-555-0199",
		Organization:  "Navy",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	_, args := s.call(t, "create")
	card := args["create"].(map[string]any)["card"].(map[string]any)
	if card["@type"] != "Card" || card["version"] != "1.0" {
		t.Errorf("card = %v, want @type Card and version 1.0", card)
	}
	if books := card["addressBookIds"].(map[string]any); books["book1"] != true {
		t.Errorf("addressBookIds = %v, want book1", books)
	}
	components := card["name"].(map[string]any)["components"].([]any)
	if len(components) != 2 {
		t.Fatalf("name has %d components, want 2", len(components))
	}
	if c := components[0].(map[string]any); c["kind"] != "given" || c["value"] != "Grace" {
		t.Errorf("first component = %v", c)
	}
	email := card["emails"].(map[string]any)["work"].(map[string]any)
	if email["address"] != "grace@example.com" || email["pref"] != float64(1) {
		t.Errorf("email = %v", email)
	}
	phone := card["phones"].(map[string]any)["mobile"].(map[string]any)
	if features := phone["features"].(map[string]any); features["mobile"] != true {
		t.Errorf("phone features = %v", features)
	}
	if got.Created["card"].ID != "card9" {
		t.Errorf("created card = %+v, want id card9", got.Created["card"])
	}
}

// TestUpdateContactEmail checks a patch that reaches into a card, where the
// pointer names an entry the caller chooses.
func TestUpdateContactEmail(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["ContactCard/set", {"accountId": "acct1", "oldState": "c1", "newState": "c2",
	                         "updated": {"card1": null}}, "update"]
	  ]
	}`}

	got, err := jmapq.UpdateContactEmail(context.Background(), s.client(), jmapq.UpdateContactEmailParams{
		CardID:   "card1",
		EmailKey: "work",
		Address:  "ada@example.org",
	})
	if err != nil {
		t.Fatalf("UpdateContactEmail: %v", err)
	}

	_, args := s.call(t, "update")
	patch := args["update"].(map[string]any)["card1"].(map[string]any)
	if patch["emails/work/address"] != "ada@example.org" {
		t.Errorf("patch = %v, want emails/work/address set", patch)
	}
	if patch["emails/work/pref"] != float64(1) {
		t.Errorf("patch = %v, want emails/work/pref set to 1", patch)
	}
	// The rest of the card is untouched: a patch sends only what changed.
	if len(patch) != 2 {
		t.Errorf("patch holds %d members, want 2", len(patch))
	}
	if _, updated := got.Updated["card1"]; !updated {
		t.Errorf("Updated = %v, want an entry for card1", got.Updated)
	}
}

// TestAgenda covers a calendar query that expands a recurring event into its
// occurrences, which is what a day view asks for.
func TestAgenda(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["CalendarEvent/query", {"accountId": "acct1", "queryState": "q1",
	                             "canCalculateChanges": false, "position": 0,
	                             "ids": ["ev1", "ev2"]}, "search"],
	    ["CalendarEvent/get", {"accountId": "acct1", "state": "e1", "notFound": [], "list": [
	      {"id": "ev1", "baseEventId": "series1", "title": "Standup",
	       "start": "2024-05-06T09:00:00", "duration": "PT15M",
	       "timeZone": "Europe/London", "status": "confirmed",
	       "participants": {"me": {"email": "me@example.com", "roles": {"owner": true}}}},
	      {"id": "ev2", "baseEventId": null, "title": "Review",
	       "start": "2024-05-06T14:00:00", "duration": "PT1H",
	       "timeZone": "Europe/London", "status": "tentative"}
	    ]}, "fetch"]
	  ]
	}`}

	got, err := jmapq.Agenda(context.Background(), s.client(), jmapq.AgendaParams{
		CalendarID: "cal1",
		From:       "2024-05-06T00:00:00",
		Until:      "2024-05-07T00:00:00",
		TimeZone:   "Europe/London",
	})
	if err != nil {
		t.Fatalf("Agenda: %v", err)
	}

	_, search := s.call(t, "search")
	if search["expandRecurrences"] != true {
		t.Errorf("expandRecurrences = %v, want true", search["expandRecurrences"])
	}
	if search["timeZone"] != "Europe/London" {
		t.Errorf("timeZone = %v", search["timeZone"])
	}
	_, fetch := s.call(t, "fetch")
	if fetch["reduceParticipants"] != true {
		t.Errorf("reduceParticipants = %v, want true", fetch["reduceParticipants"])
	}

	if len(got.List) != 2 {
		t.Fatalf("got %d events, want 2", len(got.List))
	}
	first := got.List[0]
	if first.Title != "Standup" {
		t.Errorf("title = %q", first.Title)
	}
	// A JSCalendar time is local, and the zone is a property of its own.
	if first.Start != "2024-05-06T09:00:00" {
		t.Errorf("start = %q", first.Start)
	}
	if !first.Start.Valid() {
		t.Errorf("start %q does not have the form of a LocalDateTime", first.Start)
	}
	if first.TimeZone == nil || *first.TimeZone != "Europe/London" {
		t.Errorf("timeZone = %v", first.TimeZone)
	}
	d, err := first.Duration.ToTimeDuration()
	if err != nil {
		t.Fatalf("parsing the duration: %v", err)
	}
	if d != 15*time.Minute {
		t.Errorf("duration = %v, want 15m", d)
	}
	// An occurrence of a recurring event says which series it came from.
	if first.BaseEventID == nil || *first.BaseEventID != "series1" {
		t.Errorf("baseEventId = %v, want series1", first.BaseEventID)
	}
	if got.List[1].BaseEventID != nil {
		t.Errorf("a one-off event has baseEventId %v, want null", *got.List[1].BaseEventID)
	}
}

// TestCreateEvent checks that the shape of a JSCalendar event survives the trip
// out, including the negative offset that puts an alert before the event.
func TestCreateEvent(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["CalendarEvent/set", {"accountId": "acct1", "newState": "e2",
	                           "created": {"meeting": {"id": "ev9", "isOrigin": true}}}, "create"]
	  ]
	}`}

	got, err := jmapq.CreateEvent(context.Background(), s.client(), jmapq.CreateEventParams{
		CalendarID:       "cal1",
		UID:              "urn:uuid:9999",
		Title:            "Weekly sync",
		Start:            "2024-05-06T09:00:00",
		Duration:         "PT30M",
		TimeZone:         "Europe/London",
		OrganiserAddress: "me@example.com",
		OrganiserSendTo:  "mailto:me@example.com",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	_, args := s.call(t, "create")
	event := args["create"].(map[string]any)["meeting"].(map[string]any)
	if event["@type"] != "Event" || event["start"] != "2024-05-06T09:00:00" {
		t.Errorf("event = %v", event)
	}
	if cals := event["calendarIds"].(map[string]any); cals["cal1"] != true {
		t.Errorf("calendarIds = %v", cals)
	}
	rule := event["recurrenceRules"].([]any)[0].(map[string]any)
	if rule["frequency"] != "weekly" {
		t.Errorf("recurrence rule = %v", rule)
	}
	if day := rule["byDay"].([]any)[0].(map[string]any); day["day"] != "mo" {
		t.Errorf("byDay = %v", day)
	}
	alert := event["alerts"].(map[string]any)["reminder"].(map[string]any)
	trigger := alert["trigger"].(map[string]any)
	if trigger["offset"] != "-PT15M" {
		t.Errorf("trigger = %v, want an offset of -PT15M", trigger)
	}
	participant := event["participants"].(map[string]any)["organiser"].(map[string]any)
	if roles := participant["roles"].(map[string]any); roles["owner"] != true {
		t.Errorf("participant roles = %v", roles)
	}
	if !got.Created["meeting"].IsOrigin {
		t.Errorf("created event = %+v, want isOrigin", got.Created["meeting"])
	}
}

// TestRescheduleOccurrence checks a patch keyed by the start of the occurrence
// it changes, which is how one instance of a series is moved.
func TestRescheduleOccurrence(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["CalendarEvent/set", {"accountId": "acct1", "oldState": "e1", "newState": "e2",
	                           "updated": {"series1": null}}, "reschedule"]
	  ]
	}`}

	_, err := jmapq.RescheduleOccurrence(context.Background(), s.client(), jmapq.RescheduleOccurrenceParams{
		EventID:    "series1",
		Occurrence: "2024-05-06T09:00:00",
		NewStart:   "2024-05-06T11:00:00",
		NewTitle:   "Standup (moved)",
	})
	if err != nil {
		t.Fatalf("RescheduleOccurrence: %v", err)
	}

	_, args := s.call(t, "reschedule")
	if args["sendSchedulingMessages"] != true {
		t.Errorf("sendSchedulingMessages = %v, want true", args["sendSchedulingMessages"])
	}
	patch := args["update"].(map[string]any)["series1"].(map[string]any)
	if patch["recurrenceOverrides/2024-05-06T09:00:00/start"] != "2024-05-06T11:00:00" {
		t.Errorf("patch = %v, want the occurrence's start moved", patch)
	}
	if patch["recurrenceOverrides/2024-05-06T09:00:00/title"] != "Standup (moved)" {
		t.Errorf("patch = %v, want the occurrence's title changed", patch)
	}
	// The series itself is untouched: only the one occurrence moves.
	if len(patch) != 2 {
		t.Errorf("patch holds %d members, want 2", len(patch))
	}
}

// TestFindPeople covers JMAP Sharing: finding who an account can be shared
// with, which is what a sharing dialogue needs before it can offer anything.
func TestFindPeople(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["Principal/query", {"accountId": "acct1", "queryState": "p1",
	                         "canCalculateChanges": false, "position": 0, "ids": ["pr1"]}, "search"],
	    ["Principal/get", {"accountId": "acct1", "state": "p1", "notFound": [], "list": [
	      {"id": "pr1", "type": "individual", "name": "Ada Lovelace",
	       "email": "ada@example.com", "timeZone": "Europe/London"}
	    ]}, "fetch"]
	  ]
	}`}

	got, err := jmapq.FindPeople(context.Background(), s.client(), jmapq.FindPeopleParams{
		Phrase: "ada",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("FindPeople: %v", err)
	}

	_, search := s.call(t, "search")
	conditions := search["filter"].(map[string]any)["conditions"].([]any)
	if len(conditions) != 2 {
		t.Fatalf("filter has %d conditions, want 2", len(conditions))
	}
	if c := conditions[1].(map[string]any); c["type"] != "individual" {
		t.Errorf("second condition = %v, want type individual", c)
	}

	if len(got.List) != 1 {
		t.Fatalf("got %d principals, want 1", len(got.List))
	}
	who := got.List[0]
	if who.Type != "individual" || who.Name != "Ada Lovelace" {
		t.Errorf("principal = %+v", who)
	}
	if who.Email == nil || *who.Email != "ada@example.com" {
		t.Errorf("email = %v", who.Email)
	}
}

// TestRecentlyShared covers the other half of sharing: what someone else has
// shared with the user, which nothing else would tell them.
func TestRecentlyShared(t *testing.T) {
	s := &stub{t: t, response: `{
	  "sessionState": "session-1",
	  "methodResponses": [
	    ["ShareNotification/query", {"accountId": "acct1", "queryState": "n1",
	                                 "canCalculateChanges": false, "position": 0, "ids": ["n1"]}, "search"],
	    ["ShareNotification/get", {"accountId": "acct1", "state": "n1", "notFound": [], "list": [
	      {"id": "n1", "created": "2024-05-01T09:00:00Z",
	       "changedBy": {"name": "Ada", "email": "ada@example.com", "principalId": "pr1"},
	       "objectType": "Mailbox", "objectAccountId": "acct2", "objectId": "mbx9",
	       "oldRights": null, "newRights": {"mayReadItems": true}, "name": "Shared plans"}
	    ]}, "fetch"]
	  ]
	}`}

	got, err := jmapq.RecentlyShared(context.Background(), s.client(), jmapq.RecentlySharedParams{
		Since: jmapc.NewUTCDate(mustTime(t, "2024-04-01T00:00:00Z")),
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("RecentlyShared: %v", err)
	}

	_, search := s.call(t, "search")
	if sort := search["sort"].([]any)[0].(map[string]any); sort["property"] != "created" {
		t.Errorf("sort = %v, want created", sort)
	}

	if len(got.List) != 1 {
		t.Fatalf("got %d notifications, want 1", len(got.List))
	}
	n := got.List[0]
	if n.ObjectType != "Mailbox" || n.Name != "Shared plans" {
		t.Errorf("notification = %+v", n)
	}
	// A null oldRights means the user could not see the object at all before,
	// so this is something newly shared rather than a change of permissions.
	if n.OldRights != nil {
		t.Errorf("oldRights = %v, want nil for something newly shared", n.OldRights)
	}
	if !n.NewRights["mayReadItems"] {
		t.Errorf("newRights = %v, want mayReadItems", n.NewRights)
	}
	if n.ChangedBy.PrincipalID == nil || *n.ChangedBy.PrincipalID != "pr1" {
		t.Errorf("changedBy = %+v", n.ChangedBy)
	}
}
