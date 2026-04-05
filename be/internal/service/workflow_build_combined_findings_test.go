package service

import (
	"encoding/json"
	"testing"

	"be/internal/model"
)

// TestBuildCombinedFindings_ExcludesWorkflowLevelFindings verifies that string
// workflow-level findings (e.g. user_instructions) stored in workflow_instances.findings
// do NOT appear in BuildCombinedFindings output.
func TestBuildCombinedFindings_ExcludesWorkflowLevelFindings(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	wfFindings := map[string]interface{}{
		"user_instructions": "Build the login page with OAuth",
		"_orchestration":    map[string]interface{}{"status": "running"},
	}
	data, err := json.Marshal(wfFindings)
	if err != nil {
		t.Fatalf("marshal workflow findings: %v", err)
	}
	if _, err := pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(data), wfiID); err != nil {
		t.Fatalf("update workflow instance findings: %v", err)
	}

	wi := &model.WorkflowInstance{ID: wfiID, Findings: string(data)}
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
	pool, svc, wfiID := setupDeriveTestEnv(t)

	sessionFindings := `{"my_key":"my_value","count":42}`
	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`, sessionFindings, "s1"); err != nil {
		t.Fatalf("update session findings: %v", err)
	}

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
// when no agent sessions exist, regardless of workflow-level findings.
func TestBuildCombinedFindings_EmptyWhenNoSessions(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	wfFindings := map[string]interface{}{
		"user_instructions": "some instructions",
		"summary":           "workflow summary",
	}
	data, _ := json.Marshal(wfFindings)
	pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(data), wfiID)

	wi := &model.WorkflowInstance{ID: wfiID, Findings: string(data)}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 0 {
		t.Errorf("BuildCombinedFindings should return empty map when no sessions exist, got %d entries: %v", len(combined), combined)
	}
}

// TestBuildCombinedFindings_MultipleAgents verifies that multiple agent sessions
// are each keyed by their agent_type in the combined map.
func TestBuildCombinedFindings_MultipleAgents(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-analyzer", wfiID, "analyzer", "completed", "pass", "")
	pool.Exec(`UPDATE agent_sessions SET findings = '{"analysis_result":"done"}' WHERE id = ?`, "s-analyzer")

	insertSession(t, pool, "s-builder", wfiID, "builder", "completed", "pass", "")
	pool.Exec(`UPDATE agent_sessions SET findings = '{"build_output":"binary_path"}' WHERE id = ?`, "s-builder")

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
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-opus", wfiID, "analyzer", "completed", "pass", "")
	pool.Exec(`UPDATE agent_sessions SET model_id = 'opus', findings = '{"key":"val"}' WHERE id = ?`, "s-opus")

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
// invalid JSON in findings are silently skipped.
func TestBuildCombinedFindings_InvalidFindingsJSONSkipped(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-bad", wfiID, "analyzer", "completed", "pass", "")
	pool.Exec(`UPDATE agent_sessions SET findings = 'not-valid-json' WHERE id = ?`, "s-bad")

	wi := &model.WorkflowInstance{ID: wfiID}
	combined := svc.BuildCombinedFindings(wi)

	if len(combined) != 0 {
		t.Errorf("invalid JSON findings should be skipped, got %d entries: %v", len(combined), combined)
	}
}

// TestBuildCombinedFindings_NoLeakFromWorkflowLevel is the core regression test:
// a string value in workflow instance findings must not appear in combined findings.
// If it did, JavaScript's Object.keys on a string returns character indices,
// causing one-char-per-line rendering in the UI.
func TestBuildCombinedFindings_NoLeakFromWorkflowLevel(t *testing.T) {
	pool, svc, wfiID := setupDeriveTestEnv(t)

	wfData := map[string]interface{}{
		"user_instructions": "Do X then Y",
		"other_wf_key":      "some summary text",
	}
	wfJSON, _ := json.Marshal(wfData)
	pool.Exec(`UPDATE workflow_instances SET findings = ? WHERE id = ?`, string(wfJSON), wfiID)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	pool.Exec(`UPDATE agent_sessions SET findings = '{"result":"passed"}' WHERE id = ?`, "s1")

	wi := &model.WorkflowInstance{ID: wfiID, Findings: string(wfJSON)}
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
