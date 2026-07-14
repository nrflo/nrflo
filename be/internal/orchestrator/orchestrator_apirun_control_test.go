package orchestrator

import (
	"context"
	"strings"
	"testing"

	"be/internal/service"
	"be/internal/types"
)

// seedForeignInstance creates a second project owning one active workflow
// instance, and returns that instance's id.
func (e *testEnv) seedForeignInstance(t *testing.T, projectID, instanceID string) string {
	t.Helper()

	projectSvc := service.NewProjectService(e.pool, e.orch.clock)
	if _, err := projectSvc.Create(projectID, &types.ProjectCreateRequest{
		Name:     projectID,
		RootPath: t.TempDir(),
	}); err != nil {
		t.Fatalf("seed foreign project: %v", err)
	}
	if _, err := e.pool.Exec(`
		INSERT INTO workflows (id, project_id, description, created_at, updated_at, scope_type, groups)
		VALUES ('foreign-wf', ?, '', datetime('now'), datetime('now'), 'project', '[]')`, projectID); err != nil {
		t.Fatalf("seed foreign workflow: %v", err)
	}
	if _, err := e.pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, retry_count, created_at, updated_at)
		VALUES (?, ?, '', 'foreign-wf', 'active', 'project', 0, datetime('now'), datetime('now'))`,
		instanceID, projectID); err != nil {
		t.Fatalf("seed foreign instance: %v", err)
	}
	return instanceID
}

// TestAPIWorkflowControlRejectsForeignInstance pins the project guard: both
// ContinueWorkflow and FailWorkflow take a caller-supplied instance_id (an
// api-mode agent tool arg, and a console workflow_continue/workflow_fail arg)
// but resolve the project root and workflow def from the *caller's* projectID.
// Without the guard, a caller scoped to project A could fail project B's run —
// or, worse, resume B's instance inside A's repo.
func TestAPIWorkflowControlRejectsForeignInstance(t *testing.T) {
	env := newTestEnv(t)
	foreignInstance := env.seedForeignInstance(t, "other-project", "wfi-foreign")

	ctl := env.orch.APIWorkflowControl(env.pool)

	t.Run("continue", func(t *testing.T) {
		err := ctl.ContinueWorkflow(context.Background(), env.project, foreignInstance, "resume")
		if err == nil {
			t.Fatal("expected cross-project ContinueWorkflow to be rejected, got nil error")
		}
		if !strings.Contains(err.Error(), "does not belong to project") {
			t.Fatalf("expected project-guard error, got: %v", err)
		}
	})

	t.Run("fail", func(t *testing.T) {
		err := ctl.FailWorkflow(context.Background(), env.project, foreignInstance, "kill it")
		if err == nil {
			t.Fatal("expected cross-project FailWorkflow to be rejected, got nil error")
		}
		if !strings.Contains(err.Error(), "does not belong to project") {
			t.Fatalf("expected project-guard error, got: %v", err)
		}
		// The foreign run must be untouched.
		if wfi := env.getWorkflowInstance(t, foreignInstance); wfi.Status != "active" {
			t.Fatalf("foreign instance status changed to %q; guard did not stop the fail", wfi.Status)
		}
	})

	t.Run("same project passes the guard", func(t *testing.T) {
		ctlc, ok := ctl.(apiWorkflowControl)
		if !ok {
			t.Fatalf("APIWorkflowControl returned %T, want apiWorkflowControl", ctl)
		}
		wfi, err := ctlc.guardedInstance("other-project", foreignInstance)
		if err != nil {
			t.Fatalf("owner project rejected by guard: %v", err)
		}
		if wfi.ID != foreignInstance {
			t.Fatalf("guardedInstance returned %q, want %q", wfi.ID, foreignInstance)
		}
	})
}
