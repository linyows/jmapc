package example

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/example/jmapq"
	"github.com/linyows/jmapc/jmaptest"
)

// TestListInboxEmailsAgainstTheTestServer runs a generated function against the
// server jmapc ships for the purpose, which is how somebody using jmapc tests
// the code they wrote around it. Nothing here knows what the request looks
// like: the query said that, and the server resolves the back reference the
// way a real one does.
func TestListInboxEmailsAgainstTheTestServer(t *testing.T) {
	srv := jmaptest.New(t)
	srv.Reply("Email/query", map[string]any{
		"accountId": jmaptest.AccountID, "queryState": "q1", "position": 0,
		"ids": []string{"m1", "m2"},
	})
	srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
		list := make([]map[string]any, 0, len(c.IDs()))
		for _, id := range c.IDs() {
			list = append(list, map[string]any{
				"id": id, "threadId": "t1", "subject": "about " + string(id),
				"from":       []map[string]any{{"email": "someone@example.com"}},
				"receivedAt": "2026-09-04T09:00:00Z", "preview": "...", "hasAttachment": false,
			})
		}
		return map[string]any{"accountId": jmaptest.AccountID, "state": "s1", "list": list, "notFound": []string{}}, nil
	})

	res, err := jmapq.ListInboxEmails(context.Background(), srv.Client(),
		jmapq.ListInboxEmailsParams{MailboxID: "mbx1", Limit: 10})
	if err != nil {
		t.Fatalf("ListInboxEmails: %v", err)
	}
	if len(res.List) != 2 || *res.List[1].Subject != "about m2" {
		t.Fatalf("the emails came back as %v", res.List)
	}

	// The account the query left out was filled in from the session, and the
	// two calls travelled together.
	if got := srv.Call("Email/query").AccountID(); got != jmaptest.AccountID {
		t.Errorf("the query ran against %q", got)
	}
	if n := srv.Requests(); n != 1 {
		t.Errorf("the query took %d requests, want one", n)
	}
}

// TestSendEmailRefusedByTheServer covers the failure that answers 200: the
// server lists the records it would not act on, and the generated code reports
// it rather than reading the request as a success.
func TestSendEmailRefusedByTheServer(t *testing.T) {
	srv := jmaptest.New(t)
	srv.Reply("Email/set", map[string]any{
		"accountId": jmaptest.AccountID, "newState": "s2",
		"notCreated": map[string]any{"draft": map[string]any{
			"type": "invalidProperties", "properties": []string{"subject"},
		}},
	})
	srv.Reply("EmailSubmission/set", map[string]any{"accountId": jmaptest.AccountID, "newState": "sub2"})

	_, err := jmapq.SendEmail(context.Background(), srv.Client(), jmapq.SendEmailParams{
		DraftsMailboxID: "drafts", SentMailboxID: "sent", IdentityID: "id1",
		FromAddress: "me@example.com", ToAddress: "you@example.com",
		Subject: "Lunch", Body: "Thursday?",
	})
	var refused *jmapc.SetErrors
	if !errors.As(err, &refused) {
		t.Fatalf("SendEmail answered %v, want the refusal", err)
	}
	if refused.Failures[0].Key != "draft" {
		t.Errorf("the refusal was filed under %q", refused.Failures[0].Key)
	}
}

// TestSyncEmailsWatchAgainstTheTestServer covers the loop a watching query
// generates, against a server that can push: the watch catches up on
// connecting, waits, and catches up again when the server says Email has moved
// on.
func TestSyncEmailsWatchAgainstTheTestServer(t *testing.T) {
	srv := jmaptest.New(t)
	answered := make(chan struct{}, 4)
	catchUps := 0
	srv.Handle("Email/changes", func(c *jmaptest.Call) (any, error) {
		catchUps++
		defer func() { answered <- struct{}{} }()
		if catchUps == 1 {
			// Nothing happened while the client was away.
			return changes("s1", nil), nil
		}
		return changes("s2", []jmapc.ID{"m1"}), nil
	})
	srv.Handle("Email/get", func(c *jmaptest.Call) (any, error) {
		list := make([]map[string]any, 0, len(c.IDs()))
		for _, id := range c.IDs() {
			list = append(list, map[string]any{"id": id, "threadId": "t1", "mailboxIds": map[string]bool{"mbx1": true},
				"keywords": map[string]bool{}, "subject": "the new one", "receivedAt": "2026-09-04T09:00:00Z"})
		}
		return map[string]any{"accountId": jmaptest.AccountID, "state": "s2", "list": list, "notFound": []string{}}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		// The catch-up on connecting has been answered, so the client is
		// listening and there is something to tell it.
		<-answered
		srv.Push(jmaptest.AccountID, map[string]string{"Email": "s2"})
	}()

	var subjects []string
	err := jmapq.SyncEmailsWatch(ctx, srv.Client(), jmapq.SyncEmailsParams{SinceState: "s1"},
		func(ctx context.Context, res *jmapq.SyncEmailsResult) error {
			for _, email := range res.EmailGet.List {
				subjects = append(subjects, *email.Subject)
			}
			if res.EmailChanges.NewState == "s2" {
				cancel()
			}
			return nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncEmailsWatch: %v", err)
	}
	if len(subjects) != 1 || subjects[0] != "the new one" {
		t.Errorf("the watch reported %v, want the one new message", subjects)
	}
}

// changes renders an Email/changes response reaching the given state.
func changes(newState string, created []jmapc.ID) map[string]any {
	if created == nil {
		created = []jmapc.ID{}
	}
	return map[string]any{
		"accountId": jmaptest.AccountID, "oldState": "s1", "newState": newState,
		"hasMoreChanges": false, "created": created, "updated": []jmapc.ID{}, "destroyed": []jmapc.ID{},
	}
}
