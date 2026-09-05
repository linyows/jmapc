package jmapc

import (
	"context"
	"sync"
)

// limiter holds a client to the number of requests a server said it will take
// at once. RFC 8620 has the server state that in the session, and a client
// that ignores it is asking to be refused: waiting for a slot costs the same
// as being told to wait, without the round trip or the failure.
type limiter struct {
	mu    sync.Mutex
	slots chan struct{}
}

// hold waits for a slot and returns the function that gives it back. A limit
// of zero is no limit, which is what a server saying nothing means.
//
// The limit is read once, when the first request is held: a session fetched
// again later may say something different, and the client goes on with what it
// started with rather than letting the slots in flight lose track of how many
// there are.
//
// waiting is called only when no slot is free, and the function it returns is
// called once one is. A request that acquires a slot without blocking is not
// reported.
func (l *limiter) hold(ctx context.Context, limit int, waiting func() func()) (func(), error) {
	if limit <= 0 {
		return func() {}, nil
	}
	l.mu.Lock()
	if l.slots == nil {
		l.slots = make(chan struct{}, limit)
	}
	slots := l.slots
	l.mu.Unlock()

	release := func() { <-slots }
	select {
	case slots <- struct{}{}:
		return release, nil
	default:
	}

	held := waiting()
	defer held()
	select {
	case slots <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// limit returns what the session says about a kind of request, and zero where
// it says nothing or where there is no session yet. It never fetches one: a
// client told not to check the session before sending is not made to fetch it
// to count.
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
