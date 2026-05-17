package service

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// upsertSessionFinding stores a single key/value finding in scope=session.
func upsertSessionFinding(t *testing.T, pool *db.Pool, wfiID, sessionID, agentType, modelID, key string, value interface{}) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal finding value: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	if err := fr.Upsert("session", sessionID, key, raw,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: agentType, ModelID: modelID},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding %q: %v", key, err)
	}
}

// upsertSessionFindingsFromJSON parses a JSON object and upserts each key as a session finding.
func upsertSessionFindingsFromJSON(t *testing.T, pool *db.Pool, wfiID, sessionID, agentType, findingsJSON string) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(findingsJSON), &m); err != nil {
		t.Fatalf("unmarshal findings JSON: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	for k, v := range m {
		if err := fr.Upsert("session", sessionID, k, v,
			repo.Denorm{WorkflowInstanceID: wfiID, AgentType: agentType},
			repo.Actor{Source: "system"}); err != nil {
			t.Fatalf("upsert finding %q: %v", k, err)
		}
	}
}

// TestBuildCombinedFindings_ExcludesWorkflowLevelFindings verifies that string
// workflow-level findings do NOT appear in BuildCombinedFindings output.
// With the new findings table, workflow_instance scope is never included in
// BuildCombinedFindings (which only reads scope=session via ListByWorkflowInstance).
func TestBuildCombinedFindings_ExcludesWorkflowLevelFindings(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupDeriveTestEnv(t)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if _, exists := combined["user_instructions"]; exists {
		t.Errorf("BuildCombinedFindings should not include workflow-level string finding user_instructions")
	}
	if _, exists := combined["_orchestration"]; exists {
		t.Errorf("BuildCombinedFindings should not include workflow-level _orchestration")
	}
}

// TestBuildCombinedFindings_IncludesAgentSessionFindings verifies that agent session
// findings are included, keyed by agent_type.
func TestBuildCombinedFindings_IncludesAgentSessionFindings(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s1", "analyzer", `{"my_key":"my_value","count":42}`)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	agentMap, ok := combined["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("combined[analyzer] should be a map, got %T: %v", combined["analyzer"], combined["analyzer"])
	}
	if agentMap["my_key"] != "my_value" {
		t.Errorf("combined[analyzer][my_key] = %v, want %q", agentMap["my_key"], "my_value")
	}
}

// TestBuildCombinedFindings_EmptyWhenNoSessions verifies that the result is empty
// when no agent sessions exist.
func TestBuildCombinedFindings_EmptyWhenNoSessions(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupDeriveTestEnv(t)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 0 {
		t.Errorf("BuildCombinedFindings should return empty map when no sessions exist, got %d entries: %v", len(combined), combined)
	}
}

// TestBuildCombinedFindings_MultipleAgents verifies that multiple agent sessions
// are each keyed by their agent_type in the combined map.
func TestBuildCombinedFindings_MultipleAgents(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s-analyzer", "analyzer", `{"analysis_result":"done"}`)

	insertSession(t, pool, "s-builder", wfiID, "builder", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s-builder", "builder", `{"build_output":"binary_path"}`)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 2 {
		t.Fatalf("expected 2 entries in combined, got %d: %v", len(combined), combined)
	}

	analyzerMap, ok := combined["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("combined[analyzer] not a map: %T", combined["analyzer"])
	}
	if analyzerMap["analysis_result"] != "done" {
		t.Errorf("combined[analyzer][analysis_result] = %v, want %q", analyzerMap["analysis_result"], "done")
	}

	builderMap, ok := combined["builder"].(map[string]interface{})
	if !ok {
		t.Fatalf("combined[builder] not a map: %T", combined["builder"])
	}
	if builderMap["build_output"] != "binary_path" {
		t.Errorf("combined[builder][build_output] = %v, want %q", builderMap["build_output"], "binary_path")
	}
}

// TestBuildCombinedFindings_SessionWithModelID verifies that sessions with a model_id
// are keyed as "agent_type:model_id" in the combined map.
func TestBuildCombinedFindings_SessionWithModelID(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-opus", wfiID, "analyzer", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET model_id = 'opus' WHERE id = ?`, "s-opus"); err != nil {
		t.Fatalf("set model_id: %v", err)
	}
	upsertSessionFinding(t, pool, wfiID, "s-opus", "analyzer", "opus", "key", "val")

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if _, ok := combined["analyzer:opus"]; !ok {
		t.Errorf("expected key 'analyzer:opus' in combined, got keys: %v", buildCombinedFindingsKeys(combined))
	}
	if _, ok := combined["analyzer"]; ok {
		t.Errorf("unexpected bare 'analyzer' key when model_id is set")
	}
}

// TestBuildCombinedFindings_InvalidFindingsJSONSkipped verifies that sessions with
// no findings in the findings table produce no entry in combined output.
func TestBuildCombinedFindings_InvalidFindingsJSONSkipped(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-bad", wfiID, "analyzer", "completed", "pass", "")
	// No findings inserted — session has no findings table entries.

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 0 {
		t.Errorf("session with no findings should produce no entries, got %d entries: %v", len(combined), combined)
	}
}

// TestBuildCombinedFindings_NoLeakFromWorkflowLevel is the core regression test:
// workflow_instance scope findings must not appear in combined findings output.
func TestBuildCombinedFindings_NoLeakFromWorkflowLevel(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	upsertSessionFindingsFromJSON(t, pool, wfiID, "s1", "analyzer", `{"result":"passed"}`)

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	for _, forbidden := range []string{"user_instructions", "other_wf_key"} {
		if _, exists := combined[forbidden]; exists {
			t.Errorf("workflow-level key %q must not appear in BuildCombinedFindings output", forbidden)
		}
	}

	agentMap, ok := combined["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("combined[analyzer] should be a map, got %T", combined["analyzer"])
	}
	if agentMap["result"] != "passed" {
		t.Errorf("combined[analyzer][result] = %v, want %q", agentMap["result"], "passed")
	}
}

// buildCombinedFindingsKeys returns the keys of a map for error messages.
func buildCombinedFindingsKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
