package spawner

import (
	"testing"
	"time"
)

// TestResetAwareDelay verifies a known future subscription reset drives the wait,
// with fallbacks to exponential backoff for missing/past/far/unparseable resets.
func TestResetAwareDelay(t *testing.T) {
	cfg := rateLimitConfig{InitialBackoff: 60 * time.Second, MaxWait: 3600 * time.Second}
	now := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	// Known future reset within cap → wait until reset + buffer.
	reset := now.Add(90 * time.Minute).Format(time.RFC3339)
	if got, want := resetAwareDelay(cfg, 1, reset, now), 90*time.Minute+rateLimitResetBuffer; got != want {
		t.Errorf("future reset: got %v, want %v", got, want)
	}

	// No reset → exponential backoff (retry 1 = InitialBackoff).
	if got := resetAwareDelay(cfg, 1, "", now); got != cfg.InitialBackoff {
		t.Errorf("no reset: got %v, want %v", got, cfg.InitialBackoff)
	}

	// Reset in the past → fall back to backoff for the given retry count.
	past := now.Add(-time.Hour).Format(time.RFC3339)
	if got, want := resetAwareDelay(cfg, 2, past, now), computeRateLimitDelay(cfg, 2); got != want {
		t.Errorf("past reset: got %v, want backoff %v", got, want)
	}

	// Reset beyond the absolute cap → fall back to backoff.
	far := now.Add(rateLimitResetAbsCap + time.Hour).Format(time.RFC3339)
	if got := resetAwareDelay(cfg, 1, far, now); got != cfg.InitialBackoff {
		t.Errorf("far reset: got %v, want %v", got, cfg.InitialBackoff)
	}

	// Unparseable reset → backoff.
	if got := resetAwareDelay(cfg, 1, "garbage", now); got != cfg.InitialBackoff {
		t.Errorf("garbage reset: got %v, want %v", got, cfg.InitialBackoff)
	}
}
