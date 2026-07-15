package service

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// seedGlobalWorkflowDef inserts a global workflow definition under the reserved
// global project (is_global=1), mirroring how EnsureGlobalDynamicWorkflow seeds.
func seedGlobalWorkflowDef(t *testing.T, pool *db.Pool, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, is_global, created_at, updated_at)
		 VALUES (?, ?, '', ?, '[]', 1, 1, ?, ?)`,
		workflowID, GlobalProjectID, scopeType, now, now,
	); err != nil {
		t.Fatalf("seedGlobalWorkflowDef(%q, scope=%q): %v", workflowID, scopeType, err)
	}
}

// Regression for the FOREIGN KEY (787) failure on running a global workflow
// definition (e.g. 'dynamic') under a real project: the instance's workflows FK
// must ride on def_project_id (the definition's owning project), not on the
// executing project_id.
func TestWorkflowService_Init_GlobalDefUnderRealProject(t *testing.T) {
	t.Parallel()

	t.Run("project_scope", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, GlobalProjectID)
		seedSvcProject(t, pool, "proj-global-run")
		seedGlobalWorkflowDef(t, pool, "gwf-project", "project")

		wi, err := svc.InitProjectWorkflow("proj-global-run", &types.ProjectWorkflowRunRequest{
			Workflow: "gwf-project",
		})
		if err != nil {
			t.Fatalf("InitProjectWorkflow(global def): %v", err)
		}
		if wi.ProjectID != "proj-global-run" {
			t.Errorf("wi.ProjectID = %q, want %q", wi.ProjectID, "proj-global-run")
		}

		got, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.DefProjectID != GlobalProjectID {
			t.Errorf("DefProjectID = %q, want %q", got.DefProjectID, GlobalProjectID)
		}
	})

	t.Run("ticket_scope", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, GlobalProjectID)
		seedSvcProject(t, pool, "proj-global-tkt")
		seedTicketSvc(t, pool, "proj-global-tkt", "tkt-g1")
		seedGlobalWorkflowDef(t, pool, "gwf-ticket", "ticket")

		wi, err := svc.Init("proj-global-tkt", "tkt-g1", &types.WorkflowInitRequest{
			Workflow: "gwf-ticket",
		})
		if err != nil {
			t.Fatalf("Init(global def): %v", err)
		}

		got, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.DefProjectID != GlobalProjectID {
			t.Errorf("DefProjectID = %q, want %q", got.DefProjectID, GlobalProjectID)
		}
	})

	t.Run("local_def_stamps_own_project", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, "proj-local-def")
		seedWorkflowDef(t, pool, "proj-local-def", "analysis", "project")

		wi, err := svc.InitProjectWorkflow("proj-local-def", &types.ProjectWorkflowRunRequest{
			Workflow: "analysis",
		})
		if err != nil {
			t.Fatalf("InitProjectWorkflow(local def): %v", err)
		}

		got, err := repo.NewWorkflowInstanceRepo(pool, clock.Real()).Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.DefProjectID != "proj-local-def" {
			t.Errorf("DefProjectID = %q, want %q", got.DefProjectID, "proj-local-def")
		}
	})
}
