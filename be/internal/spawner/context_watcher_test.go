package spawner

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// TestResolveContextBudget covers the nullable-per-def + global-default +
// 0-disabled resolution matrix: a non-nil def override always wins (0 =
// disabled, >0 = budget); nil def or a nil ContextBudgetTokens falls through
// to defaultBudget.
func TestResolveContextBudget(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	cases := []struct {
		name          string
		def           *model.AgentDefinition
		defaultBudget int
		want          int
	}{
		{"nil def falls through to default", nil, 500, 500},
		{"def with nil override falls through to default", &model.AgentDefinition{}, 500, 500},
		{"def override zero disables regardless of default", &model.AgentDefinition{ContextBudgetTokens: intPtr(0)}, 500, 0},
		{"def override positive wins over default", &model.AgentDefinition{ContextBudgetTokens: intPtr(750)}, 500, 750},
		{"nil def, zero default stays disabled", nil, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveContextBudget(tc.def, tc.defaultBudget); got != tc.want {
				t.Errorf("resolveContextBudget() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestContextConfigInt_NilPool_ReturnsFallback verifies a nil pool never
// touches the DB and always returns the fallback.
func TestContextConfigInt_NilPool_ReturnsFallback(t *testing.T) {
	if got := contextConfigInt(nil, "context_decay_turns", 20); got != 20 {
		t.Errorf("contextConfigInt(nil pool) = %d, want fallback 20", got)
	}
}

// TestContextConfigInt_ReadsSeededValue verifies a real config row overrides
// the fallback.
func TestContextConfigInt_ReadsSeededValue(t *testing.T) {
	pool := setupTestDB(t)
	if err := pool.SetConfig("context_decay_turns", "42"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := contextConfigInt(pool, "context_decay_turns", 20); got != 42 {
		t.Errorf("contextConfigInt() = %d, want 42 (seeded value)", got)
	}
}

// TestContextConfigInt_UnsetKey_ReturnsFallback verifies an unset key (no
// row) falls back rather than erroring.
func TestContextConfigInt_UnsetKey_ReturnsFallback(t *testing.T) {
	pool := setupTestDB(t)
	if got := contextConfigInt(pool, "cache_ttl_sec", 300); got != 300 {
		t.Errorf("contextConfigInt(unset key) = %d, want fallback 300", got)
	}
}

// TestContextConfigInt_UnparseableValue_ReturnsFallback verifies a malformed
// config value falls back instead of propagating a parse error.
func TestContextConfigInt_UnparseableValue_ReturnsFallback(t *testing.T) {
	pool := setupTestDB(t)
	if err := pool.SetConfig("min_epoch_interval_calls", "not-a-number"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := contextConfigInt(pool, "min_epoch_interval_calls", 20); got != 20 {
		t.Errorf("contextConfigInt(unparseable) = %d, want fallback 20", got)
	}
}

// TestNewAPIContextWatcher_DefaultsFromNilPool verifies the constructor
// resolves the decay/idle/throttle policy knobs to their hardcoded defaults
// when given a nil pool (console api chats with no project-scoped config
// available at construction time still get a usable watcher).
func TestNewAPIContextWatcher_DefaultsFromNilPool(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newAPIContextWatcher(nil, clk, "sess-1", "claude-x", 1000)

	if w.decayTurns != defaultContextDecayTurns {
		t.Errorf("decayTurns = %d, want default %d", w.decayTurns, defaultContextDecayTurns)
	}
	if w.cacheTTL.Seconds() != float64(defaultCacheTTLSec) {
		t.Errorf("cacheTTL = %v, want default %ds", w.cacheTTL, defaultCacheTTLSec)
	}
	if w.minInterval != defaultMinEpochIntervalCalls {
		t.Errorf("minInterval = %d, want default %d", w.minInterval, defaultMinEpochIntervalCalls)
	}
	if w.budgetTokens != 1000 {
		t.Errorf("budgetTokens = %d, want 1000 (passed through verbatim)", w.budgetTokens)
	}
	if w.sessionID != "sess-1" || w.model != "claude-x" {
		t.Errorf("sessionID/model = %q/%q, want sess-1/claude-x", w.sessionID, w.model)
	}
}
