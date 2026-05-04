package api

import (
	"testing"
	"time"
)

func TestRateBucket_AllowsExactlyMax(t *testing.T) {
	b := &rateBucket{}
	now := time.Now()
	for i := 0; i < rateLimitMax; i++ {
		ok, _ := b.tryAcquire(now)
		if !ok {
			t.Fatalf("attempt %d: expected ok=true, got false", i+1)
		}
	}
}

func TestRateBucket_BlocksOnExceed(t *testing.T) {
	b := &rateBucket{}
	now := time.Now()
	for i := 0; i < rateLimitMax; i++ {
		b.tryAcquire(now)
	}
	ok, retryAfter := b.tryAcquire(now)
	if ok {
		t.Fatal("expected ok=false after exceeding max, got true")
	}
	if retryAfter <= 0 || retryAfter > rateLimitWindow {
		t.Errorf("retryAfter = %v, want in (0, %v]", retryAfter, rateLimitWindow)
	}
}

func TestRateBucket_ResetsAfterWindow(t *testing.T) {
	b := &rateBucket{}
	now := time.Now()
	for i := 0; i < rateLimitMax; i++ {
		b.tryAcquire(now)
	}
	// Confirm it's blocked at now.
	ok, _ := b.tryAcquire(now)
	if ok {
		t.Fatal("expected blocked before window reset")
	}

	// One second past the window: old attempts are pruned.
	future := now.Add(rateLimitWindow + time.Second)
	ok, _ = b.tryAcquire(future)
	if !ok {
		t.Fatal("expected ok=true after window expired, got false")
	}
}

func TestRateBucket_RetryAfterValue(t *testing.T) {
	b := &rateBucket{}
	base := time.Now()
	for i := 0; i < rateLimitMax; i++ {
		b.tryAcquire(base)
	}

	// Call 2 minutes into the window.
	callAt := base.Add(2 * time.Minute)
	ok, retryAfter := b.tryAcquire(callAt)
	if ok {
		t.Fatal("expected blocked")
	}

	// Earliest attempt expires at base+rateLimitWindow. Remaining = base+5min - (base+2min) = 3min.
	want := 3 * time.Minute
	diff := retryAfter - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("retryAfter = %v, want ~%v (±2s)", retryAfter, want)
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	l := newLoginRateLimiter()

	// Exhaust keyA.
	for i := 0; i < rateLimitMax; i++ {
		l.TryAcquire("keyA")
	}
	okA, _ := l.TryAcquire("keyA")
	if okA {
		t.Fatal("keyA should be exhausted")
	}

	// keyB is independent and should still allow.
	okB, _ := l.TryAcquire("keyB")
	if !okB {
		t.Fatal("keyB should be allowed (independent key)")
	}
}

func TestRateLimiter_SameKeyExhausted(t *testing.T) {
	l := newLoginRateLimiter()
	for i := 0; i < rateLimitMax; i++ {
		ok, _ := l.TryAcquire("shared")
		if !ok {
			t.Fatalf("attempt %d: expected ok=true", i+1)
		}
	}
	ok, _ := l.TryAcquire("shared")
	if ok {
		t.Fatal("expected ok=false on attempt beyond max")
	}
}

func TestRateLimitKey_Formats(t *testing.T) {
	cases := []struct {
		remoteAddr string
		email      string
		want       string
	}{
		{"127.0.0.1:1234", "a@b.com", "127.0.0.1|a@b.com"},
		{"[::1]:9000", "x@y.z", "::1|x@y.z"},
		{"badaddr", "e@f.g", "badaddr|e@f.g"},
	}
	for _, tc := range cases {
		got := rateLimitKey(tc.remoteAddr, tc.email)
		if got != tc.want {
			t.Errorf("rateLimitKey(%q, %q) = %q, want %q", tc.remoteAddr, tc.email, got, tc.want)
		}
	}
}
