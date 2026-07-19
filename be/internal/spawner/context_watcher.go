package spawner

import (
	"context"
	"strconv"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/spawner/apirun"
)

const (
	defaultContextDecayTurns     = 20  // context_decay_turns: tool_result/file_read staleness window, in ledger turns
	defaultCacheTTLSec           = 300 // cache_ttl_sec: idle gap before a deferred GC is free (cache already cold)
	defaultMinEpochIntervalCalls = 20  // min_epoch_interval_calls: rewrites throttled to at most once per this many PlanGC consults, except idle-gap
)

// apiContextWatcher implements apirun.ContextWatcher: a policy engine over
// the process-global context ledger (spawner/ledger*.go) that decides
// selective epoch GC for one api-mode session. One instance per spawn/
// console-chat; constructed nil-safe into apirun.Config.Watcher.
type apiContextWatcher struct {
	store     *ledgerStore
	sessionID string
	model     string
	clock     clock.Clock
	cost      ContextCostEstimator

	budgetTokens int // 0 = budget-triggered GC disabled for this session
	decayTurns   int
	cacheTTL     time.Duration
	minInterval  int

	mu                sync.Mutex
	lastRequest       time.Time
	callsSinceRewrite int
}

// newAPIContextWatcher constructs an apiContextWatcher against the
// process-global ledger store, reading its decay/idle/throttle policy knobs
// from global config once at construction (mirrors newAPILedgerObserver's
// wiring style, generalized to take primitive args so both the autonomous
// backend and the console engine can build one).
func newAPIContextWatcher(pool *db.Pool, clk clock.Clock, sessionID, modelID string, budgetTokens int) *apiContextWatcher {
	return &apiContextWatcher{
		store:        globalLedgerStore,
		sessionID:    sessionID,
		model:        modelID,
		clock:        clk,
		cost:         newPricingCostEstimator(pool, clk),
		budgetTokens: budgetTokens,
		decayTurns:   contextConfigInt(pool, "context_decay_turns", defaultContextDecayTurns),
		cacheTTL:     time.Duration(contextConfigInt(pool, "cache_ttl_sec", defaultCacheTTLSec)) * time.Second,
		minInterval:  contextConfigInt(pool, "min_epoch_interval_calls", defaultMinEpochIntervalCalls),
		lastRequest:  clk.Now(),
	}
}

// resolveContextBudget resolves the effective per-session live-token budget:
// a non-nil def override wins (0 = disabled, >0 = budget), NULL falls
// through to defaultBudget (the global context_budget_default config value,
// itself 0 = disabled). Nil-def (system agents, global workflows) also falls
// through to defaultBudget.
func resolveContextBudget(def *model.AgentDefinition, defaultBudget int) int {
	if def != nil && def.ContextBudgetTokens != nil {
		return *def.ContextBudgetTokens
	}
	return defaultBudget
}

// contextConfigInt reads an integer global config key, falling back to
// fallback when the pool is nil, the key is unset, or it doesn't parse.
func contextConfigInt(pool *db.Pool, key string, fallback int) int {
	if pool == nil {
		return fallback
	}
	v, _ := pool.GetConfig(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

// PlanGC implements apirun.ContextWatcher. It stamps the idle-gap clock on
// every call (so idle is measured from the last consult, not the last GC),
// then decides: skip when neither over budget nor idle; skip when throttled
// (rewrites capped at once per minInterval calls, except idle-gap, which
// always runs — the cache is already cold, so the rewrite is free); else
// select entries to evict via the ledger's superseded→stale→dialog ordering
// and translate the selection into a CompactionPlan.
func (w *apiContextWatcher) PlanGC(state apirun.WatcherState) (apirun.CompactionPlan, bool) {
	w.mu.Lock()
	now := w.clock.Now()
	idle := now.Sub(w.lastRequest) >= w.cacheTTL
	w.lastRequest = now
	w.callsSinceRewrite++
	calls := w.callsSinceRewrite
	w.mu.Unlock()

	summary, ok := w.store.epochSummary(w.sessionID)
	if !ok {
		return apirun.CompactionPlan{}, false
	}

	overBudget := w.budgetTokens > 0 && summary.TotalTokens > w.budgetTokens
	if !overBudget && !idle {
		return apirun.CompactionPlan{}, false
	}
	if !idle && calls < w.minInterval {
		return apirun.CompactionPlan{}, false
	}

	snap, ok := w.store.snapshot(w.sessionID)
	if !ok {
		return apirun.CompactionPlan{}, false
	}
	turnNow, _ := w.store.turnNow(w.sessionID)

	target := summary.TotalTokens - w.budgetTokens
	if !overBudget {
		target = 0 // idle-gap: evict everything eligible, uncapped
	}
	ev := selectEviction(snap.Entries, turnNow, w.decayTurns, target)
	if ev.evictCount == 0 {
		return apirun.CompactionPlan{}, false
	}

	keepPrefix, keepSuffix := resolveKeepCounts(state.MessageCount, ev.evictCount, len(snap.Entries))
	if keepPrefix+keepSuffix >= state.MessageCount {
		return apirun.CompactionPlan{}, false
	}

	w.mu.Lock()
	w.callsSinceRewrite = 0
	w.mu.Unlock()

	costSaved := w.cost.EstCostSaved(w.model, ev.tokensEvicted)
	logger.Info(context.Background(), "context watcher gc",
		"session", w.sessionID,
		"policy", "selective",
		"tokens_evicted", ev.tokensEvicted,
		"est_cost_saved", costSaved,
		"idle", idle,
		"over_budget", overBudget,
	)

	return apirun.CompactionPlan{
		KeepPrefixMsgs:  keepPrefix,
		KeepSuffixMsgs:  keepSuffix,
		ReferenceDigest: ev.digest,
		PolicyName:      "selective",
		TokensEvicted:   ev.tokensEvicted,
	}, true
}
