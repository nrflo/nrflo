package spawner

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestComputeRateLimitDelay_BackoffSequence verifies exponential doubling: 60→120→240.
func TestComputeRateLimitDelay_BackoffSequence(t *testing.T) {
	t.Parallel()
	cfg := rateLimitConfig{
		InitialBackoff: 60 * time.Second,
		MaxWait:        3600 * time.Second,
	}
	want := []time.Duration{
		60 * time.Second,  // retry 1: 60 * 2^0
		120 * time.Second, // retry 2: 60 * 2^1
		240 * time.Second, // retry 3: 60 * 2^2
		480 * time.Second, // retry 4: 60 * 2^3
		960 * time.Second, // retry 5: 60 * 2^4
	}
	for i, w := range want {
		n := i + 1
		got := computeRateLimitDelay(cfg, n)
		if got != w {
			t.Errorf("computeRateLimitDelay(cfg, %d) = %v, want %v", n, got, w)
		}
	}
}

// TestComputeRateLimitDelay_CapsAtMaxWait verifies the upper bound is respected.
func TestComputeRateLimitDelay_CapsAtMaxWait(t *testing.T) {
	t.Parallel()
	cfg := rateLimitConfig{
		InitialBackoff: 60 * time.Second,
		MaxWait:        300 * time.Second, // cap below retry-4 natural value (480s)
	}
	// retry 4: 60*8=480 > 300 → capped at 300
	if got := computeRateLimitDelay(cfg, 4); got != 300*time.Second {
		t.Errorf("computeRateLimitDelay capped: got %v, want 300s", got)
	}
	// Large retry counts all return MaxWait.
	if got := computeRateLimitDelay(cfg, 100); got != 300*time.Second {
		t.Errorf("computeRateLimitDelay(100): got %v, want 300s (MaxWait)", got)
	}
}

// TestComputeRateLimitDelay_ZeroNegativeRetryTreatedAsOne ensures retryCount ≤0 → 1.
func TestComputeRateLimitDelay_ZeroNegativeRetryTreatedAsOne(t *testing.T) {
	t.Parallel()
	cfg := rateLimitConfig{
		InitialBackoff: 60 * time.Second,
		MaxWait:        3600 * time.Second,
	}
	for _, n := range []int{0, -1, -100} {
		if got := computeRateLimitDelay(cfg, n); got != 60*time.Second {
			t.Errorf("computeRateLimitDelay(cfg, %d) = %v, want 60s", n, got)
		}
	}
}

// TestComputeRateLimitDelay_LargeShiftNoOverflow verifies shift≥32 returns MaxWait without overflow.
func TestComputeRateLimitDelay_LargeShiftNoOverflow(t *testing.T) {
	t.Parallel()
	cfg := rateLimitConfig{
		InitialBackoff: 60 * time.Second,
		MaxWait:        3600 * time.Second,
	}
	// shift = retryCount-1 = 32 → should return MaxWait without integer overflow.
	got := computeRateLimitDelay(cfg, 33)
	if got != 3600*time.Second {
		t.Errorf("computeRateLimitDelay large shift: got %v, want 3600s", got)
	}
}

// TestSplitConfigPatterns_Variants covers trimming, empty removal, and edge cases.
func TestSplitConfigPatterns_Variants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
		{"single", []string{"single"}},
		{"", []string{}},
		{" , , ", []string{}},
		{"rate limit,quota exceeded", []string{"rate limit", "quota exceeded"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := splitConfigPatterns(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitConfigPatterns(%q): len=%d want=%d; got=%v", tt.input, len(got), len(tt.want), got)
				return
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// TestProcessInfo_RecentRingBuffer verifies appendRecent caps at 10 and evicts oldest.
func TestProcessInfo_RecentRingBuffer(t *testing.T) {
	t.Parallel()
	p := &processInfo{}
	for i := 0; i < 15; i++ {
		p.appendRecent(fmt.Sprintf("block-%d", i))
	}
	if len(p.recentBlocks) != 10 {
		t.Errorf("recentBlocks len = %d, want 10", len(p.recentBlocks))
	}
	tail := p.recentTail()
	// Blocks 5-14 retained; 0-4 evicted.
	for i := 5; i < 15; i++ {
		if !strings.Contains(tail, fmt.Sprintf("block-%d", i)) {
			t.Errorf("recentTail should contain block-%d", i)
		}
	}
	for i := 0; i < 5; i++ {
		// block-1 is a substring of block-10..block-14; check via exact "block-0\n" etc.
		if strings.Contains(tail, fmt.Sprintf("block-%d\n", i)) || tail == fmt.Sprintf("block-%d", i) {
			t.Errorf("recentTail should not contain evicted block-%d", i)
		}
	}
}

// TestProcessInfo_StderrRingBuffer verifies appendStderr caps at 10.
func TestProcessInfo_StderrRingBuffer(t *testing.T) {
	t.Parallel()
	p := &processInfo{}
	for i := 0; i < 12; i++ {
		p.appendStderr(fmt.Sprintf("err-%d", i))
	}
	if len(p.stderrBlocks) != 10 {
		t.Errorf("stderrBlocks len = %d, want 10", len(p.stderrBlocks))
	}
	// Most recent 10: err-2 through err-11.
	tail := p.stderrTail()
	if !strings.Contains(tail, "err-11") {
		t.Error("stderrTail should contain err-11 (most recent)")
	}
	if strings.Contains(tail, "err-0\n") {
		t.Error("stderrTail should not contain err-0 (evicted)")
	}
}

// TestProcessInfo_AppendRecent_EmptySkipped verifies empty strings are not appended.
func TestProcessInfo_AppendRecent_EmptySkipped(t *testing.T) {
	t.Parallel()
	p := &processInfo{}
	p.appendRecent("")
	p.appendRecent("real content")
	p.appendRecent("")
	if len(p.recentBlocks) != 1 {
		t.Errorf("recentBlocks len = %d, want 1 (empty entries skipped)", len(p.recentBlocks))
	}
	if p.recentBlocks[0] != "real content" {
		t.Errorf("recentBlocks[0] = %q, want %q", p.recentBlocks[0], "real content")
	}
}

// TestProcessInfo_AppendStderr_EmptySkipped verifies empty strings are not appended to stderr.
func TestProcessInfo_AppendStderr_EmptySkipped(t *testing.T) {
	t.Parallel()
	p := &processInfo{}
	p.appendStderr("")
	p.appendStderr("stderr line")
	p.appendStderr("")
	if len(p.stderrBlocks) != 1 {
		t.Errorf("stderrBlocks len = %d, want 1", len(p.stderrBlocks))
	}
}

// TestRecentTail_JoinsWithNewline verifies recentTail joins blocks with "\n".
func TestRecentTail_JoinsWithNewline(t *testing.T) {
	t.Parallel()
	p := &processInfo{}
	p.appendRecent("alpha")
	p.appendRecent("beta")
	got := p.recentTail()
	if got != "alpha\nbeta" {
		t.Errorf("recentTail = %q, want %q", got, "alpha\nbeta")
	}
}
