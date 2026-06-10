package db

import (
	"errors"
	"testing"
	"time"
)

func TestIsBusy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain busy", errors.New("database is locked (5) (SQLITE_BUSY)"), true},
		{"snapshot busy", errors.New("database is locked (517) (SQLITE_BUSY_SNAPSHOT)"), true},
		{"code only", errors.New("SQLITE_BUSY"), true},
		{"other error", errors.New("UNIQUE constraint failed"), false},
	}
	for _, tc := range cases {
		if got := IsBusy(tc.err); got != tc.want {
			t.Errorf("%s: IsBusy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// stubBusySleep replaces the inter-attempt sleep for the duration of a test.
// Tests using it must not run in parallel (package-global swap).
func stubBusySleep(t *testing.T) {
	t.Helper()
	orig := busySleep
	busySleep = func(time.Duration) {}
	t.Cleanup(func() { busySleep = orig })
}

func TestWithBusyRetry_RetriesBusyThenSucceeds(t *testing.T) {
	stubBusySleep(t)
	attempts := 0
	err := WithBusyRetry(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithBusyRetry: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestWithBusyRetry_NonBusyReturnsImmediately(t *testing.T) {
	stubBusySleep(t)
	attempts := 0
	want := errors.New("UNIQUE constraint failed")
	err := WithBusyRetry(func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (non-busy errors must not retry)", attempts)
	}
}

func TestWithBusyRetry_ExhaustsAttempts(t *testing.T) {
	stubBusySleep(t)
	attempts := 0
	err := WithBusyRetry(func() error {
		attempts++
		return errors.New("SQLITE_BUSY")
	})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if attempts != 5 {
		t.Errorf("attempts = %d, want 5", attempts)
	}
}
