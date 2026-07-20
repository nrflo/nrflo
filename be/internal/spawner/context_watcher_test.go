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

// TestNewAPIContextWatcher_DefaultCostEstimator_UsesSeededPricing verifies the
// constructor wires a real pricingCostEstimator (not swapped for a fake, as
// context_watcher_gc_test.go's newTestWatcher does) that resolves actual
// seeded per-MTok pricing when given a real pool and a known model id.
func TestNewAPIContextWatcher_DefaultCostEstimator_UsesSeededPricing(t *testing.T) {
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newAPIContextWatcher(pool, clk, "sess-cost-default", "sonnet-5", 1000)

	// sonnet-5 cache_read = 0.3 per MTok (migration 000183 seed).
	got := w.cost.EstCostSaved("sonnet-5", 2_000_000)
	want := 2_000_000.0 / 1e6 * 0.3
	if got != want {
		t.Errorf("default cost estimator EstCostSaved = %v, want %v (seeded sonnet-5 cache_read rate)", got, want)
	}
}

// TestContextConfigFloat_NilPool_ReturnsFallback mirrors
// TestContextConfigInt_NilPool_ReturnsFallback for the float reader.
func TestContextConfigFloat_NilPool_ReturnsFallback(t *testing.T) {
	if got := contextConfigFloat(nil, "context_budget_fraction", 0.65); got != 0.65 {
		t.Errorf("contextConfigFloat(nil pool) = %v, want fallback 0.65", got)
	}
}

// TestContextConfigFloat_ReadsSeededValue mirrors
// TestContextConfigInt_ReadsSeededValue for the float reader.
func TestContextConfigFloat_ReadsSeededValue(t *testing.T) {
	pool := setupTestDB(t)
	if err := pool.SetConfig("context_budget_fraction", "0.5"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := contextConfigFloat(pool, "context_budget_fraction", 0.65); got != 0.5 {
		t.Errorf("contextConfigFloat() = %v, want 0.5 (seeded value)", got)
	}
}

// TestContextConfigFloat_UnsetKey_ReturnsFallback mirrors
// TestContextConfigInt_UnsetKey_ReturnsFallback for the float reader.
func TestContextConfigFloat_UnsetKey_ReturnsFallback(t *testing.T) {
	pool := setupTestDB(t)
	if got := contextConfigFloat(pool, "context_budget_fraction", 0.65); got != 0.65 {
		t.Errorf("contextConfigFloat(unset key) = %v, want fallback 0.65", got)
	}
}

// TestContextConfigFloat_UnparseableValue_ReturnsFallback mirrors
// TestContextConfigInt_UnparseableValue_ReturnsFallback for the float reader.
func TestContextConfigFloat_UnparseableValue_ReturnsFallback(t *testing.T) {
	pool := setupTestDB(t)
	if err := pool.SetConfig("context_budget_fraction", "not-a-float"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := contextConfigFloat(pool, "context_budget_fraction", 0.65); got != 0.65 {
		t.Errorf("contextConfigFloat(unparseable) = %v, want fallback 0.65", got)
	}
}

// TestDeriveContextBudgetDefault covers the precedence matrix: an absolute
// context_budget_default>0 wins outright; else round(fraction*maxContext)
// for both cohorts of the registry's bimodal api_context (200k/1M); a
// non-positive maxContext or fraction disables the derived default (falls to
// 0, since context_budget_default is unset in every non-absolute case here).
func TestDeriveContextBudgetDefault(t *testing.T) {
	cases := []struct {
		name       string
		absolute   string // context_budget_default; "" = unset
		fraction   string // context_budget_fraction; "" = unset (code default 0.65)
		maxContext int
		want       int
	}{
		{"absolute override wins", "5000", "0.65", 200000, 5000},
		{"fraction*200k (small cohort)", "", "0.65", 200000, 130000},
		{"fraction*1M (large cohort)", "", "0.65", 1000000, 650000},
		{"maxContext<=0 falls to absolute-or-0", "", "0.65", 0, 0},
		{"maxContext<=0 with absolute set", "5000", "0.65", 0, 5000},
		{"fraction<=0 disables derivation", "", "0", 200000, 0},
		{"fraction untouched uses migration-seeded 0.65", "", "", 200000, 130000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := setupTestDB(t)
			if tc.absolute != "" {
				if err := pool.SetConfig("context_budget_default", tc.absolute); err != nil {
					t.Fatalf("SetConfig(context_budget_default): %v", err)
				}
			}
			if tc.fraction != "" {
				if err := pool.SetConfig("context_budget_fraction", tc.fraction); err != nil {
					t.Fatalf("SetConfig(context_budget_fraction): %v", err)
				}
			}
			if got := deriveContextBudgetDefault(pool, tc.maxContext); got != tc.want {
				t.Errorf("deriveContextBudgetDefault() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestNewAPIContextWatcher_DefaultCostEstimator_NilPoolDegradesGracefully
// verifies the nil-pool construction used throughout context_watcher_gc_test.go
// (before it swaps in fakeCostEstimator) never panics and reports 0.
func TestNewAPIContextWatcher_DefaultCostEstimator_NilPoolDegradesGracefully(t *testing.T) {
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	w := newAPIContextWatcher(nil, clk, "sess-cost-nilpool", "claude-x", 1000)

	if got := w.cost.EstCostSaved("claude-x", 1_000_000); got != 0 {
		t.Errorf("EstCostSaved with nil-pool watcher = %v, want 0", got)
	}
}
