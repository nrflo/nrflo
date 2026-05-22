package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// TestClearCallbackMetadata tests that _callback is removed from workflow instance
// findings after the callback target layer completes successfully.
func TestClearCallbackMetadata(t *testing.T) {
	env := newTestEnv(t)

	env.createTicket(t, "CB-CLEAR", "Clear callback test")

	// Create workflow instance with callback metadata in findings
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-clear', ?, 'CB-CLEAR', 'test', 'active',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	cbVal, _ := json.Marshal(map[string]interface{}{"level": 0, "instructions": "Fix it", "from_layer": 1, "from_agent": "builder"})
	findingRepo.Upsert("workflow_instance", wfiID, "_callback", json.RawMessage(cbVal), repo.Denorm{}, repo.Actor{Source: "system"})           //nolint:errcheck
	findingRepo.Upsert("workflow_instance", wfiID, "other_key", json.RawMessage(`"other_value"`), repo.Denorm{}, repo.Actor{Source: "system"}) //nolint:errcheck

	// Verify _callback exists before clearing
	findings := getWFIFindings(t, env, wfiID)
	if _, ok := findings["_callback"]; !ok {
		t.Fatal("expected _callback to exist before clearing")
	}
	if findings["other_key"] != "other_value" {
		t.Error("expected other_key to be preserved")
	}

	// Clear callback metadata
	env.orch.clearCallbackMetadata(context.Background(), wfiID)

	// Verify _callback was removed
	findings = getWFIFindings(t, env, wfiID)
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
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-noclear', ?, 'CB-NOCLEAR', 'test', 'active',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	findingRepo.Upsert("workflow_instance", wfiID, "some_key", json.RawMessage(`"some_value"`), repo.Denorm{}, repo.Actor{Source: "system"}) //nolint:errcheck

	// Clear callback metadata (should be a no-op)
	env.orch.clearCallbackMetadata(context.Background(), wfiID)

	// Verify findings are unchanged
	findings := getWFIFindings(t, env, wfiID)
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
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, retry_count, created_at, updated_at)
		VALUES ('wfi-cb-empty', ?, 'CB-EMPTY', 'test', 'active',
		        0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Clear callback metadata (should be a no-op)
	env.orch.clearCallbackMetadata(context.Background(), wfiID)

	// Verify findings are still empty
	findings := getWFIFindings(t, env, wfiID)
	if len(findings) != 0 {
		t.Errorf("expected empty findings, got %d keys", len(findings))
	}
}
