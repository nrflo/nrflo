package clock

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	clk := Real()

	before := time.Now()
	result := clk.Now()
	after := time.Now()

	if result.Before(before) || result.After(after) {
		t.Errorf("Real().Now() = %v, expected time between %v and %v", result, before, after)
	}
}

func TestTestClock_Now(t *testing.T) {
	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := NewTest(fixedTime)

	result := clk.Now()

	if !result.Equal(fixedTime) {
		t.Errorf("TestClock.Now() = %v, want %v", result, fixedTime)
	}
}

func TestTestClock_Set(t *testing.T) {
	initialTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := NewTest(initialTime)

	newTime := time.Date(2025, 6, 15, 18, 30, 45, 0, time.UTC)
	clk.Set(newTime)

	result := clk.Now()
	if !result.Equal(newTime) {
		t.Errorf("After Set(%v), Now() = %v, want %v", newTime, result, newTime)
	}
}

func TestTestClock_Advance(t *testing.T) {
	tests := []struct {
		name     string
		initial  time.Time
		duration time.Duration
		want     time.Time
	}{
		{
			name:     "advance one second",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			duration: time.Second,
			want:     time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
		},
		{
			name:     "advance one hour",
			initial:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			duration: time.Hour,
			want:     time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC),
		},
		{
			name:     "advance one day",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			duration: 24 * time.Hour,
			want:     time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "advance milliseconds",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			duration: 1500 * time.Millisecond,
			want:     time.Date(2025, 1, 1, 0, 0, 1, 500000000, time.UTC),
		},
		{
			name:     "advance zero duration",
			initial:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			duration: 0,
			want:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "advance negative duration (backwards)",
			initial:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			duration: -6 * time.Hour,
			want:     time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := NewTest(tt.initial)
			clk.Advance(tt.duration)

			result := clk.Now()
			if !result.Equal(tt.want) {
				t.Errorf("After Advance(%v), Now() = %v, want %v", tt.duration, result, tt.want)
			}
		})
	}
}

func TestTestClock_MultipleAdvances(t *testing.T) {
	clk := NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	clk.Advance(time.Hour)
	clk.Advance(30 * time.Minute)
	clk.Advance(15 * time.Second)

	want := time.Date(2025, 1, 1, 1, 30, 15, 0, time.UTC)
	result := clk.Now()

	if !result.Equal(want) {
		t.Errorf("After multiple advances, Now() = %v, want %v", result, want)
	}
}

func TestTestClock_SetAfterAdvance(t *testing.T) {
	clk := NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	clk.Advance(time.Hour)
	clk.Set(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))

	want := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	result := clk.Now()

	if !result.Equal(want) {
		t.Errorf("After Advance then Set, Now() = %v, want %v", result, want)
	}
}

func TestTestClock_ConcurrentAccess(t *testing.T) {
	clk := NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	// Launch multiple goroutines to test thread safety
	done := make(chan bool)
	const numGoroutines = 10

	// Half advancing, half reading
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				clk.Advance(time.Millisecond)
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = clk.Now()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final time is as expected (500ms advanced)
	expected := time.Date(2025, 1, 1, 0, 0, 0, 500000000, time.UTC)
	result := clk.Now()

	if !result.Equal(expected) {
		t.Errorf("After concurrent operations, Now() = %v, want %v", result, expected)
	}
}

func TestTestClock_Format(t *testing.T) {
	// Test that TestClock.Now() returns a proper time.Time that can be formatted
	clk := NewTest(time.Date(2025, 1, 15, 14, 30, 45, 123456789, time.UTC))

	result := clk.Now().UTC().Format(time.RFC3339Nano)
	want := "2025-01-15T14:30:45.123456789Z"

	if result != want {
		t.Errorf("Now().UTC().Format(RFC3339Nano) = %v, want %v", result, want)
	}
}

func TestTestClock_DateOnly(t *testing.T) {
	// Test that TestClock works with date-only formatting (for daily_stats)
	clk := NewTest(time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC))

	result := clk.Now().UTC().Format("2006-01-02")
	want := "2025-01-15"

	if result != want {
		t.Errorf("Now().UTC().Format(date-only) = %v, want %v", result, want)
	}
}
