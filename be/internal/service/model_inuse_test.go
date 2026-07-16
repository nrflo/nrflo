package service

import (
	"testing"

	"be/internal/types"
)

func seedDualModeModel(t *testing.T, svc *ModelService, id string) *ModelService {
	t.Helper()
	if _, err := svc.Create(types.ModelCreateRequest{
		ID: id, Provider: "openai", DisplayName: id, CLIModel: id + "-cli", APIModel: id + "-api",
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}
	return svc
}

func seedProjectWorkflow(t *testing.T, svc *ModelService, projectID, workflowID, observerModel string) {
	t.Helper()
	now := svc.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := svc.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, '/tmp', ?, ?)`,
		projectID, projectID, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := svc.pool.Exec(
		`INSERT INTO workflows (project_id, id, scope_type, observer_model, created_at, updated_at) VALUES (?, ?, 'project', ?, ?, ?)`,
		projectID, workflowID, observerModel, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}

func seedAgentDef(t *testing.T, svc *ModelService, projectID, workflowID, agentID, mode, model, lowModel string) {
	t.Helper()
	now := svc.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := svc.pool.Exec(
		`INSERT INTO agent_definitions (project_id, workflow_id, id, model, low_consumption_model, execution_mode, prompt, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'p', ?, ?)`,
		projectID, workflowID, agentID, model, lowModel, mode, now, now); err != nil {
		t.Fatalf("seed agent def: %v", err)
	}
}

func TestModelInUseBlockedByWorkflowObserver(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "obs-wf")
	seedProjectWorkflow(t, svc, "p1", "wf1", "obs-wf")

	disabled := false
	if _, err := svc.Update("obs-wf", types.ModelUpdateRequest{Enabled: &disabled}); err == nil {
		t.Fatal("disable succeeded despite workflow observer ref")
	}
	if err := svc.Delete("obs-wf"); err == nil {
		t.Fatal("delete succeeded despite workflow observer ref")
	}
}

func TestModelInUseBlockedByGlobalObserverSetting(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "obs-global")
	if err := svc.pool.SetConfig(observerModelKey, "obs-global"); err != nil {
		t.Fatalf("set observer_model: %v", err)
	}

	disabled := false
	if _, err := svc.Update("obs-global", types.ModelUpdateRequest{Enabled: &disabled}); err == nil {
		t.Fatal("disable succeeded despite global observer setting")
	}
	if err := svc.Delete("obs-global"); err == nil {
		t.Fatal("delete succeeded despite global observer setting")
	}
}

func TestModelInUseBlockedByProjectObserverSetting(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "obs-project")
	if err := svc.pool.SetProjectConfig("proj-x", observerModelKey, "obs-project"); err != nil {
		t.Fatalf("set project observer_model: %v", err)
	}

	if err := svc.Delete("obs-project"); err == nil {
		t.Fatal("delete succeeded despite project observer setting")
	}
}

func TestModelClearCLIBlockedByCLIDef(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "clear-cli")
	seedProjectWorkflow(t, svc, "p2", "wf2", "")
	seedAgentDef(t, svc, "p2", "wf2", "a2", "cli_interactive", "clear-cli", "")

	empty := ""
	if _, err := svc.Update("clear-cli", types.ModelUpdateRequest{CLIModel: &empty}); err == nil {
		t.Fatal("clearing cli_model succeeded despite cli-mode def ref")
	}
}

func TestModelClearCLIBlockedByCLIDefLowConsumption(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "clear-cli-low")
	seedProjectWorkflow(t, svc, "p2b", "wf2b", "")
	// Referenced only via low_consumption_model on a cli-mode def.
	seedAgentDef(t, svc, "p2b", "wf2b", "a2b", "cli_interactive", "other-model", "clear-cli-low")

	empty := ""
	if _, err := svc.Update("clear-cli-low", types.ModelUpdateRequest{CLIModel: &empty}); err == nil {
		t.Fatal("clearing cli_model succeeded despite low_consumption_model ref")
	}
}

func TestModelClearCLIBlockedByCLISystemDef(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "clear-cli-sys")
	now := svc.clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	// System defs store 'cli_interactive' (migration 000105), not 'cli'.
	if _, err := svc.pool.Exec(
		`INSERT INTO system_agent_definitions (id, model, role, execution_mode, created_at, updated_at)
		 VALUES ('sys-clear-cli', 'clear-cli-sys', 'sys-clear', 'cli_interactive', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed system def: %v", err)
	}

	empty := ""
	if _, err := svc.Update("clear-cli-sys", types.ModelUpdateRequest{CLIModel: &empty}); err == nil {
		t.Fatal("clearing cli_model succeeded despite cli-mode system def ref")
	}
}

func TestModelClearAPIBlockedByAPIDef(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "clear-api")
	seedProjectWorkflow(t, svc, "p3", "wf3", "")
	seedAgentDef(t, svc, "p3", "wf3", "a3", "api", "clear-api", "")

	empty := ""
	if _, err := svc.Update("clear-api", types.ModelUpdateRequest{APIModel: &empty}); err == nil {
		t.Fatal("clearing api_model succeeded despite api-mode def ref")
	}
}

func TestModelClearModeWithNoRefsSucceeds(t *testing.T) {
	t.Parallel()
	svc := setupModelService(t)
	seedDualModeModel(t, svc, "clear-crossmode")
	seedProjectWorkflow(t, svc, "p4", "wf4", "")
	// Only an api-mode def references the model; clearing cli_model must succeed.
	seedAgentDef(t, svc, "p4", "wf4", "a4", "api", "clear-crossmode", "")

	empty := ""
	if _, err := svc.Update("clear-crossmode", types.ModelUpdateRequest{CLIModel: &empty}); err != nil {
		t.Fatalf("clearing cli_model with no cli refs failed: %v", err)
	}
}
