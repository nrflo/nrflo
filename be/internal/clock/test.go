package clock

import (
	"sync"
	"time"
)

// TestClock is a Clock implementation for tests that allows manual time control.
type TestClock struct {
	mu  sync.Mutex
	now time.Time
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
}

// Advance moves the clock forward by the given duration.
func (c *TestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
