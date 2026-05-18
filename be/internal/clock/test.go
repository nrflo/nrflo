package clock

import (
	"sync"
	"time"
)

type timer struct {
	deadline time.Time
	ch       chan time.Time
}

// TestClock is a Clock implementation for tests that allows manual time control.
type TestClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []timer
}

// NewTest creates a TestClock set to the given time.
func NewTest(now time.Time) *TestClock {
	return &TestClock{now: now}
}

// Now returns the current test time.
func (c *TestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set sets the clock to the given time.
func (c *TestClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
	c.fireTimers()
}

// Advance moves the clock forward by the given duration.
func (c *TestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.fireTimers()
}

// After returns a channel that receives the clock's current time once d has
// elapsed on this clock (via Advance or Set crossing the deadline).
func (c *TestClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	deadline := c.now.Add(d)
	if !deadline.After(c.now) {
		ch <- c.now
	} else {
		c.timers = append(c.timers, timer{deadline: deadline, ch: ch})
	}
	return ch
}

// fireTimers fires pending timers whose deadline <= c.now. Must be called with c.mu held.
func (c *TestClock) fireTimers() {
	remaining := c.timers[:0]
	for _, t := range c.timers {
		if !t.deadline.After(c.now) {
			t.ch <- c.now
		} else {
			remaining = append(remaining, t)
		}
	}
	c.timers = remaining
}
