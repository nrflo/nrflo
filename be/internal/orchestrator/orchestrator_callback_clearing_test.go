package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestClearCallbackMetadata tests that _callback is removed from workflow instance
// findings after the callback target layer completes successfully.
func TestClearCallbackMetadata(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-CLEAR", "Clear callback test")

	// Create workflow instance with callback metadata in findings
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-clear', ?, 'CB-CLEAR', 'test', 'active', '["analyzer","builder"]',
		        '{"analyzer":{"status":"active"},"builder":{"status":"pending"}}',
		        '{"_callback":{"level":0,"instructions":"Fix it","from_layer":1,"from_agent":"builder"},"other_key":"other_value"}',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Verify _callback exists before clearing
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if _, ok := findings["_callback"]; !ok {
		t.Fatal("expected _callback to exist before clearing")
	}
	if findings["other_key"] != "other_value" {
		t.Error("expected other_key to be preserved")
	}

	// Clear callback metadata
	env.orch.clearCallbackMetadata(wfiID)

	// Verify _callback was removed
	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("expected _callback to be removed after clearing")
	}
	// Verify other findings are preserved
	if findings["other_key"] != "other_value" {
		t.Error("expected other_key to be preserved after clearing _callback")
	}
}

// TestClearCallbackMetadata_NoCallback tests that clearCallbackMetadata is safe
// when there's no _callback key to clear.
func TestClearCallbackMetadata_NoCallback(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-NOCLEAR", "No callback test")

	// Create workflow instance without callback metadata
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-noclear', ?, 'CB-NOCLEAR', 'test', 'active', '["analyzer"]',
		        '{"analyzer":{"status":"active"}}',
		        '{"some_key":"some_value"}',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Clear callback metadata (should be a no-op)
	env.orch.clearCallbackMetadata(wfiID)

	// Verify findings are unchanged
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if findings["some_key"] != "some_value" {
		t.Error("expected existing findings to be preserved")
	}
}

// TestClearCallbackMetadata_EmptyFindings tests that clearCallbackMetadata
// handles empty findings correctly.
func TestClearCallbackMetadata_EmptyFindings(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-EMPTY", "Empty findings test")

	// Create workflow instance with empty findings
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-empty', ?, 'CB-EMPTY', 'test', 'active', '["analyzer"]',
		        '{"analyzer":{"status":"active"}}',
		        '{}',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Clear callback metadata (should be a no-op)
	env.orch.clearCallbackMetadata(wfiID)

	// Verify findings are still empty
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if len(findings) != 0 {
		t.Errorf("expected empty findings, got %d keys", len(findings))
	}
}

// TestClearCallbackMetadata_InvalidWorkflowID tests that clearCallbackMetadata
// handles invalid workflow instance IDs gracefully.
func TestClearCallbackMetadata_InvalidWorkflowID(t *testing.T) {
	env := newTestEnv(t)

	// Try to clear callback metadata for non-existent workflow instance
	// Should not panic, just log error
	env.orch.clearCallbackMetadata("nonexistent-wfi-id")

	// No assertion needed - we're just verifying it doesn't panic
}

// TestClearCallbackMetadata_ProjectScope tests that clearCallbackMetadata
// works for project-scoped workflows.
func TestClearCallbackMetadata_ProjectScope(t *testing.T) {
	env := newTestEnv(t)

	// Update test workflow to project scope
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE id = 'test'`)
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	// Create project-scoped workflow instance with callback metadata
	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-proj-clear', ?, '', 'test', 'active', 'project', '["analyzer"]',
		        '{"analyzer":{"status":"active"}}',
		        '{"_callback":{"level":0,"instructions":"Project callback","from_agent":"verifier"},"project_key":"project_value"}',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create project workflow instance: %v", err)
	}

	// Clear callback metadata
	env.orch.clearCallbackMetadata(wfiID)

	// Verify _callback was removed but other findings preserved
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("expected _callback to be removed from project workflow")
	}
	if findings["project_key"] != "project_value" {
		t.Error("expected project_key to be preserved")
	}
}

// TestClearCallbackMetadata_MultipleCallbackFields tests that all _callback
// fields are removed (level, instructions, from_layer, from_agent).
func TestClearCallbackMetadata_MultipleCallbackFields(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-MULTI", "Multiple callback fields test")

	// Create workflow instance with full callback metadata
	callbackData := map[string]interface{}{
		"level":        2,
		"instructions": "Detailed instructions here",
		"from_layer":   3,
		"from_agent":   "qa-verifier",
	}
	findingsData := map[string]interface{}{
		"_callback":  callbackData,
		"keep_this":  "value",
		"and_this":   123,
	}
	findingsJSON, _ := json.Marshal(findingsData)

	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, phase_order, phases, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, 'CB-MULTI', 'test', 'active', '["analyzer"]',
		        '{"analyzer":{"status":"active"}}',
		        ?, 0, datetime('now'), datetime('now'))
		RETURNING id`, uuid.New().String(), env.project, string(findingsJSON)).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Verify all callback fields exist
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
	callback, ok := findings["_callback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _callback to exist before clearing")
	}
	if callback["level"] != float64(2) {
		t.Error("expected level field in callback")
	}
	if callback["instructions"] != "Detailed instructions here" {
		t.Error("expected instructions field in callback")
	}
	if callback["from_layer"] != float64(3) {
		t.Error("expected from_layer field in callback")
	}
	if callback["from_agent"] != "qa-verifier" {
		t.Error("expected from_agent field in callback")
	}

	// Clear callback metadata
	env.orch.clearCallbackMetadata(wfiID)

	// Verify entire _callback key is removed (not just individual fields)
	wi = env.getWorkflowInstance(t, wfiID)
	findings = wi.GetFindings()
	if _, ok := findings["_callback"]; ok {
		t.Error("expected entire _callback key to be removed")
	}
	// Verify other findings preserved
	if findings["keep_this"] != "value" {
		t.Error("expected keep_this to be preserved")
	}
	if findings["and_this"] != float64(123) {
		t.Error("expected and_this to be preserved")
	}
}
