package service

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// setupNodeIdentityTestEnv builds a project + workflow with two static agents
// (analyzer L0, builder L1) plus a planner and a fanout_template def sharing
// L0, and an active instance. Mirrors setupDeriveTestEnv (workflow_derive_test.go).
func setupNodeIdentityTestEnv(t *testing.T) (*db.Pool, *WorkflowService, *model.WorkflowInstance) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "node_identity_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	projectID := "ni-proj"
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err = pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}
	if _, err = pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES ('ni-wf', ?, '', 'ticket', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("workflow insert: %v", err)
	}

	for _, ad := range []struct {
		id       string
		layer    int
		nodeRole string
	}{
		{"analyzer", 0, "static"},
		{"builder", 1, "static"},
		{"plan-fanout", 0, "planner"},
		{"fanout-tmpl", 0, "fanout_template"},
	} {
		if _, err = pool.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, node_role, created_at, updated_at) VALUES (?, ?, 'ni-wf', '', ?, ?, ?, ?)`,
			ad.id, projectID, ad.layer, ad.nodeRole, now, now); err != nil {
			t.Fatalf("agent_definition insert %s: %v", ad.id, err)
		}
	}

	wfiID := "ni-wfi"
	if _, err = pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', 'ni-wf', 'ticket', 'active', 0, ?, ?)`,
		wfiID, projectID, now, now); err != nil {
		t.Fatalf("workflow_instance insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())
	wi, err := svc.wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("get workflow instance: %v", err)
	}
	return pool, svc, wi
}

// TestGetWorkflowDef_ExcludesPlannerAndFanoutTemplate verifies GetWorkflowDef's
// Phases list contains only the static defs — a planner/fanout_template def
// never becomes a phase.
func TestGetWorkflowDef_ExcludesPlannerAndFanoutTemplate(t *testing.T) {
	t.Parallel()
	_, svc, wi := setupNodeIdentityTestEnv(t)

	wf, err := svc.GetWorkflowDef(wi.ProjectID, wi.WorkflowID)
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(wf.Phases) != 2 {
		t.Fatalf("Phases count = %d, want 2 (planner/fanout_template must be excluded)", len(wf.Phases))
	}
	for _, p := range wf.Phases {
		if p.NodeID == "plan-fanout" || p.NodeID == "fanout-tmpl" {
			t.Errorf("Phases contains non-static def %q", p.NodeID)
		}
	}
}

// TestNodeIdentity_ReadModelKeyedOnNodeID verifies the v4 read model
// (phase_order, phase_layers, phases) is keyed on node_id for a static
// workflow — where node_id == agent_type == the agent_definitions id, so the
// keys are byte-identical to what the old agent_type-based keying produced.
// It also proves the planner/fanout_template defs never leak into the derived
// phase map.
func TestNodeIdentity_ReadModelKeyedOnNodeID(t *testing.T) {
	t.Parallel()
	pool, svc, wi := setupNodeIdentityTestEnv(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, result, restart_count, created_at, updated_at)
		VALUES ('sess-analyzer', ?, '', ?, 'analyzer', 'analyzer', 'analyzer', 'completed', 'pass', 0, ?, ?)`,
		wi.ProjectID, wi.ID, now, now); err != nil {
		t.Fatalf("insert analyzer session: %v", err)
	}
	if _, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, restart_count, created_at, updated_at)
		VALUES ('sess-builder', ?, '', ?, 'builder', 'builder', 'builder', 'running', 0, ?, ?)`,
		wi.ProjectID, wi.ID, now, now); err != nil {
		t.Fatalf("insert builder session: %v", err)
	}

	status, err := svc.GetStatusByInstance(wi)
	if err != nil {
		t.Fatalf("GetStatusByInstance: %v", err)
	}

	phaseOrder, ok := status["phase_order"].([]string)
	if !ok {
		t.Fatalf("phase_order type = %T, want []string", status["phase_order"])
	}
	wantOrder := []string{"analyzer", "builder"}
	if len(phaseOrder) != len(wantOrder) {
		t.Fatalf("phase_order = %v, want %v", phaseOrder, wantOrder)
	}
	for i, want := range wantOrder {
		if phaseOrder[i] != want {
			t.Errorf("phase_order[%d] = %q, want %q", i, phaseOrder[i], want)
		}
	}

	phaseLayers, ok := status["phase_layers"].(map[string]int)
	if !ok {
		t.Fatalf("phase_layers type = %T, want map[string]int", status["phase_layers"])
	}
	if phaseLayers["analyzer"] != 0 || phaseLayers["builder"] != 1 {
		t.Errorf("phase_layers = %v, want {analyzer:0, builder:1}", phaseLayers)
	}
	if _, exists := phaseLayers["plan-fanout"]; exists {
		t.Error("phase_layers contains planner def, want excluded")
	}
	if _, exists := phaseLayers["fanout-tmpl"]; exists {
		t.Error("phase_layers contains fanout_template def, want excluded")
	}

	phases, ok := status["phases"].(map[string]model.PhaseStatus)
	if !ok {
		t.Fatalf("phases type = %T, want map[string]model.PhaseStatus", status["phases"])
	}
	if len(phases) != 2 {
		t.Fatalf("phases map len = %d, want 2 (byte-identical key set for a static workflow)", len(phases))
	}
	analyzerStatus, ok := phases["analyzer"]
	if !ok {
		t.Fatal("phases map missing key \"analyzer\"")
	}
	if analyzerStatus.Status != "completed" || analyzerStatus.Result != "pass" {
		t.Errorf("phases[analyzer] = %+v, want {completed, pass}", analyzerStatus)
	}
	builderStatus, ok := phases["builder"]
	if !ok {
		t.Fatal("phases map missing key \"builder\"")
	}
	if builderStatus.Status != "in_progress" {
		t.Errorf("phases[builder] = %+v, want {in_progress}", builderStatus)
	}
}
