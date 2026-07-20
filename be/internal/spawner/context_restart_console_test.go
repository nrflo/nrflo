package spawner

import "testing"

// TestProactiveRestartConsoleThreshold_BudgetCapsPctOfWindow verifies a
// console.Profile's ContextBudgetTokens (e.g. t0-decider's 50000) caps the
// percentage-of-window ceiling when it is the smaller value.
func TestProactiveRestartConsoleThreshold_BudgetCapsPctOfWindow(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if err := pool.SetConfig("proactive_restart_console_pct", "75"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// 75% of 200000 = 150000, well above the 50000 budget.
	got := ProactiveRestartConsoleThreshold(pool, 200000, 50000)
	if got != 50000 {
		t.Errorf("ProactiveRestartConsoleThreshold(200000, 50000) = %d, want 50000 (budget wins)", got)
	}
}

// TestProactiveRestartConsoleThreshold_BudgetAboveWindow_PctWins verifies the
// budget only caps the ceiling downward — a budget larger than the
// percentage-of-window value never raises the threshold above it.
func TestProactiveRestartConsoleThreshold_BudgetAboveWindow_PctWins(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if err := pool.SetConfig("proactive_restart_console_pct", "75"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// 75% of 200000 = 150000, under the 150000 t0-hands-sized budget: equal,
	// so pct-of-window still governs (not raised above it).
	got := ProactiveRestartConsoleThreshold(pool, 200000, 150000)
	if got != 150000 {
		t.Errorf("ProactiveRestartConsoleThreshold(200000, 150000) = %d, want 150000", got)
	}

	// A budget far above the window's pct-of-window ceiling must not raise it.
	got = ProactiveRestartConsoleThreshold(pool, 200000, 1000000)
	if got != 150000 {
		t.Errorf("ProactiveRestartConsoleThreshold(200000, 1000000) = %d, want 150000 (pct-of-window ceiling, budget must not raise it)", got)
	}
}

// TestProactiveRestartConsoleThreshold_NoBudget_UnchangedFromPreProfile
// verifies budget=0 (no profile) reproduces the pre-profile pct-of-window
// value exactly.
func TestProactiveRestartConsoleThreshold_NoBudget_UnchangedFromPreProfile(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if err := pool.SetConfig("proactive_restart_console_pct", "50"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	got := ProactiveRestartConsoleThreshold(pool, 200000, 0)
	if got != 100000 {
		t.Errorf("ProactiveRestartConsoleThreshold(200000, 0) = %d, want 100000", got)
	}
}

// TestProactiveRestartConsoleThreshold_NoMaxContext_ReturnsZero verifies an
// unknown context window disables rotation regardless of budget.
func TestProactiveRestartConsoleThreshold_NoMaxContext_ReturnsZero(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if got := ProactiveRestartConsoleThreshold(pool, 0, 50000); got != 0 {
		t.Errorf("ProactiveRestartConsoleThreshold(0, 50000) = %d, want 0", got)
	}
}

// TestWatcherBudget_ProfileBudgetWins verifies a positive profile budget
// (e.g. t0-decider's 50000) is used verbatim, ignoring the global default.
func TestWatcherBudget_ProfileBudgetWins(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if err := pool.SetConfig("context_budget_default", "9999"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := watcherBudget(pool, 50000); got != 50000 {
		t.Errorf("watcherBudget(pool, 50000) = %d, want 50000", got)
	}
}

// TestWatcherBudget_NoProfile_FallsBackToGlobalDefault verifies budget=0 (no
// profile, or t0-hands-style unset override) falls back to the
// context_budget_default global config — today's pre-profile behavior.
func TestWatcherBudget_NoProfile_FallsBackToGlobalDefault(t *testing.T) {
	t.Parallel()
	pool, _ := newRestartConfigPool(t)
	if err := pool.SetConfig("context_budget_default", "12345"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := watcherBudget(pool, 0); got != 12345 {
		t.Errorf("watcherBudget(pool, 0) = %d, want 12345 (global default)", got)
	}
}
