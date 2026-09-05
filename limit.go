package jmapc

import (
	"context"
	"sync"
)

// limiter limits a client to the number of requests the server accepts at
// once. RFC 8620 has the server state that number in the session, and a client
// that ignores it is refused. Waiting locally for a slot costs the same as
// being refused, without the round trip or the failure.
type limiter struct {
	mu    sync.Mutex
	slots chan struct{}
}

// hold waits for a slot and returns the function that releases it. A limit of
// zero is no limit, which is what a server that states none means.
//
// The limit is read once, when the first request is held: a session fetched
// again later may state a different number, and the client keeps the one it
// started with rather than losing count of the requests already in flight.
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
