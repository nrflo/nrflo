package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// exportTestEnv groups all services used in export/import tests.
type exportTestEnv struct {
	pool            *db.Pool
	exportSvc       *WorkflowExportService
	workflowSvc     *WorkflowService
	agentSvc        *AgentDefinitionService
	layerPolicySvc  *WorkflowLayerPolicyService
	notifySvc       *NotificationService
	pythonScriptSvc *PythonScriptService
	projectID       string
}

// setupExportImportEnv creates an isolated DB and instantiates all services for
// export/import testing. The returned env is scoped to projectID "proj1".
func setupExportImportEnv(t *testing.T) *exportTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "export_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj1', 'P', '/tmp', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed proj1: %v", err)
	}

	clk := clock.Real()
	wfSvc := NewWorkflowService(pool, clk)
	cliModelSvc := NewCLIModelService(pool, clk)
	scriptRepo := repo.NewPythonScriptRepo(pool, clk)
	agentSvc := NewAgentDefinitionService(pool, clk, cliModelSvc, scriptRepo)
	lpSvc := NewWorkflowLayerPolicyService(pool, clk)
	notifySvc := NewNotificationService(pool, clk, nil, nil, wfSvc)
	pythonScriptSvc := NewPythonScriptService(pool, clk)
	exportSvc := NewWorkflowExportService(pool, clk, wfSvc, agentSvc, lpSvc, notifySvc, pythonScriptSvc)

	return &exportTestEnv{
		pool:            pool,
		exportSvc:       exportSvc,
		workflowSvc:     wfSvc,
		agentSvc:        agentSvc,
		layerPolicySvc:  lpSvc,
		notifySvc:       notifySvc,
		pythonScriptSvc: pythonScriptSvc,
		projectID:       "proj1",
	}
}

// createSimpleWorkflow seeds a workflow with one cli_interactive agent in layer 0.
func (e *exportTestEnv) createSimpleWorkflow(t *testing.T, wfID string) {
	t.Helper()
	if _, err := e.workflowSvc.CreateWorkflowDef(e.projectID, &types.WorkflowDefCreateRequest{
		ID:          wfID,
		Description: "simple test workflow",
	}); err != nil {
		t.Fatalf("createSimpleWorkflow(%q) CreateWorkflowDef: %v", wfID, err)
	}
	if _, err := e.agentSvc.CreateAgentDef(e.projectID, wfID, &types.AgentDefCreateRequest{
		ID:     "agent-a",
		Layer:  0,
		Prompt: "do the thing",
	}); err != nil {
		t.Fatalf("createSimpleWorkflow(%q) CreateAgentDef: %v", wfID, err)
	}
}

// seedProject2 inserts project "proj2" into the shared pool.
func (e *exportTestEnv) seedProject2(t *testing.T) string {
	t.Helper()
	if _, err := e.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj2', 'Q', '/tmp2', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seedProject2: %v", err)
	}
	return "proj2"
}

// --- Export tests ---

func TestExport_StripProjectIDAndWorkflowID(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-strip")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-strip"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Fatalf("bundle.Workflows len = %d, want 1", len(bundle.Workflows))
	}
	entry := bundle.Workflows[0]
	if entry.Workflow.ProjectID != "" {
		t.Errorf("Workflow.ProjectID = %q, want empty (stripped)", entry.Workflow.ProjectID)
	}
	for _, a := range entry.Agents {
		if a.ProjectID != "" {
			t.Errorf("Agent[%s].ProjectID = %q, want empty", a.ID, a.ProjectID)
		}
		if a.WorkflowID != "" {
			t.Errorf("Agent[%s].WorkflowID = %q, want empty", a.ID, a.WorkflowID)
		}
	}
}

func TestExport_IncludesPythonScript(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	script, err := env.pythonScriptSvc.Create(env.projectID, &types.PythonScriptCreateRequest{
		Name: "script-a",
		Code: "print('hello')",
	})
	if err != nil {
		t.Fatalf("Create script: %v", err)
	}
	if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{
		ID: "wf-sc-inc",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}
	if _, err := env.agentSvc.CreateAgentDef(env.projectID, "wf-sc-inc", &types.AgentDefCreateRequest{
		ID:             "agent-sc",
		Layer:          0,
		ExecutionMode:  "script",
		PythonScriptID: &script.ID,
	}); err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-sc-inc"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(bundle.PythonScripts) != 1 {
		t.Fatalf("PythonScripts len = %d, want 1", len(bundle.PythonScripts))
	}
	if bundle.PythonScripts[0].ID != script.ID {
		t.Errorf("PythonScript.ID = %q, want %q", bundle.PythonScripts[0].ID, script.ID)
	}
	if bundle.PythonScripts[0].ProjectID != "" {
		t.Errorf("PythonScript.ProjectID = %q, want empty (stripped)", bundle.PythonScripts[0].ProjectID)
	}
}

func TestExport_MultiWorkflow_DedupesPythonScripts(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	script, err := env.pythonScriptSvc.Create(env.projectID, &types.PythonScriptCreateRequest{
		Name: "shared-script",
		Code: "print('shared')",
	})
	if err != nil {
		t.Fatalf("Create shared script: %v", err)
	}
	for _, wfID := range []string{"wf-dedup-a", "wf-dedup-b"} {
		if _, err := env.workflowSvc.CreateWorkflowDef(env.projectID, &types.WorkflowDefCreateRequest{ID: wfID}); err != nil {
			t.Fatalf("CreateWorkflowDef(%q): %v", wfID, err)
		}
		if _, err := env.agentSvc.CreateAgentDef(env.projectID, wfID, &types.AgentDefCreateRequest{
			ID:             "ag-" + wfID,
			Layer:          0,
			ExecutionMode:  "script",
			PythonScriptID: &script.ID,
		}); err != nil {
			t.Fatalf("CreateAgentDef(%q): %v", wfID, err)
		}
	}

	bundle, err := env.exportSvc.Export(env.projectID, nil)
	if err != nil {
		t.Fatalf("Export all: %v", err)
	}
	if len(bundle.PythonScripts) != 1 {
		t.Errorf("PythonScripts len = %d, want 1 (deduped)", len(bundle.PythonScripts))
	}
	if len(bundle.Workflows) != 2 {
		t.Errorf("Workflows len = %d, want 2", len(bundle.Workflows))
	}
}

func TestExport_ExcludesReservedWorkflows(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-visible")

	if _, err := env.pool.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('__spec_import__', 'proj1', '', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("insert reserved workflow: %v", err)
	}

	bundle, err := env.exportSvc.Export(env.projectID, nil)
	if err != nil {
		t.Fatalf("Export all: %v", err)
	}
	if len(bundle.Workflows) != 1 {
		t.Errorf("Workflows len = %d, want 1 (reserved excluded)", len(bundle.Workflows))
	}
	for _, entry := range bundle.Workflows {
		if entry.Workflow != nil && IsReservedWorkflowName(entry.Workflow.ID) {
			t.Errorf("bundle includes reserved workflow %q", entry.Workflow.ID)
		}
	}
}

func TestExport_NonExistentWorkflow_Error(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)

	_, err := env.exportSvc.Export(env.projectID, []string{"no-such-wf"})
	if err == nil {
		t.Fatal("Export of non-existent workflow: expected error, got nil")
	}
}

func TestExport_BundleVersion(t *testing.T) {
	t.Parallel()
	env := setupExportImportEnv(t)
	env.createSimpleWorkflow(t, "wf-ver")

	bundle, err := env.exportSvc.Export(env.projectID, []string{"wf-ver"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if bundle.Version != "1.0" {
		t.Errorf("bundle.Version = %q, want \"1.0\"", bundle.Version)
	}
	if bundle.ExportedAt == "" {
		t.Error("bundle.ExportedAt is empty")
	}
}
