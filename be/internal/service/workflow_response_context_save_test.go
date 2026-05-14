package service

import (
	"testing"

	"be/internal/model"
)

// TestActiveAgents_ExcludesContextSaver verifies that context-saver and
// conflict-resolver sessions are excluded from the active agents map even when running.
func TestActiveAgents_ExcludesContextSaver(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-impl", wfiID, "implementor", "running", "", "")
	insertSession(t, pool, "s-cs", wfiID, "context-saver", "running", "", "")
	insertSession(t, pool, "s-cr", wfiID, "conflict-resolver", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})

	if len(result) != 1 {
		t.Errorf("buildActiveAgentsMap len = %d, want 1 (only implementor); keys: %v", len(result), mapKeys(result))
	}
	if _, ok := result["implementor"]; !ok {
		t.Errorf("buildActiveAgentsMap missing 'implementor'; got keys: %v", mapKeys(result))
	}
	if _, ok := result["context-saver"]; ok {
		t.Error("context-saver must not appear in buildActiveAgentsMap")
	}
	if _, ok := result["conflict-resolver"]; ok {
		t.Error("conflict-resolver must not appear in buildActiveAgentsMap")
	}
}

// TestAgentHistory_ExcludesSystemAgents verifies that context-saver and
// conflict-resolver sessions are excluded from agent history.
func TestAgentHistory_ExcludesSystemAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-impl", wfiID, "implementor", "completed", "pass", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-cs", wfiID, "context-saver", "completed", "pass", "2025-01-01T00:00:02Z")
	insertSession(t, pool, "s-cr1", wfiID, "conflict-resolver", "completed", "pass", "2025-01-01T00:00:03Z")
	insertSession(t, pool, "s-cr2", wfiID, "conflict-resolver", "failed", "fail", "2025-01-01T00:00:04Z")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})

	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1 (system agents excluded)", len(history))
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("history[0] = %T, want map[string]interface{}", history[0])
	}
	if entry["agent_type"] != "implementor" {
		t.Errorf("history[0].agent_type = %v, want 'implementor'", entry["agent_type"])
	}
}

// TestDerivePhaseStatuses_IgnoresSystemAgents verifies that context-saver and
// conflict-resolver sessions do not appear as phase keys and do not perturb
// the maxLayer or seen tracking.
func TestDerivePhaseStatuses_IgnoresSystemAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "2025-01-01T00:00:01Z")
	insertSession(t, pool, "s-builder", wfiID, "builder", "running", "", "2025-01-01T00:00:02Z")
	insertSession(t, pool, "s-cs", wfiID, "context-saver", "completed", "pass", "2025-01-01T00:00:03Z")
	insertSession(t, pool, "s-cr", wfiID, "conflict-resolver", "completed", "pass", "2025-01-01T00:00:04Z")

	got := svc.derivePhaseStatuses(wfiID, twoPhases)

	assertPhase(t, got, "analyzer", "completed", "pass")
	assertPhase(t, got, "builder", "in_progress", "")

	if _, ok := got["context-saver"]; ok {
		t.Error("context-saver must not appear as a phase key in derivePhaseStatuses")
	}
	if _, ok := got["conflict-resolver"]; ok {
		t.Error("conflict-resolver must not appear as a phase key in derivePhaseStatuses")
	}
}

// TestBuildCombinedFindings_ExcludesSystemAgents verifies that context-saver and
// conflict-resolver session findings are excluded from the aggregated findings map.
func TestBuildCombinedFindings_ExcludesSystemAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-impl", wfiID, "implementor", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = '{"k":"v"}' WHERE id = ?`, "s-impl"); err != nil {
		t.Fatalf("set implementor findings: %v", err)
	}

	insertSession(t, pool, "s-cs", wfiID, "context-saver", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = '{"to_resume":"x"}' WHERE id = ?`, "s-cs"); err != nil {
		t.Fatalf("set context-saver findings: %v", err)
	}

	insertSession(t, pool, "s-cr", wfiID, "conflict-resolver", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = '{"resolved":"y"}' WHERE id = ?`, "s-cr"); err != nil {
		t.Fatalf("set conflict-resolver findings: %v", err)
	}

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 1 {
		t.Errorf("BuildCombinedFindings len = %d, want 1 (system agents excluded); keys: %v", len(combined), buildCombinedFindingsKeys(combined))
	}
	if _, ok := combined["implementor"]; !ok {
		t.Errorf("combined missing 'implementor' key; got: %v", buildCombinedFindingsKeys(combined))
	}
	if _, ok := combined["context-saver"]; ok {
		t.Error("combined must not contain 'context-saver' key")
	}
	if _, ok := combined["conflict-resolver"]; ok {
		t.Error("combined must not contain 'conflict-resolver' key")
	}
}

// mapKeys returns the keys of a map[string]interface{} for error messages.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
