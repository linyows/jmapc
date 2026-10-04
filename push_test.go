package jmapc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// pushServer is a JMAP server that keeps push subscriptions, and posts each
// new one its verification code before it answers the call that created it,
// which is the order a code may arrive in.
type pushServer struct {
	*testServer
	t *testing.T

	mu sync.Mutex
	// subs are the subscriptions, by id.
	subs map[ID]*pushSub
	// created counts the subscriptions ever created.
	created int
	// maxLifetime, where set, is the longest expiry the server grants.
	maxLifetime time.Duration
	// postCodes says whether a new subscription is sent its code.
	postCodes bool
	// lose, where set, makes the server forget every subscription before
	// answering the next update or get, once.
	lose bool
	// refuseDestroy makes the server refuse to destroy a subscription.
	refuseDestroy bool
	// asked holds each expiry an update asked for.
	asked []string
}

type pushSub struct {
	url      string
	code     string
	verified bool
	expires  *UTCDate
	// keys are what the subscription was made with, which every push to it
	// is encrypted for; nil for one made without.
	keys map[string]string
}

// deliver posts a push to a subscription's URL as the server does: encrypted
// for its keys where it has them, as it is.
func deliver(t *testing.T, s *pushSub, body string) (*http.Response, error) {
	t.Helper()
	if s.keys == nil {
		return http.Post(s.url, "application/json", strings.NewReader(body))
	}
	req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(encryptPush(t, s.keys, []byte(body), 4096)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	return http.DefaultClient.Do(req)
}

func newPushServer(t *testing.T) *pushServer {
	t.Helper()
	ps := &pushServer{testServer: newTestServer(t), t: t, subs: map[ID]*pushSub{}, postCodes: true}
	ps.apiHandler = func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MethodCalls []Invocation `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("the server could not read the request: %v", err)
			return
		}
		call := req.MethodCalls[0]
		raw, _ := json.Marshal(call.Args)
		var answer any
		switch call.Name {
		case "PushSubscription/set":
			answer = ps.set(raw)
		case "PushSubscription/get":
			answer = ps.get(raw)
		default:
			t.Errorf("the receiver called %s", call.Name)
		}
		out, _ := json.Marshal(answer)
		fmt.Fprintf(w, `{"sessionState":"sess1","methodResponses":[[%q,%s,%q]]}`, call.Name, out, call.CallID)
	}
	return ps
}

func (ps *pushServer) set(raw json.RawMessage) any {
	var args struct {
		Create  map[ID]PushSubscription `json:"create"`
		Update  map[ID]map[string]any   `json:"update"`
		Destroy []ID                    `json:"destroy"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		ps.t.Errorf("the server could not read the set: %v", err)
	}
	created, notCreated := map[ID]any{}, map[ID]any{}
	updated, notUpdated := map[ID]any{}, map[ID]any{}
	var destroyed []ID

	for cid, sub := range args.Create {
		ps.mu.Lock()
		ps.created++
		id := ID(fmt.Sprintf("ps%d", ps.created))
		s := &pushSub{url: sub.URL, code: fmt.Sprintf("code-%s", id), expires: ps.grant(sub.Expires)}
		if sub.Keys != nil {
			s.keys = map[string]string{"p256dh": sub.Keys.P256dh, "auth": sub.Keys.Auth}
		}
		ps.subs[id] = s
		post := ps.postCodes
		ps.mu.Unlock()
		if post {
			body := fmt.Sprintf(`{"@type":"PushVerification","pushSubscriptionId":%q,"verificationCode":%q}`, id, s.code)
			resp, err := deliver(ps.t, s, body)
			if err != nil {
				ps.t.Errorf("posting the verification: %v", err)
			} else {
				resp.Body.Close()
				if resp.StatusCode != http.StatusCreated {
					ps.t.Errorf("the receiver answered the verification with %d", resp.StatusCode)
				}
			}
		}
		answer := map[string]any{"id": id}
		if s.expires != nil && (sub.Expires == nil || !s.expires.Equal(sub.Expires.Time)) {
			answer["expires"] = s.expires
		}
		created[cid] = answer
	}

	ps.mu.Lock()
	if ps.lose && len(args.Update) > 0 {
		clear(ps.subs)
		ps.lose = false
	}
	for id, patch := range args.Update {
		s, ok := ps.subs[id]
		if !ok {
			notUpdated[id] = map[string]string{"type": "notFound"}
			continue
		}
		var changed any
		if code, ok := patch["verificationCode"].(string); ok {
			if code != s.code {
				notUpdated[id] = map[string]string{"type": "invalidProperties"}
				continue
			}
			s.verified = true
		}
		if when, ok := patch["expires"].(string); ok {
			ps.asked = append(ps.asked, when)
			var asked UTCDate
			_ = json.Unmarshal([]byte(fmt.Sprintf("%q", when)), &asked)
			s.expires = ps.grant(&asked)
			if !s.expires.Equal(asked.Time) {
				changed = map[string]any{"expires": s.expires}
			}
		}
		updated[id] = changed
	}
	notDestroyed := map[ID]any{}
	for _, id := range args.Destroy {
		if ps.refuseDestroy {
			notDestroyed[id] = map[string]string{"type": "forbidden"}
			continue
		}
		if _, ok := ps.subs[id]; ok {
			delete(ps.subs, id)
			destroyed = append(destroyed, id)
		}
	}
	ps.mu.Unlock()
	return map[string]any{
		"created": created, "notCreated": notCreated,
		"updated": updated, "notUpdated": notUpdated,
		"destroyed": destroyed, "notDestroyed": notDestroyed,
	}
}

func (ps *pushServer) get(raw json.RawMessage) any {
	var args struct {
		IDs []ID `json:"ids"`
	}
	_ = json.Unmarshal(raw, &args)
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.lose {
		clear(ps.subs)
		ps.lose = false
	}
	list, notFound := []any{}, []ID{}
	for _, id := range args.IDs {
		if s, ok := ps.subs[id]; ok {
			list = append(list, map[string]any{"id": id, "expires": s.expires})
		} else {
			notFound = append(notFound, id)
		}
	}
	return map[string]any{"list": list, "notFound": notFound}
}

// grant is the expiry the server grants for one asked for.
func (ps *pushServer) grant(asked *UTCDate) *UTCDate {
	if ps.maxLifetime == 0 {
		return asked
	}
	limit := NewUTCDate(time.Now().Add(ps.maxLifetime))
	if asked == nil || asked.After(limit.Time) {
		return &limit
	}
	return asked
}

func (ps *pushServer) subscriptions() map[ID]pushSub {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := map[ID]pushSub{}
	for id, s := range ps.subs {
		out[id] = *s
	}
	return out
}

// events collects what a receiver reports, and lets a test wait for a step.
type events struct {
	mu   sync.Mutex
	seen []PushEvent
	ch   chan PushEvent
}

func newEvents() *events { return &events{ch: make(chan PushEvent, 64)} }

func (e *events) record(ev PushEvent) {
	e.mu.Lock()
	e.seen = append(e.seen, ev)
	e.mu.Unlock()
	e.ch <- ev
}

func (e *events) waitFor(t *testing.T, kind PushEventKind) PushEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev := <-e.ch:
			if ev.Kind == kind {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event arrived; saw %v", kind, e.kinds())
			return PushEvent{}
		}
	}
}

func (e *events) kinds() []PushEventKind {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []PushEventKind
	for _, ev := range e.seen {
		out = append(out, ev.Kind)
	}
	return out
}

// startReceiver serves a receiver on a server of its own and runs it.
func startReceiver(t *testing.T, ps *pushServer, opts PushReceiverOptions, tune func(*PushReceiver)) (*PushReceiver, *events, func() error) {
	t.Helper()
	ev := newEvents()
	var r *PushReceiver
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.ServeHTTP(w, req) }))
	t.Cleanup(srv.Close)
	opts.URL = srv.URL + "/push/secret"
	opts.OnEvent = ev.record
	r = NewPushReceiver(ps.client(), opts)
	if tune != nil {
		tune(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(cancel)
	return r, ev, func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after its context ended")
			return nil
		}
	}
}

// blockAfter returns a wait that returns at once n times, then waits for the
// context to end, so that a test sees a set number of renewals.
func blockAfter(n int, waited *[]time.Duration, mu *sync.Mutex) func(ctx context.Context, d time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		*waited = append(*waited, d)
		count := len(*waited)
		mu.Unlock()
		if count <= n {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestPushReceiverSubscribesVerifiesAndRemoves(t *testing.T) {
	ps := newPushServer(t)
	var changes []*StateChange
	var mu sync.Mutex
	r, ev, stop := startReceiver(t, ps, PushReceiverOptions{
		DeviceClientID: "app-1",
		Types:          []string{"Email"},
		OnStateChange: func(ctx context.Context, c *StateChange) {
			mu.Lock()
			changes = append(changes, c)
			mu.Unlock()
		},
	}, func(r *PushReceiver) { r.wait = blockAfter(0, new([]time.Duration), new(sync.Mutex)) })

	verified := ev.waitFor(t, PushVerified)
	subs := ps.subscriptions()
	if len(subs) != 1 || !subs[verified.SubscriptionID].verified {
		t.Fatalf("the server holds %v, want the one subscription, verified", subs)
	}
	sub := subs[verified.SubscriptionID]
	if sub.keys == nil {
		t.Fatal("the subscription was made without keys, though nothing asked for plain text")
	}

	// A plain state change, which anyone who knows the URL could post, is
	// refused: only one encrypted for the keys came from the server.
	plain, err := http.Post(r.opts.URL, "application/json",
		strings.NewReader(`{"@type":"StateChange","changed":{"a1":{"Email":"forged"}}}`))
	if err != nil {
		t.Fatalf("posting a plain state change: %v", err)
	}
	plain.Body.Close()
	if plain.StatusCode != http.StatusBadRequest {
		t.Errorf("the receiver answered a plain state change with %d, want 400", plain.StatusCode)
	}

	// A state change the server pushes reaches OnStateChange.
	resp, err := deliver(t, &sub, `{"@type":"StateChange","changed":{"a1":{"Email":"s2"}}}`)
	if err != nil {
		t.Fatalf("pushing a state change: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("the receiver answered the state change with %d, want 201", resp.StatusCode)
	}
	mu.Lock()
	if len(changes) != 1 {
		t.Errorf("OnStateChange saw %d changes, want 1", len(changes))
	} else if state, _ := changes[0].StateOf("a1", "Email"); state != "s2" {
		t.Errorf("the change reports %q, want s2", state)
	}
	mu.Unlock()

	if err := stop(); !errors.Is(err, context.Canceled) {
		t.Errorf("Run returned %v, want the context's error", err)
	}
	if len(ps.subscriptions()) != 0 {
		t.Errorf("the subscription was left on the server: %v", ps.subscriptions())
	}
	if got := fmt.Sprint(ev.kinds()); got != "[created verified destroyed]" {
		t.Errorf("events = %s", got)
	}
}

// TestPushReceiverRenewsWithinWhatTheServerGrants checks that the expiry is
// extended before it passes, and that the receiver keeps to the expiry the
// server grants where it grants less than it was asked.
func TestPushReceiverRenewsWithinWhatTheServerGrants(t *testing.T) {
	ps := newPushServer(t)
	ps.maxLifetime = time.Hour
	var waited []time.Duration
	var mu sync.Mutex
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1", Lifetime: 24 * time.Hour},
		func(r *PushReceiver) { r.wait = blockAfter(2, &waited, &mu) })

	created := ev.waitFor(t, PushCreated)
	if d := time.Until(created.Expires); d > time.Hour+time.Minute {
		t.Errorf("the receiver took the expiry as %v away, more than the hour the server granted", d)
	}
	ev.waitFor(t, PushRenewed)
	ev.waitFor(t, PushRenewed)
	_ = stop()

	mu.Lock()
	defer mu.Unlock()
	if len(waited) < 2 {
		t.Fatalf("the receiver waited %d times, want each renewal timed", len(waited))
	}
	// Four fifths of the hour the server granted, not of the day asked for.
	if waited[0] > 50*time.Minute || waited[0] < 40*time.Minute {
		t.Errorf("the first renewal waited %v, want about four fifths of an hour", waited[0])
	}
}

// TestPushReceiverMakesALostSubscriptionAgain checks that a subscription the
// server no longer has is made again, and verified again.
func TestPushReceiverMakesALostSubscriptionAgain(t *testing.T) {
	ps := newPushServer(t)
	var waited []time.Duration
	var mu sync.Mutex
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1", Lifetime: time.Hour},
		func(r *PushReceiver) {
			r.wait = func(ctx context.Context, d time.Duration) error {
				mu.Lock()
				waited = append(waited, d)
				first := len(waited) == 1
				mu.Unlock()
				if first {
					// The server forgets the subscription before the renewal.
					ps.mu.Lock()
					ps.lose = true
					ps.mu.Unlock()
					return nil
				}
				<-ctx.Done()
				return ctx.Err()
			}
		})

	first := ev.waitFor(t, PushVerified)
	lost := ev.waitFor(t, PushLost)
	again := ev.waitFor(t, PushVerified)
	if lost.SubscriptionID != first.SubscriptionID || again.SubscriptionID == first.SubscriptionID {
		t.Errorf("lost %s after verifying %s, then verified %s; want a new subscription", lost.SubscriptionID, first.SubscriptionID, again.SubscriptionID)
	}
	_ = stop()
	if ps.created != 2 {
		t.Errorf("the server created %d subscriptions, want 2", ps.created)
	}
}

// TestPushReceiverChecksASubscriptionThatDoesNotExpire checks a subscription
// made without an expiry: there is nothing to extend, so it is looked up
// instead, and made again where it is gone.
func TestPushReceiverChecksASubscriptionThatDoesNotExpire(t *testing.T) {
	ps := newPushServer(t)
	var calls int
	var mu sync.Mutex
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1", CheckInterval: 7 * time.Minute},
		func(r *PushReceiver) {
			r.wait = func(ctx context.Context, d time.Duration) error {
				mu.Lock()
				calls++
				n := calls
				mu.Unlock()
				if d != 7*time.Minute {
					t.Errorf("the check waited %v, want the interval", d)
				}
				if n == 2 {
					ps.mu.Lock()
					ps.lose = true
					ps.mu.Unlock()
				}
				if n <= 2 {
					return nil
				}
				<-ctx.Done()
				return ctx.Err()
			}
		})
	ev.waitFor(t, PushVerified)
	ev.waitFor(t, PushLost)
	ev.waitFor(t, PushVerified)
	_ = stop()
	for _, kind := range ev.kinds() {
		if kind == PushRenewed {
			t.Errorf("a subscription without an expiry was renewed: %v", ev.kinds())
		}
	}
}

// TestPushReceiverGivesUpWithoutAVerificationCode checks that a URL the server
// cannot reach, which is where no code arrives, ends Run with an error that
// says so, and leaves no subscription behind.
func TestPushReceiverGivesUpWithoutAVerificationCode(t *testing.T) {
	ps := newPushServer(t)
	ps.postCodes = false
	c := ps.client()
	r := NewPushReceiver(c, PushReceiverOptions{URL: "https://unreachable.example/push", DeviceClientID: "app-1", VerifyTimeout: 50 * time.Millisecond})
	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no verification code") {
		t.Errorf("Run returned %v, want it to say no code arrived", err)
	}
	if len(ps.subscriptions()) != 0 {
		t.Errorf("the unverified subscription was left on the server: %v", ps.subscriptions())
	}
}

func TestPushReceiverRefusesWhatIsNotAPush(t *testing.T) {
	r := NewPushReceiver(nil, PushReceiverOptions{PlainText: true})
	for _, tt := range []struct {
		method, body string
		want         int
	}{
		{http.MethodGet, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "not json", http.StatusBadRequest},
		{http.MethodPost, `{"@type":"Mailbox"}`, http.StatusBadRequest},
		{http.MethodPost, `{"@type":"PushVerification","pushSubscriptionId":"ps1"}`, http.StatusBadRequest},
		{http.MethodPost, strings.Repeat(" ", maxPushBody+1), http.StatusBadRequest},
		{http.MethodPost, `{"@type":"StateChange","changed":{}}`, http.StatusCreated},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tt.method, "/push", bytes.NewBufferString(tt.body)))
		if w.Code != tt.want {
			t.Errorf("%s %.40q answered %d, want %d", tt.method, tt.body, w.Code, tt.want)
		}
	}
}

// TestPushReceiverKeepsFewCodes checks that codes posted for subscriptions
// Run never asks about cannot pile up without end.
func TestPushReceiverKeepsFewCodes(t *testing.T) {
	r := NewPushReceiver(nil, PushReceiverOptions{PlainText: true})
	for i := range maxPendingCodes * 4 {
		body := fmt.Sprintf(`{"@type":"PushVerification","pushSubscriptionId":"x%d","verificationCode":"c"}`, i)
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/push", strings.NewReader(body)))
	}
	if len(r.codes) > maxPendingCodes {
		t.Errorf("the receiver keeps %d codes, want at most %d", len(r.codes), maxPendingCodes)
	}
}

// TestPushReceiverReturnsTheContextsErrorWhileVerifying checks that ending
// Run while it waits for a code returns the context's error, as Run says,
// rather than reporting that no code came.
func TestPushReceiverReturnsTheContextsErrorWhileVerifying(t *testing.T) {
	ps := newPushServer(t)
	ps.postCodes = false
	r := NewPushReceiver(ps.client(), PushReceiverOptions{URL: "https://push.example/secret", DeviceClientID: "app-1", VerifyTimeout: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	for len(ps.subscriptions()) == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want the context's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}

// TestPushReceiverExtendsAnExpiryTheServerSet checks a subscription made
// without asking for an expiry, which the server gave one all the same: it is
// extended by as long as the server granted at first, never to a date that
// has passed.
func TestPushReceiverExtendsAnExpiryTheServerSet(t *testing.T) {
	ps := newPushServer(t)
	ps.maxLifetime = time.Hour
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1"},
		func(r *PushReceiver) { r.wait = blockAfter(1, new([]time.Duration), new(sync.Mutex)) })
	renewed := ev.waitFor(t, PushRenewed)
	_ = stop()
	if d := time.Until(renewed.Expires); d < 50*time.Minute {
		t.Errorf("the extension leaves %v, want about the hour the server granted", d)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if len(ps.asked) != 1 {
		t.Fatalf("the receiver asked for %v, want one extension", ps.asked)
	}
	var asked UTCDate
	_ = json.Unmarshal([]byte(fmt.Sprintf("%q", ps.asked[0])), &asked)
	if !asked.After(time.Now()) {
		t.Errorf("the extension asked for %s, which has passed", ps.asked[0])
	}
}

// TestPushReceiverReportsOnlyWhatWasDestroyed checks that a subscription the
// server refuses to destroy is not reported as destroyed.
func TestPushReceiverReportsOnlyWhatWasDestroyed(t *testing.T) {
	ps := newPushServer(t)
	ps.refuseDestroy = true
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1"},
		func(r *PushReceiver) { r.wait = blockAfter(0, new([]time.Duration), new(sync.Mutex)) })
	ev.waitFor(t, PushVerified)
	_ = stop()
	for _, kind := range ev.kinds() {
		if kind == PushDestroyed {
			t.Errorf("a subscription the server kept was reported destroyed: %v", ev.kinds())
		}
	}
}

// TestPushReceiverInPlainTextTakesPlainPushes checks a receiver for a server
// that does not encrypt: the subscription is made without keys, and the
// pushes are read as they come.
func TestPushReceiverInPlainTextTakesPlainPushes(t *testing.T) {
	ps := newPushServer(t)
	_, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1", PlainText: true},
		func(r *PushReceiver) { r.wait = blockAfter(0, new([]time.Duration), new(sync.Mutex)) })
	verified := ev.waitFor(t, PushVerified)
	if sub := ps.subscriptions()[verified.SubscriptionID]; sub.keys != nil {
		t.Errorf("the subscription was made with keys, though plain text was asked for")
	}
	_ = stop()
}

// TestPushReceiverRefusesAPushForOtherKeys checks that an encrypted push the
// receiver's keys do not open is refused, as one forged with keys of the
// forger's own would be.
func TestPushReceiverRefusesAPushForOtherKeys(t *testing.T) {
	ps := newPushServer(t)
	r, ev, stop := startReceiver(t, ps, PushReceiverOptions{DeviceClientID: "app-1"},
		func(r *PushReceiver) { r.wait = blockAfter(0, new([]time.Duration), new(sync.Mutex)) })
	defer stop()
	ev.waitFor(t, PushVerified)
	other, _ := newPushKeys()
	forged := &pushSub{url: r.opts.URL, keys: other.subscription()}
	resp, err := deliver(t, forged, `{"@type":"StateChange","changed":{}}`)
	if err != nil {
		t.Fatalf("posting: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("the receiver answered a push for other keys with %d, want 400", resp.StatusCode)
	}
}
