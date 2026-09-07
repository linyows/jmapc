package jmapc

import (
	"context"
	"sync"
)

// limiter limits a client to the number of requests the server accepts at
// once. RFC 8620 has the server state that number in the session, and a client
// that ignores it is refused with a 400, which no retry policy sends again.
// Waiting locally for a slot costs the same as being refused, without the round
// trip or the failure.
//
// The number is read on every hold, so that a session fetched again with a
// different number takes effect from the next request. A number that has gone
// down does not cancel the requests already in flight: they return their slots
// as they finish, and no further slot is given out until fewer than the new
// number are held. The count therefore never rises above the number the server
// last stated, which is what makes following a change worth doing at all.
//
// Slots are given out in the order they were asked for, so a request does not
// wait behind one that arrived after it.
type limiter struct {
	mu sync.Mutex
	// limit is the number of slots the server allows, and zero is no limit.
	limit int
	// held counts the slots given out and not yet returned.
	held int
	// waiting holds one channel per caller waiting for a slot, oldest first.
	// The channel is closed by whoever hands the slot over, which has counted
	// it in held already.
	waiting []chan struct{}
}

// hold waits for a slot and returns the function that releases it. A limit of
// zero is no limit, which is what a server that states none means.
//
// report is called only when no slot is free, and the function it returns is
// called once one is. A request that takes a slot without waiting is not
// reported.
func (l *limiter) hold(ctx context.Context, limit int, report func() func()) (func(), error) {
	l.mu.Lock()
	l.limit = limit
	// A limit that has gone up is a slot for whoever has been waiting longest,
	// not for whoever asked last.
	l.handOut()
	if len(l.waiting) == 0 && l.free() {
		l.held++
		l.mu.Unlock()
		return l.release, nil
	}
	slot := make(chan struct{})
	l.waiting = append(l.waiting, slot)
	l.mu.Unlock()

	held := report()
	defer held()
	select {
	case <-slot:
		return l.release, nil
	case <-ctx.Done():
		l.abandon(slot)
		return nil, ctx.Err()
	}
}

// free reports whether a slot may be given out. The caller holds mu.
func (l *limiter) free() bool {
	return l.limit <= 0 || l.held < l.limit
}

// release returns a slot and passes it to whoever is waiting for one.
func (l *limiter) release() {
	l.mu.Lock()
	l.held--
	l.handOut()
	l.mu.Unlock()
}

// handOut gives slots to the callers waiting for one, oldest first, for as long
// as the limit allows. The caller holds mu.
func (l *limiter) handOut() {
	for len(l.waiting) > 0 && l.free() {
		slot := l.waiting[0]
		l.waiting = l.waiting[1:]
		l.held++
		close(slot)
	}
}

// abandon drops a caller whose context ended before it was given a slot. Where
// it was given one just as the context ended, the slot is returned rather than
// held for a request that is no longer going to be sent.
func (l *limiter) abandon(slot chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, w := range l.waiting {
		if w == slot {
			l.waiting = append(l.waiting[:i], l.waiting[i+1:]...)
			return
		}
	}
	l.held--
	l.handOut()
}

// limit returns the session's limit for a kind of request, and zero where the
// session states none or where there is no session yet. It never fetches one:
// a client configured not to check the session before sending is not forced to
// fetch it in order to count.
func (c *Client) limit(pick func(*CoreCapability) UnsignedInt) int {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return 0
	}
	core, err := session.Core()
	if err != nil {
		return 0
	}
	return int(pick(core))
}
