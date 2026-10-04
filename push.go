package jmapc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// PushReceiverOptions configures a PushReceiver.
type PushReceiverOptions struct {
	// URL is where the server is to post, which must reach the receiver's
	// ServeHTTP. JMAP gives a push no signature, so anyone who knows the URL
	// can post to it; put something in it no one can guess, such as a random
	// path segment, and serve only that path.
	URL string
	// DeviceClientID identifies this receiver to the server, and stays the
	// same when a subscription is made again: RFC 8620 asks for one that
	// does not change for as long as the device does.
	DeviceClientID string
	// Types are the data types to be told of changes to, such as "Email". Nil
	// asks for every type.
	Types []string
	// Lifetime is how long a subscription is asked to last, and how far it is
	// extended each time. The server may grant less, and the receiver keeps
	// to what it grants. Zero leaves the expiry to the server.
	Lifetime time.Duration
	// VerifyTimeout is how long to wait for the server's verification code
	// after creating a subscription. Zero waits a minute.
	VerifyTimeout time.Duration
	// CheckInterval is how often a subscription that does not expire is
	// checked for still existing. Zero checks every half hour.
	CheckInterval time.Duration
	// OnStateChange is called with each state change the server posts. It is
	// called while the server's request waits for an answer, so it should hand
	// the change on rather than act on it there.
	OnStateChange func(ctx context.Context, change *StateChange)
	// OnEvent, when set, is told of each step of the subscription's life.
	OnEvent func(PushEvent)
}

// PushEventKind names a step in the life of a push subscription.
type PushEventKind string

const (
	// PushCreated is a subscription the server has created, not yet verified.
	PushCreated PushEventKind = "created"
	// PushVerified is a subscription the server has been sent its code for,
	// and pushes to from then on.
	PushVerified PushEventKind = "verified"
	// PushRenewed is a subscription whose expiry was extended.
	PushRenewed PushEventKind = "renewed"
	// PushLost is a subscription the server no longer has, which is made
	// again.
	PushLost PushEventKind = "lost"
	// PushDestroyed is a subscription removed as Run ended.
	PushDestroyed PushEventKind = "destroyed"
)

// PushEvent reports one step in the life of a push subscription.
type PushEvent struct {
	Kind PushEventKind
	// SubscriptionID is the id of the subscription the step is about.
	SubscriptionID ID
	// Expires is when the subscription expires, and zero where it does not.
	Expires time.Time
}

// PushReceiver keeps a push subscription for as long as Run runs, and receives
// what the server posts for it, as RFC 8620, Section 7.2 describes. It is an
// http.Handler for the URL the subscription names.
//
// Run creates the subscription, waits for the server to post the verification
// code to ServeHTTP and sends it back, which is when the server starts pushing.
// It extends the subscription before it expires, makes it again where the
// server no longer has it, and removes it when Run returns. ServeHTTP hands
// each state change to OnStateChange.
//
// The body of a push is not encrypted: the subscription is made without keys.
// A receiver that needs the content of a push hidden from what carries it is
// one to put behind a push service that decrypts it.
type PushReceiver struct {
	c    *Client
	opts PushReceiverOptions

	mu sync.Mutex
	// codes holds the verification codes that have arrived, by subscription
	// id. A code may arrive before the response creating its subscription
	// does, so it is kept until Run asks for it.
	codes map[ID]string
	// arrived is signalled when a code arrives.
	arrived chan struct{}

	// wait sleeps for d or until ctx ends. Tests replace it.
	wait func(ctx context.Context, d time.Duration) error
	// now tells the time. Tests replace it.
	now func() time.Time
}

// maxPushBody is the largest push ServeHTTP reads. A state change names types
// and states, and is far smaller.
const maxPushBody = 1 << 20

// maxPendingCodes bounds the codes kept for subscriptions Run has not asked
// about, so that posts to the URL cannot grow them without end.
const maxPendingCodes = 16

// NewPushReceiver returns a receiver for the user c authenticates as,
// configured by opts. A push subscription belongs to the user rather than to
// an account, and a state change it brings may name every account the user
// has.
func NewPushReceiver(c *Client, opts PushReceiverOptions) *PushReceiver {
	if opts.VerifyTimeout <= 0 {
		opts.VerifyTimeout = time.Minute
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = 30 * time.Minute
	}
	return &PushReceiver{
		c:       c,
		opts:    opts,
		codes:   make(map[ID]string),
		arrived: make(chan struct{}, 1),
		wait:    sleep,
		now:     time.Now,
	}
}

// ServeHTTP receives what the server posts: the verification code of a new
// subscription, and state changes. It answers 201 to a push it accepted, as a
// push service does, 400 to one it could not read, and 405 to anything but a
// POST.
func (r *PushReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "a push is posted", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxPushBody+1))
	if err != nil || len(body) > maxPushBody {
		http.Error(w, "the push could not be read", http.StatusBadRequest)
		return
	}
	var kind struct {
		Type string `json:"@type"`
	}
	if err := json.Unmarshal(body, &kind); err != nil {
		http.Error(w, "the push is not JSON", http.StatusBadRequest)
		return
	}
	switch kind.Type {
	case "PushVerification":
		var v PushVerification
		if err := json.Unmarshal(body, &v); err != nil || v.PushSubscriptionID == "" || v.VerificationCode == "" {
			http.Error(w, "the verification could not be read", http.StatusBadRequest)
			return
		}
		r.keepCode(v.PushSubscriptionID, v.VerificationCode)
	case "StateChange":
		var change StateChange
		if err := json.Unmarshal(body, &change); err != nil {
			http.Error(w, "the state change could not be read", http.StatusBadRequest)
			return
		}
		if r.opts.OnStateChange != nil {
			r.opts.OnStateChange(req.Context(), &change)
		}
	default:
		http.Error(w, "the push is neither a verification nor a state change", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// keepCode keeps a verification code until Run asks for it.
func (r *PushReceiver) keepCode(id ID, code string) {
	r.mu.Lock()
	if _, known := r.codes[id]; !known && len(r.codes) >= maxPendingCodes {
		r.mu.Unlock()
		return
	}
	r.codes[id] = code
	r.mu.Unlock()
	select {
	case r.arrived <- struct{}{}:
	default:
	}
}

// awaitCode returns the verification code for id once it has arrived. It
// returns ctx's error where ctx ended first, and says that no code came where
// VerifyTimeout ran out.
func (r *PushReceiver) awaitCode(ctx context.Context, id ID) (string, error) {
	limit, cancel := context.WithTimeout(ctx, r.opts.VerifyTimeout)
	defer cancel()
	for {
		r.mu.Lock()
		code, ok := r.codes[id]
		if ok {
			// Codes for other subscriptions are of no further use.
			clear(r.codes)
		}
		r.mu.Unlock()
		if ok {
			return code, nil
		}
		select {
		case <-r.arrived:
		case <-limit.Done():
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("jmapc: no verification code reached %s for push subscription %s within %s", r.opts.URL, id, r.opts.VerifyTimeout)
		}
	}
}

// errSubscriptionLost is a subscription the server no longer has.
var errSubscriptionLost = errors.New("jmapc: the push subscription is gone")

// Run keeps a push subscription until ctx ends, then removes it and returns
// ctx's error. It returns earlier with an error where a subscription cannot be
// made or verified, or a failure that waiting will not resolve.
func (r *PushReceiver) Run(ctx context.Context) error {
	for {
		id, expires, err := r.subscribe(ctx)
		if err != nil {
			return err
		}
		// What to ask for at each extension: Lifetime, or where that leaves
		// the expiry to the server and the server set one, as long as the
		// server granted at first.
		lifetime := r.opts.Lifetime
		if lifetime <= 0 && !expires.IsZero() {
			lifetime = expires.Sub(r.now())
		}
		err = r.keep(ctx, id, expires, lifetime)
		if errors.Is(err, errSubscriptionLost) {
			r.event(PushLost, id, time.Time{})
			continue
		}
		r.destroy(id)
		return err
	}
}

// subscribe creates a subscription and verifies it.
func (r *PushReceiver) subscribe(ctx context.Context) (ID, time.Time, error) {
	sub := map[string]any{
		"deviceClientId": r.opts.DeviceClientID,
		"url":            r.opts.URL,
	}
	if r.opts.Types != nil {
		sub["types"] = r.opts.Types
	}
	asked := r.expiry()
	if !asked.IsZero() {
		sub["expires"] = NewUTCDate(asked)
	}
	resp, err := r.set(ctx, map[string]any{"create": map[string]any{"push": sub}})
	if err != nil {
		return "", time.Time{}, err
	}
	created, ok := resp.Created["push"]
	if !ok || created == nil || created.ID == "" {
		if refused, ok := resp.NotCreated["push"]; ok {
			return "", time.Time{}, fmt.Errorf("jmapc: the server refused the push subscription: %w", &refused)
		}
		return "", time.Time{}, errors.New("jmapc: the server created no push subscription")
	}
	id, expires := created.ID, granted(created.Expires, asked)
	r.event(PushCreated, id, expires)

	code, err := r.awaitCode(ctx, id)
	if err != nil {
		r.destroy(id)
		return "", time.Time{}, err
	}
	resp, err = r.set(ctx, map[string]any{"update": map[ID]any{id: map[string]any{"verificationCode": code}}})
	if err != nil {
		r.destroy(id)
		return "", time.Time{}, err
	}
	if refused, ok := resp.NotUpdated[id]; ok {
		r.destroy(id)
		return "", time.Time{}, fmt.Errorf("jmapc: the server refused the verification code: %w", &refused)
	}
	r.event(PushVerified, id, expires)
	return id, expires, nil
}

// keep extends the subscription before it expires, or checks that it still
// exists where it does not expire, until ctx ends or the subscription is gone.
func (r *PushReceiver) keep(ctx context.Context, id ID, expires time.Time, lifetime time.Duration) error {
	for {
		next := r.opts.CheckInterval
		if !expires.IsZero() {
			// Four fifths of the way, which leaves a fifth of the lifetime
			// for a renewal that has to be retried. Close to the expiry
			// that would be no wait at all, so a retry waits a second.
			next = max(expires.Sub(r.now())*4/5, time.Second)
		}
		if err := r.wait(ctx, next); err != nil {
			return err
		}
		renewed, err := r.renew(ctx, id, expires, lifetime)
		switch {
		case err == nil:
			expires = renewed
		case errors.Is(err, errSubscriptionLost):
			return err
		case IsTemporary(err) && ctx.Err() == nil && (expires.IsZero() || r.now().Before(expires)):
			// Tried again at the next turn, while the subscription lasts.
		default:
			return err
		}
	}
}

// renew extends a subscription that expires, and checks one that does not,
// returning its expiry.
func (r *PushReceiver) renew(ctx context.Context, id ID, expires time.Time, lifetime time.Duration) (time.Time, error) {
	if expires.IsZero() {
		resp, err := r.call(ctx, "PushSubscription/get", map[string]any{"ids": []ID{id}, "properties": []string{"id", "expires"}})
		if err != nil {
			return time.Time{}, err
		}
		var got PushSubscriptionGetResponse
		if err := resp.Decode("push", &got); err != nil {
			return time.Time{}, err
		}
		if len(got.List) == 0 {
			return time.Time{}, errSubscriptionLost
		}
		return granted(got.List[0].Expires, time.Time{}), nil
	}
	asked := r.now().Add(lifetime)
	resp, err := r.set(ctx, map[string]any{"update": map[ID]any{id: map[string]any{"expires": NewUTCDate(asked)}}})
	if err != nil {
		return time.Time{}, err
	}
	if refused, ok := resp.NotUpdated[id]; ok {
		if refused.Type == "notFound" {
			return time.Time{}, errSubscriptionLost
		}
		return time.Time{}, fmt.Errorf("jmapc: the server refused to extend the push subscription: %w", &refused)
	}
	got := asked
	if updated := resp.Updated[id]; updated != nil {
		got = granted(updated.Expires, asked)
	}
	r.event(PushRenewed, id, got)
	return got, nil
}

// destroy removes a subscription, on a context of its own: it runs as Run
// returns, often because ctx has ended.
func (r *PushReceiver) destroy(id ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := r.set(ctx, map[string]any{"destroy": []ID{id}})
	if err != nil {
		return
	}
	// The call can succeed and the record still be refused, which is said in
	// notDestroyed; only what was destroyed is reported as destroyed.
	for _, gone := range resp.Destroyed {
		if gone == id {
			r.event(PushDestroyed, id, time.Time{})
			return
		}
	}
}

// expiry is the expiry to ask for, zero where Lifetime leaves it to the server.
func (r *PushReceiver) expiry() time.Time {
	if r.opts.Lifetime <= 0 {
		return time.Time{}
	}
	return r.now().Add(r.opts.Lifetime)
}

// granted is the expiry the server reported, or asked where it reported none,
// which is the server keeping to what it was asked.
func granted(reported *UTCDate, asked time.Time) time.Time {
	if reported != nil {
		return reported.Time
	}
	return asked
}

// set makes a PushSubscription/set call and returns its response.
func (r *PushReceiver) set(ctx context.Context, args map[string]any) (*PushSubscriptionSetResponse, error) {
	resp, err := r.call(ctx, "PushSubscription/set", args)
	if err != nil {
		return nil, err
	}
	var out PushSubscriptionSetResponse
	if err := resp.Decode("push", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// call makes one call on a PushSubscription, which belongs to the user rather
// than to an account and so names none.
func (r *PushReceiver) call(ctx context.Context, method string, args map[string]any) (*Response, error) {
	return r.c.Do(ctx, &Request{
		Using:       []string{CapabilityCore},
		MethodCalls: []Invocation{{Name: method, CallID: "push", Args: args}},
	})
}

// event tells OnEvent of a step, where it is set.
func (r *PushReceiver) event(kind PushEventKind, id ID, expires time.Time) {
	if r.opts.OnEvent != nil {
		r.opts.OnEvent(PushEvent{Kind: kind, SubscriptionID: id, Expires: expires})
	}
}
