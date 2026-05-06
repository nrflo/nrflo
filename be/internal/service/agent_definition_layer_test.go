package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupLayerTestEnv creates an isolated DB with a project and workflow (no groups) for layer tests.
func setupLayerTestEnv(t *testing.T) (*db.Pool, *AgentDefinitionService, *WorkflowService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "layer_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj1', 'P', '/tmp', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	wfSvc := NewWorkflowService(pool, clock.Real())
	_, err = wfSvc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID: "wf1",
	})
	if err != nil {
		t.Fatalf("workflow create: %v", err)
	}

	cliModelSvc := NewCLIModelService(pool, clock.Real())
	svc := NewAgentDefinitionService(pool, clock.Real(), cliModelSvc, nil, false)
	return pool, svc, wfSvc
}

func TestCreateAgentDef_LayerStored(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	def, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "setup", Prompt: "analyze", Layer: 0,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.Layer != 0 {
		t.Errorf("Layer = %d, want 0", def.Layer)
	}

	def2, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "build", Prompt: "build", Layer: 5,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def2.Layer != 5 {
		t.Errorf("Layer = %d, want 5", def2.Layer)
	}
}

func TestGetAgentDef_ReturnsLayer(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	_, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "agent-l3", Prompt: "p", Layer: 3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", "wf1", "agent-l3")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.Layer != 3 {
		t.Errorf("GetAgentDef Layer = %d, want 3", def.Layer)
	}
}

func TestListAgentDefs_OrderedByLayerThenID(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	agents := []types.AgentDefCreateRequest{
		{ID: "zebra", Prompt: "p", Layer: 1},
		{ID: "alpha", Prompt: "p", Layer: 1},
		{ID: "setup", Prompt: "p", Layer: 0},
	}
	for _, a := range agents {
		if _, err := svc.CreateAgentDef("proj1", "wf1", &a); err != nil {
			t.Fatalf("create %s: %v", a.ID, err)
		}
	}

	defs, err := svc.ListAgentDefs("proj1", "wf1")
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3, got %d", len(defs))
	}

	// Expect: setup(L0), alpha(L1), zebra(L1)
	want := []struct {
		id    string
		layer int
	}{
		{"setup", 0}, {"alpha", 1}, {"zebra", 1},
	}
	for i, w := range want {
		if defs[i].ID != w.id || defs[i].Layer != w.layer {
			t.Errorf("defs[%d] = {%s, L%d}, want {%s, L%d}", i, defs[i].ID, defs[i].Layer, w.id, w.layer)
		}
	}
}

func TestUpdateAgentDef_UpdatesLayer(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	_, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "mover", Prompt: "p", Layer: 0,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newLayer := 2
	if err := svc.UpdateAgentDef("proj1", "wf1", "mover", &types.AgentDefUpdateRequest{
		Layer: &newLayer,
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", "wf1", "mover")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if def.Layer != 2 {
		t.Errorf("Layer after update = %d, want 2", def.Layer)
	}
}

func TestCreateAgentDef_NegativeLayerRejected(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	_, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "neg", Prompt: "p", Layer: -1,
	})
	if err == nil {
		t.Fatal("expected error for negative layer, got nil")
	}
}

func TestGetWorkflowDef_PhasesDerivedFromAgentDefs(t *testing.T) {
	t.Parallel()
	_, svc, wfSvc := setupLayerTestEnv(t)

	// Create agents with various layers
	agents := []types.AgentDefCreateRequest{
		{ID: "setup", Prompt: "p", Layer: 0},
		{ID: "build", Prompt: "p", Layer: 1},
		{ID: "verify", Prompt: "p", Layer: 2},
	}
	for _, a := range agents {
		if _, err := svc.CreateAgentDef("proj1", "wf1", &a); err != nil {
			t.Fatalf("create %s: %v", a.ID, err)
		}
	}

	wf, err := wfSvc.GetWorkflowDef("proj1", "wf1")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}

	if len(wf.Phases) != 3 {
		t.Fatalf("Phases count = %d, want 3", len(wf.Phases))
	}

	for i, want := range []struct {
		agent string
		layer int
	}{
		{"setup", 0}, {"build", 1}, {"verify", 2},
	} {
		if wf.Phases[i].Agent != want.agent || wf.Phases[i].Layer != want.layer {
			t.Errorf("Phases[%d] = {%s, L%d}, want {%s, L%d}", i, wf.Phases[i].Agent, wf.Phases[i].Layer, want.agent, want.layer)
		}
	}
}

func TestGetWorkflowDef_EmptyAgentDefsReturnsEmptyPhases(t *testing.T) {
	t.Parallel()
	_, _, wfSvc := setupLayerTestEnv(t)

	wf, err := wfSvc.GetWorkflowDef("proj1", "wf1")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(wf.Phases) != 0 {
		t.Errorf("expected 0 phases for workflow with no agents, got %d", len(wf.Phases))
	}
}

func TestListWorkflowDefs_PhasesDerivedFromAgentDefs(t *testing.T) {
	t.Parallel()
	_, svc, wfSvc := setupLayerTestEnv(t)

	if _, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "analyzer", Prompt: "p", Layer: 0,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	defs, err := wfSvc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}

	wf, ok := defs["wf1"]
	if !ok {
		t.Fatal("wf1 not found in listed defs")
	}
	if len(wf.Phases) != 1 {
		t.Fatalf("Phases count = %d, want 1", len(wf.Phases))
	}
	if wf.Phases[0].Agent != "analyzer" || wf.Phases[0].Layer != 0 {
		t.Errorf("Phases[0] = {%s, L%d}, want {analyzer, L0}", wf.Phases[0].Agent, wf.Phases[0].Layer)
	}
}

// TestAgentDefParallelToParallelAllowed verifies that [A,B]->[C,D] topologies are accepted.
// Parallel agents feeding into parallel agents was previously forbidden; the restriction
// has been removed and all layer->=0 topologies are valid.
func TestAgentDefParallelToParallelAllowed(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	agents := []types.AgentDefCreateRequest{
		{ID: "setup-a", Prompt: "p", Layer: 0},
		{ID: "setup-b", Prompt: "p", Layer: 0},
		{ID: "verify-a", Prompt: "p", Layer: 1},
		{ID: "verify-b", Prompt: "p", Layer: 1},
	}
	for _, a := range agents {
		if _, err := svc.CreateAgentDef("proj1", "wf1", &a); err != nil {
			t.Errorf("parallel-to-parallel topology must be accepted, agent %s: %v", a.ID, err)
		}
	}
}

// TestUpdateAgentDef_NegativeLayerRejected verifies that updating an agent to a
// negative layer value is rejected.
func TestUpdateAgentDef_NegativeLayerRejected(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupLayerTestEnv(t)

	if _, err := svc.CreateAgentDef("proj1", "wf1", &types.AgentDefCreateRequest{
		ID: "agent", Prompt: "p", Layer: 1,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	neg := -1
	err := svc.UpdateAgentDef("proj1", "wf1", "agent", &types.AgentDefUpdateRequest{
		Layer: &neg,
	})
	if err == nil {
		t.Fatal("expected error when updating to negative layer, got nil")
	}
}
