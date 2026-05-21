package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedProjectAndWorkflow inserts the minimal FK rows needed for agent_definition inserts.
func seedProjectAndWorkflow(t *testing.T, pool *db.Pool, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("insert project %s: %v", projectID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		workflowID, projectID, now, now); err != nil {
		t.Fatalf("insert workflow %s: %v", workflowID, err)
	}
}

// TestListExecutable_ExcludesConsultant verifies that ListExecutable returns only
// non-consultant defs while List returns all defs.
func TestListExecutable_ExcludesConsultant(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj", "wf")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "real-agent", ProjectID: "proj", WorkflowID: "wf",
		ExecutionMode: "cli_interactive", Layer: 0, Consultant: false,
	}); err != nil {
		t.Fatalf("create real agent: %v", err)
	}
	if err := r.Create(&model.AgentDefinition{
		ID: "consultant-agent", ProjectID: "proj", WorkflowID: "wf",
		ExecutionMode: "api", Layer: 0, Consultant: true,
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	all, err := r.List("proj", "wf")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List count = %d, want 2", len(all))
	}

	executable, err := r.ListExecutable("proj", "wf")
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}
	if len(executable) != 1 {
		t.Fatalf("ListExecutable count = %d, want 1", len(executable))
	}
	if executable[0].ID != "real-agent" {
		t.Errorf("ListExecutable[0].ID = %q, want real-agent", executable[0].ID)
	}
	if executable[0].Consultant {
		t.Error("ListExecutable returned a consultant def, want none")
	}
}

// TestListExecutable_EmptyWhenAllConsultants verifies that ListExecutable returns an
// empty slice when the workflow contains only consultant defs.
func TestListExecutable_EmptyWhenAllConsultants(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj2", "wf2")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	if err := r.Create(&model.AgentDefinition{
		ID: "cons-only", ProjectID: "proj2", WorkflowID: "wf2",
		ExecutionMode: "api", Layer: 0, Consultant: true,
	}); err != nil {
		t.Fatalf("create consultant agent: %v", err)
	}

	all, err := r.List("proj2", "wf2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List count = %d, want 1", len(all))
	}

	executable, err := r.ListExecutable("proj2", "wf2")
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}
	if len(executable) != 0 {
		t.Errorf("ListExecutable count = %d, want 0 (all are consultants)", len(executable))
	}
}

// TestListExecutable_MultiLayerMixed verifies ordering and completeness when
// consultants and real agents are interleaved across layers.
func TestListExecutable_MultiLayerMixed(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj3", "wf3")

	r := NewAgentDefinitionRepo(pool, clock.Real())

	defs := []model.AgentDefinition{
		{ID: "real-l0", ProjectID: "proj3", WorkflowID: "wf3", ExecutionMode: "cli_interactive", Layer: 0, Consultant: false},
		{ID: "cons-l0", ProjectID: "proj3", WorkflowID: "wf3", ExecutionMode: "api", Layer: 0, Consultant: true},
		{ID: "real-l1", ProjectID: "proj3", WorkflowID: "wf3", ExecutionMode: "cli_interactive", Layer: 1, Consultant: false},
		{ID: "cons-l1", ProjectID: "proj3", WorkflowID: "wf3", ExecutionMode: "api", Layer: 1, Consultant: true},
	}
	for i := range defs {
		if err := r.Create(&defs[i]); err != nil {
			t.Fatalf("create %s: %v", defs[i].ID, err)
		}
	}

	all, err := r.List("proj3", "wf3")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("List count = %d, want 4", len(all))
	}

	executable, err := r.ListExecutable("proj3", "wf3")
	if err != nil {
		t.Fatalf("ListExecutable: %v", err)
	}
	if len(executable) != 2 {
		t.Fatalf("ListExecutable count = %d, want 2", len(executable))
	}
	for _, d := range executable {
		if d.Consultant {
			t.Errorf("ListExecutable returned consultant def %q", d.ID)
		}
	}
	// Verify ordering: layer ASC, id ASC
	if executable[0].ID != "real-l0" || executable[1].ID != "real-l1" {
		t.Errorf("ListExecutable order = [%s, %s], want [real-l0, real-l1]",
			executable[0].ID, executable[1].ID)
	}
}
