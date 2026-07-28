package spawner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"be/internal/service"
)

// TestShouldAdvanceChain_Guard table-drives the monotonic advance guard: only
// a HARD provider failure with at least one more chain entry beyond the
// current position is eligible.
func TestShouldAdvanceChain_Guard(t *testing.T) {
	t.Parallel()
	chain2 := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "sonnet-5"},
		{Provider: "openai", ModelID: "gpt-5.3-codex"},
	}
	tests := []struct {
		name             string
		proc             *processInfo
		wantOK           bool
		wantNextPos      int
		wantNextProvider string
	}{
		{
			name:   "no_hard_fail_never_advances",
			proc:   &processInfo{hardProviderFail: false, chain: chain2, chainPos: 0},
			wantOK: false,
		},
		{
			name:   "empty_chain_never_advances",
			proc:   &processInfo{hardProviderFail: true, chain: nil, chainPos: 0},
			wantOK: false,
		},
		{
			name:   "at_last_position_never_advances",
			proc:   &processInfo{hardProviderFail: true, chain: chain2, chainPos: 1},
			wantOK: false,
		},
		{
			name:             "mid_chain_hard_fail_advances",
			proc:             &processInfo{hardProviderFail: true, chain: chain2, chainPos: 0},
			wantOK:           true,
			wantNextPos:      1,
			wantNextProvider: "openai",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextPos, entry, ok := shouldAdvanceChain(tt.proc)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if nextPos != tt.wantNextPos {
				t.Errorf("nextPos = %d, want %d", nextPos, tt.wantNextPos)
			}
			if entry.Provider != tt.wantNextProvider {
				t.Errorf("entry.Provider = %q, want %q", entry.Provider, tt.wantNextProvider)
			}
		})
	}
}

// TestShouldAdvanceChain_MonotonicSequence simulates repeated hard fails
// advancing chainPos strictly 0->1->2 (len-1) then terminal, mirroring
// TestAutoRestart_CounterIncrementSequence's shape for the fail-restart
// counter. Also verifies that after a relaunch resets hardProviderFail, a
// same-model (non-provider) failure does NOT re-advance.
func TestShouldAdvanceChain_MonotonicSequence(t *testing.T) {
	t.Parallel()
	chain := []service.AgentChainEntry{
		{Provider: "anthropic", ModelID: "m0"},
		{Provider: "openai", ModelID: "m1"},
		{Provider: "anthropic", ModelID: "m2"},
	}
	proc := &processInfo{hardProviderFail: true, chain: chain, chainPos: 0}

	// First hard fail: advances 0 -> 1.
	nextPos, _, ok := shouldAdvanceChain(proc)
	if !ok || nextPos != 1 {
		t.Fatalf("first advance: nextPos=%d ok=%v, want 1/true", nextPos, ok)
	}
	proc.chainPos = nextPos
	proc.hardProviderFail = false // relaunch resets the flag

	// Second hard fail: advances 1 -> 2.
	proc.hardProviderFail = true
	nextPos, _, ok = shouldAdvanceChain(proc)
	if !ok || nextPos != 2 {
		t.Fatalf("second advance: nextPos=%d ok=%v, want 2/true", nextPos, ok)
	}
	proc.chainPos = nextPos
	proc.hardProviderFail = false

	// Third hard fail: chainPos==len-1, chain exhausted — no advance.
	proc.hardProviderFail = true
	if _, _, ok := shouldAdvanceChain(proc); ok {
		t.Error("at chain exhaustion: shouldAdvanceChain() = true, want false")
	}

	// Re-advance-loop guard: after landing on chainPos=2 with the flag reset
	// by a relaunch, an ordinary (non-provider) fail must never re-trigger an
	// advance — hardProviderFail stays false unless the new session itself
	// hits another HARD provider error.
	proc.hardProviderFail = false
	if _, _, ok := shouldAdvanceChain(proc); ok {
		t.Error("stale-flag guard: shouldAdvanceChain() = true with hardProviderFail=false, want false")
	}
}

// TestChainExhausted table-drives the advanced-then-ran-out predicate: only a
// proc that actually advanced (chainPos>0) and landed on the chain's last
// entry is exhausted; a proc that never advanced (nil chain, or a chain
// still at chainPos==0) is not, no matter its chain length.
func TestChainExhausted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		chain    []service.AgentChainEntry
		chainPos int
		want     bool
	}{
		{name: "nil_chain_pos0", chain: nil, chainPos: 0, want: false},
		{name: "len1_pos0_never_advanced", chain: make([]service.AgentChainEntry, 1), chainPos: 0, want: false},
		{name: "len2_pos1_exhausted", chain: make([]service.AgentChainEntry, 2), chainPos: 1, want: true},
		{name: "len3_pos2_exhausted", chain: make([]service.AgentChainEntry, 3), chainPos: 2, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := &processInfo{chain: tt.chain, chainPos: tt.chainPos}
			if got := chainExhausted(proc); got != tt.want {
				t.Errorf("chainExhausted(chain len=%d, chainPos=%d) = %v, want %v", len(tt.chain), tt.chainPos, got, tt.want)
			}
		})
	}
}

// TestChainEntryModelID verifies the "cli:model" derivation used to drive
// spawnSingle from a resolved chain entry.
func TestChainEntryModelID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry service.AgentChainEntry
		want  string
	}{
		{name: "anthropic", entry: service.AgentChainEntry{Provider: "anthropic", ModelID: "sonnet-5"}, want: "claude:sonnet-5"},
		{name: "openai", entry: service.AgentChainEntry{Provider: "openai", ModelID: "gpt-5.3-codex"}, want: "codex:gpt-5.3-codex"},
		{name: "unknown_provider_falls_back_to_claude", entry: service.AgentChainEntry{Provider: "local-ollama", ModelID: "qwen"}, want: "claude:qwen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chainEntryModelID(tt.entry); got != tt.want {
				t.Errorf("chainEntryModelID(%+v) = %q, want %q", tt.entry, got, tt.want)
			}
		})
	}
}

// TestIsProviderBuildError_And_Wrap verifies the errProviderBuild sentinel
// classification: only errors wrapped via wrapProviderBuildErr are eligible
// to advance the chain; a structural error (template load, node not found,
// unknown CLI adapter) must never be misclassified as a build error.
func TestIsProviderBuildError_And_Wrap(t *testing.T) {
	t.Parallel()

	if wrapProviderBuildErr(nil) != nil {
		t.Error("wrapProviderBuildErr(nil) != nil, want nil")
	}

	structural := fmt.Errorf("failed to load template: %w", errors.New("node not found"))
	if isProviderBuildError(structural) {
		t.Error("isProviderBuildError(structural error) = true, want false")
	}

	wrapped := wrapProviderBuildErr(errors.New("api_mode_disabled"))
	if !isProviderBuildError(wrapped) {
		t.Error("isProviderBuildError(wrapped build error) = false, want true")
	}
	if wrapped == nil || wrapped.Error() == "" {
		t.Fatalf("wrapped error is empty: %v", wrapped)
	}

	// Wrapping preserves the underlying message for diagnostics.
	if got := wrapped.Error(); !strings.Contains(got, "provider build failure") || !strings.Contains(got, "api_mode_disabled") {
		t.Errorf("wrapped.Error() = %q, want it to contain both the sentinel and underlying message", got)
	}

	// A plain errors.New (never routed through wrapProviderBuildErr) must
	// never be classified as a build error even if its text happens to
	// mention "build" — classification is signal-based (errors.Is), never
	// string matching.
	textLookalike := errors.New("provider build failure: something else")
	if isProviderBuildError(textLookalike) {
		t.Error("isProviderBuildError(plain error with build-ish text) = true, want false (must use errors.Is, not string match)")
	}
}
