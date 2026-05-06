package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedProjAndWorkflow inserts a project and workflow row for FK satisfaction.
func seedProjAndWorkflow(t *testing.T, pool *db.Pool, projectID, workflowID string) {
	t.Helper()
	now := "2024-01-01T00:00:00Z"
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, "Test Project", "/tmp/test", now, now,
	); err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		workflowID, projectID, "Test Workflow", "ticket", now, now,
	); err != nil {
		t.Fatalf("seed workflow %q: %v", workflowID, err)
	}
}

func TestWorkflowLayerPolicyRepo_Upsert_Insert(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(t0))

	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "any",
	}); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow() error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByWorkflow() returned %d rows, want 1", len(rows))
	}
	if rows[0].PassPolicy != "any" {
		t.Errorf("PassPolicy = %q, want \"any\"", rows[0].PassPolicy)
	}
	if !rows[0].CreatedAt.Equal(t0) {
		t.Errorf("CreatedAt = %v, want %v", rows[0].CreatedAt, t0)
	}
	if !rows[0].UpdatedAt.Equal(t0) {
		t.Errorf("UpdatedAt = %v, want %v", rows[0].UpdatedAt, t0)
	}
}

func TestWorkflowLayerPolicyRepo_Upsert_Update(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(t0)
	r := NewWorkflowLayerPolicyRepo(pool, clk)

	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "any",
	}); err != nil {
		t.Fatalf("Upsert (insert) error: %v", err)
	}

	t1 := t0.Add(time.Minute)
	clk.Set(t1)
	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "quorum:1",
	}); err != nil {
		t.Fatalf("Upsert (update) error: %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow() error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListByWorkflow() returned %d rows, want 1", len(rows))
	}
	if rows[0].PassPolicy != "quorum:1" {
		t.Errorf("PassPolicy = %q, want \"quorum:1\"", rows[0].PassPolicy)
	}
	if !rows[0].CreatedAt.Equal(t0) {
		t.Errorf("CreatedAt = %v, want %v (must not change on update)", rows[0].CreatedAt, t0)
	}
	if !rows[0].UpdatedAt.Equal(t1) {
		t.Errorf("UpdatedAt = %v, want %v", rows[0].UpdatedAt, t1)
	}
}

func TestWorkflowLayerPolicyRepo_Delete(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "all",
	}); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	if err := r.Delete("proj1", "wf1", 0); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow() after delete error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("ListByWorkflow() after delete = %d rows, want 0", len(rows))
	}
}

func TestWorkflowLayerPolicyRepo_Delete_NonExistent(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	if err := r.Delete("proj1", "wf1", 99); err != nil {
		t.Errorf("Delete(non-existent) error = %v, want nil", err)
	}
}

func TestWorkflowLayerPolicyRepo_ListByWorkflow_LayerAscOrder(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	for _, layer := range []int{2, 0, 1} {
		if err := r.Upsert(&model.WorkflowLayerPolicy{
			ProjectID:  "proj1",
			WorkflowID: "wf1",
			Layer:      layer,
			PassPolicy: "any",
		}); err != nil {
			t.Fatalf("Upsert(layer=%d) error: %v", layer, err)
		}
	}

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow() error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("ListByWorkflow() returned %d rows, want 3", len(rows))
	}
	for i, wantLayer := range []int{0, 1, 2} {
		if rows[i].Layer != wantLayer {
			t.Errorf("rows[%d].Layer = %d, want %d", i, rows[i].Layer, wantLayer)
		}
	}
}

func TestWorkflowLayerPolicyRepo_ListByWorkflow_Empty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	rows, err := r.ListByWorkflow("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListByWorkflow() error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("ListByWorkflow() for fresh workflow = %d rows, want 0", len(rows))
	}
}

func TestWorkflowLayerPolicyRepo_CascadeOnWorkflowDelete(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjAndWorkflow(t, pool, "proj1", "wf1")

	r := NewWorkflowLayerPolicyRepo(pool, clock.NewTest(time.Now().UTC()))

	if err := r.Upsert(&model.WorkflowLayerPolicy{
		ProjectID:  "proj1",
		WorkflowID: "wf1",
		Layer:      0,
		PassPolicy: "any",
	}); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	if _, err := pool.Exec(
		`DELETE FROM workflows WHERE id = ? AND project_id = ?`, "wf1", "proj1",
	); err != nil {
		t.Fatalf("DELETE workflow error: %v", err)
	}

	var count int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM workflow_layer_policies WHERE workflow_id = ?`, "wf1",
	).Scan(&count); err != nil {
		t.Fatalf("count query error: %v", err)
	}
	if count != 0 {
		t.Errorf("workflow_layer_policies count after workflow delete = %d, want 0 (cascade)", count)
	}
}
