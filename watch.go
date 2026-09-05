package jmapc

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// CatchUp fetches what changed since a state and reports the resulting state,
// together with whether the server has more changes to report. Watch calls it,
// and calls it again while more is true, because a server answers a /changes
// call with as many changes as it chooses rather than with all of them.
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

// WithPing requests a comment from the server at that interval, so that a
// connection dropped by an intermediary is detected rather than left hanging.
// Servers clamp it to a range of their own.
func WithPing(d time.Duration) WatchOption {
	return func(c *watchConfig) { c.ping = d }
}

// WithReconnect sets how long to wait before the nth attempt to reconnect,
// counting from one. It replaces the doubling delay Watch uses otherwise, and
// is where to add jitter for a group of clients that would otherwise reconnect
// at the same time.
func WithReconnect(f func(attempt int) time.Duration) WatchOption {
	return func(c *watchConfig) { c.retry = f }
}

// Default settings for a watch: request a ping every 30 seconds, and delay
// between reconnection attempts by doubling from 1 second to 30 seconds.
const (
	defaultWatchPing = 30 * time.Second
	minWatchRetry    = time.Second
	maxWatchRetry    = 30 * time.Second
)

// Watch follows one type's changes in one account, for as long as the context
// lasts.
//
// A push event reports only that a type has changed, not what changed, so
// every client that follows changes writes the same loop: connect, request the
// changes since the state it holds, apply them, and wait for the next event.
// The parts of that loop which are easy to get wrong are implemented here
// rather than by the caller. A stream is a connection and not a subscription,
// so a dropped one is reopened, resuming from the last event it delivered,
// with a delay that doubles while the server is unreachable. Every connection
// is followed by a catch-up, because changes made while no connection was open
// were not pushed. And a server that returns only part of what changed is
// asked again until it reports no more.
//
// state is where the loop starts: the state a previous /get or /changes
// reported, which the caller holds alongside the records it fetched then.
//
// It returns the context's error when the context ends, the caller's error when
// catchUp fails, and a *RequestError when the server refuses the connection for
// a reason that retrying will not resolve.
func (c *Client) Watch(ctx context.Context, accountID ID, typeName, state string, catchUp CatchUp, opts ...WatchOption) error {
	cfg := watchConfig{ping: defaultWatchPing}
	for _, opt := range opts {
		opt(&cfg)
	}

	// A /changes call returns as many changes as the server chooses, so
	// catching up is a loop of its own.
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

		// Changes made while no connection was open were not pushed, so a new
		// connection starts with a catch-up.
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
				// Either the event is about another account or type, or it
				// reports a state this loop has already reached, which a
				// catch-up of its own causes the server to push.
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

// permanent reports whether an error will persist after a delay. A server that
// refused the request reported something about the request itself; one that is
// unreachable, overloaded, or broken did not.
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

// backoff returns the delay before the nth attempt, doubling up to a maximum
// so that a server that is down is not polled every second for the whole of
// its downtime.
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
