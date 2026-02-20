package usagelimits

import (
	"sync"
	"testing"
	"time"
)

func TestCache_GetEmpty(t *testing.T) {
	c := NewCache()
	got := c.Get()
	if got != nil {
		t.Errorf("Get() on empty cache = %+v, want nil", got)
	}
}

func TestCache_SetThenGet(t *testing.T) {
	c := NewCache()
	data := &UsageLimits{
		Claude: ToolUsage{
			Available: true,
			Session:   &UsageMetric{UsedPct: 45.2, ResetsAt: "in 2h"},
		},
		Codex:     ToolUsage{Available: false},
		FetchedAt: time.Now(),
	}

	c.Set(data)
	got := c.Get()

	if got == nil {
		t.Fatal("Get() after Set() returned nil")
	}
	if got != data {
		t.Errorf("Get() = %p, want %p (same pointer)", got, data)
	}
	if got.Claude.Session == nil {
		t.Fatal("Claude.Session is nil")
	}
	if got.Claude.Session.UsedPct != 45.2 {
		t.Errorf("Claude.Session.UsedPct = %v, want 45.2", got.Claude.Session.UsedPct)
	}
}

func TestCache_SetNil(t *testing.T) {
	c := NewCache()
	data := &UsageLimits{FetchedAt: time.Now()}
	c.Set(data)
	c.Set(nil)
	got := c.Get()
	if got != nil {
		t.Errorf("Get() after Set(nil) = %+v, want nil", got)
	}
}

func TestCache_OverwriteWithNewData(t *testing.T) {
	c := NewCache()

	first := &UsageLimits{Claude: ToolUsage{Available: true}}
	second := &UsageLimits{Claude: ToolUsage{Available: false}}

	c.Set(first)
	c.Set(second)

	got := c.Get()
	if got != second {
		t.Errorf("Get() = %p, want second value %p", got, second)
	}
	if got.Claude.Available {
		t.Error("Claude.Available should be false after second Set()")
	}
}

func TestCache_Concurrent(t *testing.T) {
	// Run with -race to verify no data races.
	c := NewCache()

	var wg sync.WaitGroup
	const goroutines = 50

	// Writers
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Set(&UsageLimits{
				Claude:    ToolUsage{Available: i%2 == 0},
				FetchedAt: time.Now(),
			})
		}(i)
	}

	// Readers
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Get()
		}()
	}

	wg.Wait()
	// No assertions needed — race detector catches concurrent access violations.
}

func TestCache_ConcurrentReadsDontBlock(t *testing.T) {
	c := NewCache()
	data := &UsageLimits{Claude: ToolUsage{Available: true}}
	c.Set(data)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := c.Get()
			if got == nil {
				t.Errorf("concurrent Get() returned nil")
			}
		}()
	}
	wg.Wait()
}
