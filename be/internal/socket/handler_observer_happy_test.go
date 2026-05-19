package socket

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/service"
)

// TestObserverGlobalHealth_DefaultDisabled verifies health response when observer is off.
func TestObserverGlobalHealth_DefaultDisabled(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	seedObserverSession(t, env, "obs-health-off", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.global.health", map[string]interface{}{
		"session_id": "obs-health-off",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}
	dbOK, _ := result["db"].(bool)
	if !dbOK {
		t.Errorf("db = %v, want true", result["db"])
	}
	enabled, _ := result["observer_enabled"].(bool)
	if enabled {
		t.Errorf("observer_enabled = true, want false (default)")
	}
}

// TestObserverGlobalHealth_EnabledReflected verifies health reflects the enabled flag.
func TestObserverGlobalHealth_EnabledReflected(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	enableObserver(t, env)
	seedObserverSession(t, env, "obs-health-on", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.global.health", map[string]interface{}{
		"session_id": "obs-health-on",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var result map[string]interface{}
	json.Unmarshal(resp.Result, &result) //nolint:errcheck
	if enabled, _ := result["observer_enabled"].(bool); !enabled {
		t.Errorf("observer_enabled = false, want true after enabling")
	}
}

// TestObserverGlobalProjects_HappyPath verifies global.projects returns the seeded project.
func TestObserverGlobalProjects_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	seedObserverSession(t, env, "obs-global-projs", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.global.projects", map[string]interface{}{
		"session_id": "obs-global-projs",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	var projects []map[string]interface{}
	if err := json.Unmarshal(resp.Result, &projects); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("expected at least one project, got empty")
	}
	found := false
	for _, p := range projects {
		if p["id"] == env.project {
			found = true
		}
	}
	if !found {
		t.Errorf("seeded project %q not found in projects list", env.project)
	}
}

// TestObserverProjectWorkflows_HappyPath verifies project.workflows matches service output.
func TestObserverProjectWorkflows_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-WF")
	wfiID := wfiIDForTicket(t, env, "TEST-WF")

	seedObserverSession(t, env, "obs-proj-wfs", "project", env.project, wfiID)

	resp := env.handler.Handle(obsReq("observer.project.workflows", map[string]interface{}{
		"session_id": "obs-proj-wfs",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	wfSvc := service.NewWorkflowService(env.pool, clock.Real())
	expected, err := wfSvc.ListWorkflowDefs(env.project)
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}

	// ListWorkflowDefs returns map[string]WorkflowDef keyed by workflow ID.
	var defs map[string]interface{}
	if err := json.Unmarshal(resp.Result, &defs); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(defs) != len(expected) {
		t.Errorf("got %d workflow defs, want %d", len(defs), len(expected))
	}
	if _, ok := defs["test"]; !ok {
		t.Errorf("workflow key 'test' not found in result map (keys: %v)", defs)
	}
}

// TestObserverWorkflowShow_HappyPath verifies workflow.show resolves from session WFI.
func TestObserverWorkflowShow_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-SHOW")
	wfiID := wfiIDForTicket(t, env, "TEST-SHOW")

	seedObserverSession(t, env, "obs-wf-show", "workflow", env.project, wfiID)

	resp := env.handler.Handle(obsReq("observer.workflow.show", map[string]interface{}{
		"session_id": "obs-wf-show",
		// no workflow_id — resolveWorkflowScope defaults from session's WFI
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	// WorkflowDef does not serialize its own ID; check a stable field.
	var def map[string]interface{}
	if err := json.Unmarshal(resp.Result, &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if def["description"] != "Test workflow" {
		t.Errorf("workflow def description = %v, want 'Test workflow'", def["description"])
	}
}

// TestObserverWorkflowFindings_HappyPath verifies workflow.findings returns for a fresh WFI.
func TestObserverWorkflowFindings_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-FIND")
	wfiID := wfiIDForTicket(t, env, "TEST-FIND")

	seedObserverSession(t, env, "obs-wf-find", "workflow", env.project, wfiID)

	resp := env.handler.Handle(obsReq("observer.workflow.findings", map[string]interface{}{
		"session_id": "obs-wf-find",
	}))
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if len(resp.Result) == 0 {
		t.Error("expected non-empty result for findings")
	}
}

// TestObserverNilRunner_Trigger verifies internal error when workflowRunner is nil.
func TestObserverNilRunner_Trigger(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	enableObserver(t, env)
	// Global-scope can call workflow.trigger; nil check fires before resolveWorkflowScope.
	seedObserverSession(t, env, "obs-nil-trigger", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.workflow.trigger", map[string]interface{}{
		"session_id": "obs-nil-trigger",
		"project_id": env.project,
		"workflow_id": "test",
	}))
	if resp.Error == nil {
		t.Fatal("expected internal error for nil runner, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
	if resp.Error.Message != "workflow runner not available" {
		t.Errorf("message = %q, want 'workflow runner not available'", resp.Error.Message)
	}
}

// TestObserverNilRunner_RetryFailed verifies internal error when workflowRunner is nil.
func TestObserverNilRunner_RetryFailed(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	enableObserver(t, env)
	// Nil check in obsWorkflowRetryFailed fires before WFI load, so wfiID="" is safe.
	seedObserverSession(t, env, "obs-nil-retry", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.workflow.retry_failed", map[string]interface{}{
		"session_id": "obs-nil-retry",
	}))
	if resp.Error == nil {
		t.Fatal("expected internal error for nil runner, got nil")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("code = %d, want %d (internal)", resp.Error.Code, ErrCodeInternal)
	}
	if resp.Error.Message != "workflow runner not available" {
		t.Errorf("message = %q, want 'workflow runner not available'", resp.Error.Message)
	}
}

// TestObserverProjectEnvSetUnset_HappyPath verifies env set→list→unset round-trip.
func TestObserverProjectEnvSetUnset_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	enableObserver(t, env)
	env.createTicketAndWorkflow(t, "TEST-ENV")
	wfiID := wfiIDForTicket(t, env, "TEST-ENV")

	seedObserverSession(t, env, "obs-proj-env", "project", env.project, wfiID)

	// Set
	resp := env.handler.Handle(obsReq("observer.project.env.set", map[string]interface{}{
		"session_id": "obs-proj-env",
		"name":       "MY_TEST_VAR",
		"value":      "hello",
	}))
	if resp.Error != nil {
		t.Fatalf("env.set error: %v", resp.Error)
	}

	// List to verify presence
	resp = env.handler.Handle(obsReq("observer.project.env.list", map[string]interface{}{
		"session_id": "obs-proj-env",
	}))
	if resp.Error != nil {
		t.Fatalf("env.list error: %v", resp.Error)
	}
	var vars []map[string]interface{}
	json.Unmarshal(resp.Result, &vars) //nolint:errcheck
	found := false
	for _, v := range vars {
		if v["name"] == "MY_TEST_VAR" {
			found = true
		}
	}
	if !found {
		t.Error("MY_TEST_VAR not found in env list after set")
	}

	// Unset
	resp = env.handler.Handle(obsReq("observer.project.env.unset", map[string]interface{}{
		"session_id": "obs-proj-env",
		"name":       "MY_TEST_VAR",
	}))
	if resp.Error != nil {
		t.Fatalf("env.unset error: %v", resp.Error)
	}
}

// TestObserverGlobalProjectCreate_HappyPath verifies global.project.create returns new project.
func TestObserverGlobalProjectCreate_HappyPath(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	enableObserver(t, env)
	seedObserverSession(t, env, "obs-global-pcreate", "global", env.project, "")

	resp := env.handler.Handle(obsReq("observer.global.project.create", map[string]interface{}{
		"session_id": "obs-global-pcreate",
		"project_id": "new-obs-project",
		"name":       "Observer Created Project",
		"root_path":  t.TempDir(),
	}))
	if resp.Error != nil {
		t.Fatalf("project.create error: %v", resp.Error)
	}
	var project map[string]interface{}
	if err := json.Unmarshal(resp.Result, &project); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if project["id"] != "new-obs-project" {
		t.Errorf("project id = %v, want 'new-obs-project'", project["id"])
	}
}
