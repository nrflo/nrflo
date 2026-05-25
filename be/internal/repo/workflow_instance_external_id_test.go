package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestCreate_ExternalID_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("with_external_id_and_context", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(
			`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-ext", "Ext Project", t.TempDir(), now, now)
		if err != nil {
			t.Fatalf("seed project: %v", err)
		}
		_, err = pool.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-ext", "proj-ext", "Ext Workflow", "project", now, now)
		if err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		r := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:              "wfi-ext-1",
			ProjectID:       "proj-ext",
			WorkflowID:      "wf-ext",
			ScopeType:       "project",
			Status:          model.WorkflowInstanceActive,
			ExternalID:      "ext-abc-123",
			ExternalContext: `{"source":"github","issue":42}`,
		}
		if err := r.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := r.Get("wfi-ext-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "ext-abc-123" {
			t.Errorf("ExternalID = %q, want %q", got.ExternalID, "ext-abc-123")
		}
		if got.ExternalContext != `{"source":"github","issue":42}` {
			t.Errorf("ExternalContext = %q, want %q", got.ExternalContext, `{"source":"github","issue":42}`)
		}
	})

	t.Run("empty_external_id_and_context_are_null", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := pool.Exec(
			`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-noext", "No Ext", t.TempDir(), now, now)
		if err != nil {
			t.Fatalf("seed project: %v", err)
		}
		_, err = pool.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-noext", "proj-noext", "No Ext Workflow", "project", now, now)
		if err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		r := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:         "wfi-noext-1",
			ProjectID:  "proj-noext",
			WorkflowID: "wf-noext",
			ScopeType:  "project",
			Status:     model.WorkflowInstanceActive,
			// ExternalID and ExternalContext intentionally empty
		}
		if err := r.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Verify both columns are NULL via raw query
		var extIDNull, extCtxNull bool
		row := pool.QueryRow(
			`SELECT external_id IS NULL, external_context IS NULL FROM workflow_instances WHERE id = ?`,
			"wfi-noext-1")
		if err := row.Scan(&extIDNull, &extCtxNull); err != nil {
			t.Fatalf("raw NULL check: %v", err)
		}
		if !extIDNull {
			t.Error("external_id should be NULL when ExternalID is empty, but is not")
		}
		if !extCtxNull {
			t.Error("external_context should be NULL when ExternalContext is empty, but is not")
		}

		// Verify repo.Get returns empty strings
		got, err := r.Get("wfi-noext-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "" {
			t.Errorf("ExternalID = %q, want empty string", got.ExternalID)
		}
		if got.ExternalContext != "" {
			t.Errorf("ExternalContext = %q, want empty string", got.ExternalContext)
		}
	})
}
