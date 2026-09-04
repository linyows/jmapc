package jmapc

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// CatchUp fetches what changed since a state and reports where that leaves the
// caller: the state it has now reached, and whether the server says there is
// still more to come. Watch calls it, and calls it again while more is true,
// because a server answers a /changes call with as much as it cares to and says
// so rather than sending everything.
//
// An error stops the watch and is what Watch returns.
type CatchUp func(ctx context.Context, sinceState string) (newState string, more bool, err error)

// WatchOption tunes a watch.
type WatchOption func(*watchConfig)

// watchConfig holds the settings a watch runs with.
type watchConfig struct {
	ping  time.Duration
	retry func(attempt int) time.Duration
}

// WithPing asks the server to send a comment every so often, so that a
// connection something in the middle has dropped is noticed rather than
// hanging. Servers clamp it to a range of their own choosing.
func WithPing(d time.Duration) WatchOption {
	return func(c *watchConfig) { c.ping = d }
}

// WithRetry decides how long to wait before the nth attempt to reconnect,
// counting from one. It replaces the doubling wait that Watch uses otherwise,
// and is where to add jitter for a fleet of clients that would otherwise
// reconnect together.
func WithRetry(f func(attempt int) time.Duration) WatchOption {
	return func(c *watchConfig) { c.retry = f }
}

// Default settings for a watch: ask for a ping every half minute, and back off
// from a second to half a minute between attempts to reconnect.
const (
	defaultWatchPing = 30 * time.Second
	minWatchRetry    = time.Second
	maxWatchRetry    = 30 * time.Second
)

// Watch follows one type's changes in one account, for as long as the context
// lasts.
//
// A push event says only that a type has moved on, not what changed, so this is
// the shape every client that follows changes ends up writing: connect, ask
// what changed since the state you hold, apply it, and wait to be told again.
// The parts that are easy to get wrong are here rather than in the caller. A
// stream is a connection and not a subscription, so a dropped one is reopened,
// resuming from the last event it delivered, with a wait that doubles while the
// server is unreachable. Every connection is followed by a catch-up, since what
// happened while there was none was never pushed to anybody. And a server that
// answers with only part of what changed is asked again until it says there is
// no more.
//
// state is where the loop starts: the state a previous /get or /changes
// reported, which the caller holds alongside the records it fetched then.
//
// It returns the context's error when the context ends, the caller's error when
// catchUp fails, and a *RequestError when the server refuses the connection for
// a reason that waiting will not fix.
func (c *Client) Watch(ctx context.Context, accountID ID, typeName, state string, catchUp CatchUp, opts ...WatchOption) error {
	cfg := watchConfig{ping: defaultWatchPing}
	for _, opt := range opts {
		opt(&cfg)
	}

	// A /changes call answers with as much as it chooses to, so catching up is
	// a loop of its own.
	settle := func() error {
		for {
			newState, more, err := catchUp(ctx, state)
			if err != nil {
				return err
			}
			if newState != "" {
				state = newState
			}
			if !more {
				return nil
			}
		}
	}

	var lastEventID string
	attempt := 0
	for {
		stream, err := c.EventSource(ctx, &EventSourceOptions{
			Types:       []string{typeName},
			Ping:        cfg.ping,
			LastEventID: lastEventID,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if permanent(err) {
				return err
			}
			attempt++
			if err := wait(ctx, cfg.retry, attempt); err != nil {
				return err
			}
			continue
		}
		attempt = 0

		// Whatever changed while there was no connection was pushed to nobody,
		// so the first thing a new connection does is ask.
		if err := settle(); err != nil {
			stream.Close()
			return err
		}

		for {
			change, err := stream.Next()
			if err != nil {
				break
			}
			pushed, ok := change.StateOf(accountID, typeName)
			if !ok || pushed == state {
				// Either the event is about somebody else, or it is about a
				// state this loop has already reached: a catch-up of its own
				// makes the server push what it has just been told about.
				continue
			}
			if err := settle(); err != nil {
				stream.Close()
				return err
			}
		}

		lastEventID = stream.LastEventID()
		stream.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempt++
		if err := wait(ctx, cfg.retry, attempt); err != nil {
			return err
		}
	}
}

// permanent reports whether an error will still be there after a wait. A
// server that refused the request is telling the client something about the
// request; one that is unreachable, overloaded, or broken is not.
func permanent(err error) bool {
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		return false
	}
	return reqErr.Status >= 400 && reqErr.Status < 500 &&
		reqErr.Status != http.StatusTooManyRequests
}

// wait sleeps before the nth attempt, or returns as soon as the context ends.
func wait(ctx context.Context, retry func(int) time.Duration, attempt int) error {
	d := backoff(attempt)
	if retry != nil {
		d = retry(attempt)
	}
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoff returns the wait before the nth attempt, doubling up to a ceiling so
// that a server that is down is not asked every second for as long as it is.
func backoff(attempt int) time.Duration {
	d := minWatchRetry
	for i := 1; i < attempt && d < maxWatchRetry; i++ {
		d *= 2
	}
	if d > maxWatchRetry {
		return maxWatchRetry
	}
	return d
}
