package service

import (
	"testing"
)

// TestDerivePhaseStatuses_ExcludesHookSessions verifies that underscore-prefixed
// transient hook sessions (_finalize, _pause, _notification) are excluded from
// phase status derivation and do not appear as phase keys.
func TestDerivePhaseStatuses_ExcludesHookSessions(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "running", "", "2025-01-01T00:00:00Z")
	insertSession(t, pool, "s-pause", wfiID, "_pause", "running", "", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-notification", wfiID, "_notification", "running", "", "2025-01-01T00:00:02Z")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "2025-01-01T00:00:03Z")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	if _, ok := got["_finalize"]; ok {
		t.Error("_finalize must not appear as a phase key in derivePhaseStatuses")
	}
	if _, ok := got["_pause"]; ok {
		t.Error("_pause must not appear as a phase key in derivePhaseStatuses")
	}
	if _, ok := got["_notification"]; ok {
		t.Error("_notification must not appear as a phase key in derivePhaseStatuses")
	}
	assertPhase(t, got, "analyzer", "in_progress", "")
	assertPhase(t, got, "builder", "pending", "")
}

// TestDerivePhaseStatuses_HookOnlyNoPhases verifies that when only hook sessions
// exist, all known phases remain pending (no spurious skip inference from hook layers).
func TestDerivePhaseStatuses_HookOnlyNoPhases(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "completed", "pass", "2025-01-01T00:00:00Z")
	insertSession(t, pool, "s-pause", wfiID, "_pause", "failed", "fail", "2025-01-01T00:00:01Z")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	assertPhase(t, got, "analyzer", "pending", "")
	assertPhase(t, got, "builder", "pending", "")
}

// TestBuildAgentHistory_ExcludesHookSessions verifies that terminal _finalize,
// _pause, and _notification sessions are excluded from buildAgentHistory.
func TestBuildAgentHistory_ExcludesHookSessions(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "completed", "pass", "2025-01-01T00:00:00Z")
	insertSession(t, pool, "s-pause", wfiID, "_pause", "failed", "fail", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-notification", wfiID, "_notification", "completed", "pass", "2025-01-01T00:00:02Z")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "2025-01-01T00:00:03Z")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})

	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1 (hook sessions excluded)", len(history))
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("buildAgentHistory[0] type = %T, want map[string]interface{}", history[0])
	}
	if entry["agent_type"] != "analyzer" {
		t.Errorf("history entry agent_type = %v, want %q", entry["agent_type"], "analyzer")
	}
}

// TestBuildAgentHistory_HookOnlyReturnsEmpty verifies that a sole _finalize session
// produces an empty history slice.
func TestBuildAgentHistory_HookOnlyReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "completed", "pass", "")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 0 {
		t.Errorf("buildAgentHistory with only _finalize = %d entries, want 0", len(history))
	}
}

// TestBuildActiveAgentsMap_ExcludesHookSessions verifies that running _finalize,
// _pause, and _notification sessions are excluded from buildActiveAgentsMap.
func TestBuildActiveAgentsMap_ExcludesHookSessions(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "running", "", "")
	insertSession(t, pool, "s-pause", wfiID, "_pause", "running", "", "")
	insertSession(t, pool, "s-notification", wfiID, "_notification", "running", "", "")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})

	if _, ok := result["_finalize"]; ok {
		t.Error("_finalize session must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["_pause"]; ok {
		t.Error("_pause session must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["_notification"]; ok {
		t.Error("_notification session must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["analyzer"]; !ok {
		t.Error("analyzer session must appear in buildActiveAgentsMap")
	}
	if len(result) != 1 {
		t.Errorf("buildActiveAgentsMap len = %d, want 1 (only analyzer)", len(result))
	}
}

// TestBuildActiveAgentsMap_HookOnlyReturnsEmpty verifies that when only hook
// sessions are running, the active agents map is empty.
func TestBuildActiveAgentsMap_HookOnlyReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "running", "", "")
	insertSession(t, pool, "s-pause", wfiID, "_pause", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if len(result) != 0 {
		t.Errorf("buildActiveAgentsMap with only hook sessions = %d entries, want 0", len(result))
	}
}

// TestLikeEscapeDoesNotExcludeLegitAgents verifies that the LIKE '\_%' ESCAPE '\'
// filter does not accidentally exclude legitimate agent types that contain an
// underscore in a non-leading position (e.g. "qa_verifier").
func TestLikeEscapeDoesNotExcludeLegitAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// Insert a legitimate agent with embedded underscore (non-prefix).
	// Note: it must be in the phases slice to show up in derivePhaseStatuses.
	now := "2025-01-01T00:00:00Z"
	_, err := pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES ('qa_verifier', 'test-proj', 'test-wf', '', 2, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert agent_definition qa_verifier: %v", err)
	}

	insertSession(t, pool, "s-qa", wfiID, "qa_verifier", "running", "", "")
	insertSession(t, pool, "s-finalize", wfiID, "_finalize", "running", "", "")

	threePhases := append(twoPhases, PhaseDef{ID: "qa_verifier", Agent: "qa_verifier", Layer: 2})

	got := svc.derivePhaseStatuses(wfiID, threePhases)

	assertPhase(t, got, "qa_verifier", "in_progress", "")
	if _, ok := got["_finalize"]; ok {
		t.Error("_finalize must not appear as a phase key")
	}

	activeAgents := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if _, ok := activeAgents["qa_verifier"]; !ok {
		t.Error("qa_verifier (non-prefix underscore) must appear in buildActiveAgentsMap")
	}
	if _, ok := activeAgents["_finalize"]; ok {
		t.Error("_finalize must not appear in buildActiveAgentsMap")
	}
}
