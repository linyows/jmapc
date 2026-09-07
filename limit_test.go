package jmapc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// counter follows how many slots a limiter has given out at once, so that a
// test can say what the highest number was.
type counter struct {
	now  atomic.Int64
	high atomic.Int64
}

func (c *counter) enter() {
	now := c.now.Add(1)
	for {
		high := c.high.Load()
		if now <= high || c.high.CompareAndSwap(high, now) {
			return
		}
	}
}

func (c *counter) leave() { c.now.Add(-1) }

// noReport is the callback a limiter is given where a test does not care
// whether a hold had to wait.
func noReport() func() { return func() {} }

// held takes a slot, or fails the test where none arrives in time. It returns
// the function that releases it.
func held(t *testing.T, l *limiter, limit int) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := l.hold(ctx, limit, noReport)
	if err != nil {
		t.Fatalf("waiting for a slot under a limit of %d: %v", limit, err)
	}
	return release
}

// waits reports whether a hold is still waiting after a moment. It is how a
// test says "this one does not get through", which cannot be observed directly.
func waits(l *limiter, limit int) (blocked bool, release func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := l.hold(ctx, limit, noReport)
	if err != nil {
		return true, nil
	}
	return false, got
}

func TestLimiterGivesOutNoMoreSlotsThanTheLimit(t *testing.T) {
	l := &limiter{}
	var seen counter
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := held(t, l, 2)
			seen.enter()
			time.Sleep(5 * time.Millisecond)
			seen.leave()
			release()
		}()
	}
	wg.Wait()
	if high := seen.high.Load(); high > 2 {
		t.Errorf("%d slots were held at once, and the limit is 2", high)
	}
}

// TestLimiterFollowsALimitThatGoesDown is what a session fetched again with a
// smaller maxConcurrentRequests comes to. The requests already in flight are
// not cancelled, and no further slot is given out until enough of them have
// finished, so the client never has more in flight than the server last said
// it would take.
func TestLimiterFollowsALimitThatGoesDown(t *testing.T) {
	l := &limiter{}
	releases := make([]func(), 0, 5)
	for range 5 {
		releases = append(releases, held(t, l, 5))
	}

	// The server now says two, with five in flight. Nothing more goes out.
	for i := range 3 {
		if blocked, release := waits(l, 2); !blocked {
			release()
			t.Fatalf("a slot was given out with %d in flight and a limit of 2", 5-i)
		}
		releases[i]()
	}
	// Two are in flight, which is the limit, so a hold still waits.
	if blocked, release := waits(l, 2); !blocked {
		release()
		t.Fatal("a slot was given out with two in flight and a limit of 2")
	}
	// One returns, and the next hold goes through.
	releases[3]()
	release := held(t, l, 2)
	release()
	releases[4]()
}

// TestLimiterFollowsALimitThatGoesUp covers the other direction: the slots a
// larger limit allows are given to whoever has been waiting longest.
func TestLimiterFollowsALimitThatGoesUp(t *testing.T) {
	l := &limiter{}
	first := held(t, l, 1)

	waiting := make(chan func(), 1)
	go func() {
		waiting <- held(t, l, 1)
	}()
	select {
	case <-waiting:
		t.Fatal("a second slot was given out under a limit of one")
	case <-time.After(20 * time.Millisecond):
	}

	// A request arriving with the larger limit lets the one already waiting
	// through, rather than taking the slot for itself.
	release := held(t, l, 3)
	select {
	case got := <-waiting:
		got()
	case <-time.After(time.Second):
		t.Fatal("the caller that was waiting did not get a slot under the larger limit")
	}
	release()
	first()
}

func TestLimiterHandsSlotsOutInTheOrderTheyWereAskedFor(t *testing.T) {
	l := &limiter{}
	occupied := held(t, l, 1)

	var mu sync.Mutex
	var order []int
	var started, done sync.WaitGroup
	for i := range 5 {
		started.Add(1)
		done.Add(1)
		go func() {
			started.Done()
			release := held(t, l, 1)
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			release()
			done.Done()
		}()
		// Each caller reaches the limiter before the next one is started, so
		// the order they asked in is known.
		started.Wait()
		time.Sleep(5 * time.Millisecond)
	}
	occupied()
	done.Wait()

	mu.Lock()
	defer mu.Unlock()
	for i, got := range order {
		if got != i {
			t.Fatalf("the slots went out in the order %v, want them in the order they were asked for", order)
		}
	}
}

// TestLimiterKeepsNoSlotForACancelledWait covers the caller that gives up. Its
// slot has to go back, whether it was still waiting or had just been given one,
// or the client loses a slot for good.
func TestLimiterKeepsNoSlotForACancelledWait(t *testing.T) {
	l := &limiter{}
	occupied := held(t, l, 1)

	for range 20 {
		ctx, cancel := context.WithCancel(context.Background())
		go cancel()
		if release, err := l.hold(ctx, 1, noReport); err == nil {
			// The slot arrived as the context ended, which is allowed; what
			// matters is that it is returned.
			release()
		}
	}
	occupied()

	// Every slot is back, so the next caller does not wait.
	release := held(t, l, 1)
	release()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held != 0 {
		t.Errorf("%d slots are still held, want none", l.held)
	}
	if len(l.waiting) != 0 {
		t.Errorf("%d callers are still waiting, want none", len(l.waiting))
	}
}

func TestLimiterWithoutALimitLetsEverythingThrough(t *testing.T) {
	l := &limiter{}
	releases := make([]func(), 0, 50)
	for range 50 {
		releases = append(releases, held(t, l, 0))
	}
	for _, release := range releases {
		release()
	}
}

// TestLimiterReportsOnlyTheWaits checks that taking a free slot is not reported
// as a wait, since a report of a wait that did not happen is worse than none.
func TestLimiterReportsOnlyTheWaits(t *testing.T) {
	l := &limiter{}
	var reports atomic.Int64
	report := func() func() {
		reports.Add(1)
		return func() {}
	}

	release, err := l.hold(context.Background(), 1, report)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if got := reports.Load(); got != 0 {
		t.Errorf("a free slot was reported as a wait %d times", got)
	}

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		second, err := l.hold(context.Background(), 1, report)
		if err != nil {
			t.Errorf("hold: %v", err)
			return
		}
		second()
	}()
	time.Sleep(20 * time.Millisecond)
	release()
	<-waited

	if got := reports.Load(); got != 1 {
		t.Errorf("the wait was reported %d times, want once", got)
	}
}

// TestTheClientFollowsAConcurrencyLimitThatChanged is the two halves together:
// the session is fetched again because a response reported it changed, and the
// number of requests the client keeps in flight follows what the new session
// states. Before this the limit was the one read at the first request, for the
// life of the client.
func TestTheClientFollowsAConcurrencyLimitThatChanged(t *testing.T) {
	ts := newStateServer(t)
	ts.concurrent.Store(4)
	ts.apiDelay.Store(20)
	c := ts.client()

	send := func(n int) {
		t.Helper()
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.Do(context.Background(), &Request{Using: []string{CapabilityCore}}); err != nil {
					t.Errorf("Do: %v", err)
				}
			}()
		}
		wg.Wait()
	}

	send(8)
	if high := ts.inFlightHigh.Load(); high > 4 {
		t.Fatalf("%d requests were in flight at once under a limit of 4", high)
	}
	if high := ts.inFlightHigh.Load(); high < 2 {
		t.Fatalf("the requests did not overlap, so the test says nothing: %d at once", high)
	}

	// The server now allows one at a time, and says so under a new state.
	ts.concurrent.Store(1)
	ts.change("sess2", "a1")
	send(1)
	ts.inFlightHigh.Store(0)

	send(8)
	if high := ts.inFlightHigh.Load(); high > 1 {
		t.Errorf("%d requests were in flight at once after the limit went down to 1", high)
	}
}
