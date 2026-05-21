package service

import (
	"testing"
	"time"

	"be/internal/db"
)

// insertSessionWithPhase inserts an agent_session where phase differs from agent_type.
// Used to test the `phase NOT LIKE '\_%' ESCAPE '\'` branch of transientAgentTypeExclusion.
func insertSessionWithPhase(t *testing.T, pool *db.Pool, id, wfiID, agentType, phase, status, result string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var resultVal interface{}
	if result != "" {
		resultVal = result
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 status, result, result_reason, pid, context_left, ancestor_session_id,
			 spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		id, wfiID, phase, agentType, status, resultVal, now, now, now)
	if err != nil {
		t.Fatalf("insertSessionWithPhase %s (phase=%s agent_type=%s): %v", id, phase, agentType, err)
	}
}

// TestDerivePhaseStatuses_ExcludesConsultPhase verifies that a _consult-phase session
// is excluded from derivePhaseStatuses even when agent_type is a normal non-underscore
// value (e.g. "architect"). This guards the `phase NOT LIKE '\_%' ESCAPE '\'` clause.
func TestDerivePhaseStatuses_ExcludesConsultPhase(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSessionWithPhase(t, pool, "s-consult", wfiID, "architect", "_consult", "completed", "pass")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	if _, ok := got["_consult"]; ok {
		t.Error("_consult must not appear as a phase key in derivePhaseStatuses")
	}
	assertPhase(t, got, "analyzer", "in_progress", "")
	assertPhase(t, got, "builder", "pending", "")
}

// TestDerivePhaseStatuses_ExcludesUnderscoreAgentType verifies that sessions with an
// underscore-prefixed agent_type (_consult) are also excluded.
func TestDerivePhaseStatuses_ExcludesUnderscoreAgentType(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-consult", wfiID, "_consult", "completed", "pass", "")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	if _, ok := got["_consult"]; ok {
		t.Error("_consult agent_type must not appear as a phase key in derivePhaseStatuses")
	}
	assertPhase(t, got, "analyzer", "in_progress", "")
}

// TestBuildAgentHistory_ExcludesConsultPhase verifies that _consult-phase sessions are
// omitted from history even when agent_type is a non-underscore value like "architect".
func TestBuildAgentHistory_ExcludesConsultPhase(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSessionWithPhase(t, pool, "s-consult", wfiID, "architect", "_consult", "completed", "pass")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1 (consult excluded)", len(history))
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("history[0] type = %T, want map[string]interface{}", history[0])
	}
	if entry["agent_type"] != "analyzer" {
		t.Errorf("history[0] agent_type = %v, want 'analyzer'", entry["agent_type"])
	}
}

// TestBuildActiveAgentsMap_ExcludesConsultPhase verifies running _consult-phase sessions
// are excluded from the active agents map even when agent_type is a normal value.
func TestBuildActiveAgentsMap_ExcludesConsultPhase(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSessionWithPhase(t, pool, "s-consult", wfiID, "architect", "_consult", "running", "")
	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})

	if _, ok := result["architect"]; ok {
		t.Error("architect with _consult phase must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["analyzer"]; !ok {
		t.Error("analyzer must appear in buildActiveAgentsMap")
	}
	if len(result) != 1 {
		t.Errorf("buildActiveAgentsMap len = %d, want 1", len(result))
	}
}

// TestBuildActiveAgentsMap_NormalPhaseForSameAgentType_StillSurfaced verifies that a
// normal-phase session for the same agent_type as a _consult-phase session is still shown.
func TestBuildActiveAgentsMap_NormalPhaseForSameAgentType_StillSurfaced(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	// _consult phase for "architect" → excluded
	insertSessionWithPhase(t, pool, "s-consult", wfiID, "architect", "_consult", "completed", "pass")
	// Normal session for "architect" in a regular phase → must appear
	insertSession(t, pool, "s-arch-normal", wfiID, "architect", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})

	if _, ok := result["architect"]; !ok {
		t.Error("architect with a normal phase must appear in buildActiveAgentsMap")
	}
}
