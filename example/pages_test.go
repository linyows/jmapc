package example

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linyows/jmapc"
	"github.com/linyows/jmapc/example/jmapq"
)

// pagingStub answers each request with the next window it was given, and
// records the position each request asked from.
type pagingStub struct {
	t         *testing.T
	responses []string
	positions []float64
}

func (s *pagingStub) start() *httptest.Server {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	s.t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/jmap", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
		  "capabilities": {"urn:ietf:params:jmap:core": {}, "urn:ietf:params:jmap:mail": {}},
		  "accounts": {"acct1": {"name": "someone@example.com", "isPersonal": true}},
		  "primaryAccounts": {"urn:ietf:params:jmap:mail": "acct1"},
		  "username": "someone@example.com",
		  "apiUrl": %q,
		  "state": "session-1"
		}`, srv.URL+"/jmap/api/")
	})
	mux.HandleFunc("/jmap/api/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.t.Errorf("reading the request: %v", err)
			return
		}
		var req struct {
			MethodCalls []json.RawMessage `json:"methodCalls"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.t.Errorf("the request is not JSON: %v", err)
			return
		}
		var search []any
		if err := json.Unmarshal(req.MethodCalls[0], &search); err != nil {
			s.t.Errorf("the first call is not an invocation: %v", err)
			return
		}
		position, _ := search[1].(map[string]any)["position"].(float64)
		s.positions = append(s.positions, position)

		if len(s.positions) > len(s.responses) {
			s.t.Errorf("the walk asked for a window past the end, from %v", position)
			w.Write([]byte(`{"sessionState":"session-1","methodResponses":[]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, s.responses[len(s.positions)-1])
	})
	return srv
}

// window renders a response holding one window of the search and the emails in
// it, as the two calls of SearchEmails ask for them.
func window(position int, total int, ids []string) string {
	list := make([]string, len(ids))
	quoted := make([]string, len(ids))
	for i, id := range ids {
		list[i] = fmt.Sprintf(`{"id": %q, "subject": "message %s", "from": [{"email": "a@example.com"}],
		                        "receivedAt": "2026-09-04T09:00:00Z"}`, id, id)
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf(`{"sessionState": "session-1", "methodResponses": [
	  ["Email/query", {"accountId": "acct1", "queryState": "q1", "canCalculateChanges": false,
	                   "position": %d, "total": %d, "ids": [%s]}, "search"],
	  ["Email/get", {"accountId": "acct1", "state": "s1", "list": [%s], "notFound": []}, "fetch"]
	]}`, position, total, join(quoted), join(list))
}

// join puts a comma between the parts, which is all this needs of a list.
func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// client returns a client pointed at the paging stub.
func (s *pagingStub) client() *jmapc.Client {
	return jmapc.New(s.start().URL+"/.well-known/jmap", jmapc.WithBearerToken("token"))
}

// TestSearchEmailsPages walks a search whose results do not fit in one request:
// each window says where it sits, and the walk stops when the total says there
// is nothing after it.
func TestSearchEmailsPages(t *testing.T) {
	s := &pagingStub{t: t, responses: []string{
		window(0, 3, []string{"m1", "m2"}),
		window(2, 3, []string{"m3"}),
	}}

	var subjects []string
	for page, err := range jmapq.SearchEmailsPages(context.Background(), s.client(),
		jmapq.SearchEmailsParams{Phrase: "invoice", FirstMailboxID: "mbx1", SecondMailboxID: "mbx2"}) {
		if err != nil {
			t.Fatalf("the walk failed: %v", err)
		}
		for _, email := range page.EmailGet.List {
			subjects = append(subjects, *email.Subject)
		}
	}

	if len(subjects) != 3 {
		t.Errorf("the walk found %v, want all three messages", subjects)
	}
	// The second request starts where the first window ended, and the total
	// ends the walk without a third asking for a window that is not there.
	want := []float64{0, 2}
	if len(s.positions) != len(want) {
		t.Fatalf("the walk made %d requests, from %v, want two", len(s.positions), s.positions)
	}
	for i, position := range want {
		if s.positions[i] != position {
			t.Errorf("request %d asked from %v, want %v", i, s.positions[i], position)
		}
	}
}

// TestSearchEmailsPagesStopsAtAnEmptyWindow covers a server that does not count
// the total: the walk ends at the window with nothing in it, and that window is
// not handed back.
func TestSearchEmailsPagesStopsAtAnEmptyWindow(t *testing.T) {
	s := &pagingStub{t: t, responses: []string{
		window(0, 0, []string{"m1"}),
		window(1, 0, nil),
	}}

	pages := 0
	for page, err := range jmapq.SearchEmailsPages(context.Background(), s.client(), jmapq.SearchEmailsParams{}) {
		if err != nil {
			t.Fatalf("the walk failed: %v", err)
		}
		if len(page.EmailGet.List) == 0 {
			t.Error("a window with nothing in it was handed back")
		}
		pages++
	}
	if pages != 1 {
		t.Errorf("the walk handed back %d windows, want one", pages)
	}
}

// TestSearchEmailsPagesReportsAFailure checks that an error ends the walk and
// arrives where the caller is looking for it.
func TestSearchEmailsPagesReportsAFailure(t *testing.T) {
	s := &pagingStub{t: t, responses: []string{
		`{"sessionState": "session-1", "methodResponses": [
		  ["error", {"type": "invalidArguments"}, "search"]]}`,
	}}

	var failed error
	pages := 0
	for page, err := range jmapq.SearchEmailsPages(context.Background(), s.client(), jmapq.SearchEmailsParams{}) {
		if err != nil {
			failed = err
			continue
		}
		_ = page
		pages++
	}
	var methodErrs jmapc.MethodErrors
	if !errors.As(failed, &methodErrs) {
		t.Fatalf("the walk reported %v, want the method error", failed)
	}
	if pages != 0 {
		t.Errorf("the walk handed back %d windows for a failed request", pages)
	}
}
