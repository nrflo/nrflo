package service

import (
	"strings"
	"testing"
	"time"

	"be/internal/types"
)

// insertTestProject inserts a minimal project row for in-use check tests.
func insertTestProject(t *testing.T, svc *CLIModelService, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := svc.pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'Test', ?, ?)`,
		projectID, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestProject(%q): %v", projectID, err)
	}
}

// insertTestWorkflow inserts a minimal workflow row for in-use check tests.
func insertTestWorkflow(t *testing.T, svc *CLIModelService, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := svc.pool.Exec(
		`INSERT INTO workflows (id, project_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		workflowID, projectID, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestWorkflow(%q, %q): %v", projectID, workflowID, err)
	}
}

// insertTestAgentDef inserts an agent_definitions row referencing specified models.
func insertTestAgentDef(t *testing.T, svc *CLIModelService, projectID, workflowID, agentID, modelID, lcModelID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := svc.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, low_consumption_model, prompt, timeout, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', 20, ?, ?)`,
		agentID, projectID, workflowID, modelID, lcModelID, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestAgentDef(%q, %q, %q): %v", projectID, workflowID, agentID, err)
	}
}

// insertTestSystemAgentDef inserts a system_agent_definitions row referencing a model.
func insertTestSystemAgentDef(t *testing.T, svc *CLIModelService, agentID, modelID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := svc.pool.Exec(
		`INSERT INTO system_agent_definitions (id, model, prompt, timeout, created_at, updated_at)
		 VALUES (?, ?, '', 20, ?, ?)`,
		agentID, modelID, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestSystemAgentDef(%q): %v", agentID, err)
	}
}

// --- ModelInUseCheck ---

func TestCLIModel_ModelInUseCheck_Unused(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "unused-model",
		CLIType:     "claude",
		DisplayName: "Unused",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.ModelInUseCheck("unused-model"); err != nil {
		t.Errorf("ModelInUseCheck(unused-model) = %v, want nil", err)
	}
}

func TestCLIModel_ModelInUseCheck_UsedByAgentDefModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "agent-model",
		CLIType:     "claude",
		DisplayName: "Agent Model",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestProject(t, svc, "proj-a")
	insertTestWorkflow(t, svc, "proj-a", "feature")
	insertTestAgentDef(t, svc, "proj-a", "feature", "implementor", "agent-model", "")

	err := svc.ModelInUseCheck("agent-model")
	if err == nil {
		t.Fatal("ModelInUseCheck expected error for model in agent_definitions.model, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
	if !strings.Contains(err.Error(), "proj-a/feature/implementor") {
		t.Errorf("error = %q, want to contain 'proj-a/feature/implementor'", err.Error())
	}
}

func TestCLIModel_ModelInUseCheck_UsedByLowConsumptionModel(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "lc-model",
		CLIType:     "claude",
		DisplayName: "Low Consumption",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestProject(t, svc, "proj-lc")
	insertTestWorkflow(t, svc, "proj-lc", "bugfix")
	insertTestAgentDef(t, svc, "proj-lc", "bugfix", "qa-verifier", "sonnet", "lc-model")

	err := svc.ModelInUseCheck("lc-model")
	if err == nil {
		t.Fatal("ModelInUseCheck expected error for model in agent_definitions.low_consumption_model, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
	if !strings.Contains(err.Error(), "proj-lc/bugfix/qa-verifier") {
		t.Errorf("error = %q, want to contain 'proj-lc/bugfix/qa-verifier'", err.Error())
	}
}

func TestCLIModel_ModelInUseCheck_UsedBySystemAgentDef(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "sys-model",
		CLIType:     "claude",
		DisplayName: "System Model",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestSystemAgentDef(t, svc, "test-system-agent", "sys-model")

	err := svc.ModelInUseCheck("sys-model")
	if err == nil {
		t.Fatal("ModelInUseCheck expected error for model in system_agent_definitions, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
	if !strings.Contains(err.Error(), "system/test-system-agent") {
		t.Errorf("error = %q, want to contain 'system/test-system-agent'", err.Error())
	}
}

func TestCLIModel_ModelInUseCheck_CaseInsensitive(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "ci-model",
		CLIType:     "claude",
		DisplayName: "Case Insensitive",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestProject(t, svc, "proj-ci")
	insertTestWorkflow(t, svc, "proj-ci", "feature")
	// Reference the model with mixed case in the agent def
	insertTestAgentDef(t, svc, "proj-ci", "feature", "implementor", "CI-MODEL", "")

	err := svc.ModelInUseCheck("ci-model")
	if err == nil {
		t.Fatal("ModelInUseCheck expected error for case-insensitive model reference, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
}

// --- Disable in-use model rejected ---

func TestCLIModel_DisableInUseModel_Rejected(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "inuse-custom",
		CLIType:     "claude",
		DisplayName: "In Use Custom",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestProject(t, svc, "proj-inuse")
	insertTestWorkflow(t, svc, "proj-inuse", "feature")
	insertTestAgentDef(t, svc, "proj-inuse", "feature", "implementor", "inuse-custom", "")

	_, err := svc.Update("inuse-custom", types.CLIModelUpdateRequest{Enabled: boolPtr(false)})
	if err == nil {
		t.Fatal("expected error when disabling in-use model, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}

	m, err := svc.Get("inuse-custom")
	if err != nil {
		t.Fatalf("Get after rejected disable: %v", err)
	}
	if !m.Enabled {
		t.Error("model disabled despite rejection, want still enabled")
	}
}

func TestCLIModel_DisableModel_ChecksBothModelAndLowConsumption(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "both-model",
		CLIType:     "claude",
		DisplayName: "Both Model",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestProject(t, svc, "proj-both")
	insertTestWorkflow(t, svc, "proj-both", "feature")
	// Reference only via low_consumption_model, not primary model
	insertTestAgentDef(t, svc, "proj-both", "feature", "qa-verifier", "sonnet", "both-model")

	_, err := svc.Update("both-model", types.CLIModelUpdateRequest{Enabled: boolPtr(false)})
	if err == nil {
		t.Fatal("expected error when disabling model used as low_consumption_model, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
}

func TestCLIModel_DisableModel_ChecksSystemAgentDefs(t *testing.T) {
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	if _, err := svc.Create(types.CLIModelCreateRequest{
		ID:          "syscheck-model",
		CLIType:     "claude",
		DisplayName: "Sys Check",
		MappedModel: "claude-custom",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	insertTestSystemAgentDef(t, svc, "syscheck-resolver", "syscheck-model")

	_, err := svc.Update("syscheck-model", types.CLIModelUpdateRequest{Enabled: boolPtr(false)})
	if err == nil {
		t.Fatal("expected error when disabling model used by system agent def, got nil")
	}
	if !strings.Contains(err.Error(), "model is in use") {
		t.Errorf("error = %q, want to contain 'model is in use'", err.Error())
	}
}
