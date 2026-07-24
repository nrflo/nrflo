package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// insertSessionModelID sets model_id on a session already inserted via insertSession.
func insertSessionModelID(t *testing.T, pool *db.Pool, id, modelID string) {
	t.Helper()
	if _, err := pool.Exec(`UPDATE agent_sessions SET model_id = ? WHERE id = ?`, modelID, id); err != nil {
		t.Fatalf("insertSessionModelID(%s, %q): %v", id, modelID, err)
	}
}

// assertKeysAbsent fails if any of the given keys is present in m.
func assertKeysAbsent(t *testing.T, m map[string]interface{}, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := m[k]; ok {
			t.Errorf("key %q = %v, want absent", k, v)
		}
	}
}

// TestBuildActiveAgentsMap_RemovedKeysAbsent verifies a running-agent active entry
// has no agent_id/cli/model/result keys and does have session_id + model_id.
func TestBuildActiveAgentsMap_RemovedKeysAbsent(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "s1", wfiID, "rk-agent", "running", "", "")
	insertSessionModelID(t, pool, "s1", "claude:sonnet-5")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	m := getAgentEntry(t, result, "rk-agent:claude:sonnet-5")

	assertKeysAbsent(t, m, "agent_id", "cli", "model", "result")

	if got := m["session_id"]; got != "s1" {
		t.Errorf("session_id = %v, want s1", got)
	}
	if got := m["model_id"]; got != "claude:sonnet-5" {
		t.Errorf("model_id = %v, want claude:sonnet-5", got)
	}
}

// TestBuildActiveAgentsMap_ResultPresentWhenSet verifies that once a real result
// value is present on the row, the "result" key IS populated (rate-limit-continued
// / completing case), unlike the no-result running case above.
func TestBuildActiveAgentsMap_ResultPresentWhenSet(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "s1", wfiID, "rk-agent", "running", "pass", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	m := getAgentEntry(t, result, "rk-agent")

	if got := m["result"]; got != "pass" {
		t.Errorf("result = %v, want pass", got)
	}
}

// TestBuildActiveAgentsMap_NoModelIDNoModelKeys verifies that when model_id is
// unset, neither model_id nor the removed cli/model derivations appear.
func TestBuildActiveAgentsMap_NoModelIDNoModelKeys(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "s1", wfiID, "rk-agent", "running", "", "")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	m := getAgentEntry(t, result, "rk-agent")

	assertKeysAbsent(t, m, "agent_id", "cli", "model", "result", "model_id")
}

// TestBuildAgentHistory_RemovedKeysAbsent verifies a history entry has no
// agent_id key and does have session_id + model_id (when set).
func TestBuildAgentHistory_RemovedKeysAbsent(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "sh1", wfiID, "rk-h-agent", "completed", "pass", "")
	insertSessionModelID(t, pool, "sh1", "openai:gpt-5")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1", len(history))
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatalf("buildAgentHistory[0] = %T, want map", history[0])
	}

	assertKeysAbsent(t, entry, "agent_id", "cli", "model")

	if got := entry["session_id"]; got != "sh1" {
		t.Errorf("session_id = %v, want sh1", got)
	}
	if got := entry["model_id"]; got != "openai:gpt-5" {
		t.Errorf("model_id = %v, want openai:gpt-5", got)
	}
}

// TestBuildV4State_NoAgentRetriesKey verifies the top-level v4 result map no
// longer includes the hardcoded agent_retries key.
func TestBuildV4State_NoAgentRetriesKey(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	if v, ok := state["agent_retries"]; ok {
		t.Errorf(`state["agent_retries"] = %v, want key absent`, v)
	}
}
