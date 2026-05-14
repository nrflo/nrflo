package service

import (
	"testing"
)

// TestBuildActiveAgentsMap_ExcludesPlanner verifies that planner sessions are
// excluded from the active agents map even when status=running.
func TestBuildActiveAgentsMap_ExcludesPlanner(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-planner", wfiID, "planner", "running", "", "")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})

	if _, ok := result["planner"]; ok {
		t.Error("planner session must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["analyzer"]; !ok {
		t.Error("analyzer session must appear in buildActiveAgentsMap")
	}
	if len(result) != 1 {
		t.Errorf("buildActiveAgentsMap len = %d, want 1 (only analyzer)", len(result))
	}
}

// TestBuildAgentHistory_ExcludesPlanner verifies that planner sessions are
// excluded from the agent history even after interactive_completed.
func TestBuildAgentHistory_ExcludesPlanner(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-planner", wfiID, "planner", "interactive_completed", "pass", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "2025-01-01T00:00:02Z")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})

	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1 (planner excluded)", len(history))
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("buildAgentHistory[0] = %T, want map", history[0])
	}
	if entry["agent_type"] != "analyzer" {
		t.Errorf("history entry agent_type = %v, want 'analyzer'", entry["agent_type"])
	}
}

// TestDerivePhaseStatuses_IgnoresPlannerSession verifies that planner sessions
// are excluded from phase status derivation and do not appear as phase keys.
func TestDerivePhaseStatuses_IgnoresPlannerSession(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-planner", wfiID, "planner", "user_interactive", "", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "2025-01-01T00:00:02Z")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	if _, ok := got["planner"]; ok {
		t.Error("planner must not appear as a phase key in derivePhaseStatuses")
	}
	assertPhase(t, got, "analyzer", "in_progress", "")
	assertPhase(t, got, "builder", "pending", "")
}

// TestBuildActiveAgentsMap_OnlyPlanner_ReturnsEmpty verifies that when only a
// planner session is running, the active agents map is empty.
func TestBuildActiveAgentsMap_OnlyPlanner_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-planner", wfiID, "planner", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if len(result) != 0 {
		t.Errorf("buildActiveAgentsMap with only planner = %d entries, want 0", len(result))
	}
}

// TestBuildAgentHistory_PlannerOnlyNoHistory verifies that a sole planner session
// produces an empty history slice.
func TestBuildAgentHistory_PlannerOnlyNoHistory(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-planner", wfiID, "planner", "interactive_completed", "pass", "")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 0 {
		t.Errorf("buildAgentHistory with only planner = %d entries, want 0", len(history))
	}
}
