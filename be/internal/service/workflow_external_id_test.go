package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

func TestWorkflowService_Init_ExternalIDPassThrough(t *testing.T) {
	t.Parallel()

	t.Run("external_id_and_context_preserved", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, "proj-ext-ticket")
		seedTicketSvc(t, pool, "proj-ext-ticket", "tkt-ext")
		seedWorkflowDef(t, pool, "proj-ext-ticket", "feature", "ticket")

		req := &types.WorkflowInitRequest{
			Workflow:        "feature",
			ExternalID:      "gh-issue-99",
			ExternalContext: `{"repo":"acme/app","number":99}`,
		}

		wi, err := svc.Init("proj-ext-ticket", "tkt-ext", req)
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
		if wi.ExternalID != "gh-issue-99" {
			t.Errorf("wi.ExternalID = %q, want %q", wi.ExternalID, "gh-issue-99")
		}
		if wi.ExternalContext != `{"repo":"acme/app","number":99}` {
			t.Errorf("wi.ExternalContext = %q, want %q", wi.ExternalContext, `{"repo":"acme/app","number":99}`)
		}

		wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
		got, err := wfiRepo.Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "gh-issue-99" {
			t.Errorf("repo.Get ExternalID = %q, want %q", got.ExternalID, "gh-issue-99")
		}
		if got.ExternalContext != `{"repo":"acme/app","number":99}` {
			t.Errorf("repo.Get ExternalContext = %q, want %q", got.ExternalContext, `{"repo":"acme/app","number":99}`)
		}
	})

	t.Run("empty_external_id_stays_empty", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, "proj-noext-ticket")
		seedTicketSvc(t, pool, "proj-noext-ticket", "tkt-noext")
		seedWorkflowDef(t, pool, "proj-noext-ticket", "feature", "ticket")

		wi, err := svc.Init("proj-noext-ticket", "tkt-noext", &types.WorkflowInitRequest{
			Workflow: "feature",
		})
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
		if wi.ExternalID != "" {
			t.Errorf("wi.ExternalID = %q, want empty", wi.ExternalID)
		}
		if wi.ExternalContext != "" {
			t.Errorf("wi.ExternalContext = %q, want empty", wi.ExternalContext)
		}

		wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
		got, err := wfiRepo.Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "" {
			t.Errorf("repo.Get ExternalID = %q, want empty", got.ExternalID)
		}
		if got.ExternalContext != "" {
			t.Errorf("repo.Get ExternalContext = %q, want empty", got.ExternalContext)
		}
	})
}

func TestWorkflowService_InitProjectWorkflow_ExternalIDPassThrough(t *testing.T) {
	t.Parallel()

	t.Run("external_id_and_context_preserved", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, "proj-ext-proj")
		seedWorkflowDef(t, pool, "proj-ext-proj", "analysis", "project")

		req := &types.ProjectWorkflowRunRequest{
			Workflow:        "analysis",
			ExternalID:      "jira-FEAT-42",
			ExternalContext: `{"project":"FEAT","key":"FEAT-42"}`,
		}

		wi, err := svc.InitProjectWorkflow("proj-ext-proj", req)
		if err != nil {
			t.Fatalf("InitProjectWorkflow: %v", err)
		}
		if wi.ExternalID != "jira-FEAT-42" {
			t.Errorf("wi.ExternalID = %q, want %q", wi.ExternalID, "jira-FEAT-42")
		}
		if wi.ExternalContext != `{"project":"FEAT","key":"FEAT-42"}` {
			t.Errorf("wi.ExternalContext = %q, want %q", wi.ExternalContext, `{"project":"FEAT","key":"FEAT-42"}`)
		}

		wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
		got, err := wfiRepo.Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "jira-FEAT-42" {
			t.Errorf("repo.Get ExternalID = %q, want %q", got.ExternalID, "jira-FEAT-42")
		}
		if got.ExternalContext != `{"project":"FEAT","key":"FEAT-42"}` {
			t.Errorf("repo.Get ExternalContext = %q, want %q", got.ExternalContext, `{"project":"FEAT","key":"FEAT-42"}`)
		}
	})

	t.Run("empty_external_id_stays_empty", func(t *testing.T) {
		t.Parallel()
		svc, pool := setupWorkflowSeedTestEnv(t)

		seedSvcProject(t, pool, "proj-noext-proj")
		seedWorkflowDef(t, pool, "proj-noext-proj", "analysis", "project")

		wi, err := svc.InitProjectWorkflow("proj-noext-proj", &types.ProjectWorkflowRunRequest{
			Workflow: "analysis",
		})
		if err != nil {
			t.Fatalf("InitProjectWorkflow: %v", err)
		}
		if wi.ExternalID != "" {
			t.Errorf("wi.ExternalID = %q, want empty", wi.ExternalID)
		}
		if wi.ExternalContext != "" {
			t.Errorf("wi.ExternalContext = %q, want empty", wi.ExternalContext)
		}

		wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
		got, err := wfiRepo.Get(wi.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ExternalID != "" {
			t.Errorf("repo.Get ExternalID = %q, want empty", got.ExternalID)
		}
		if got.ExternalContext != "" {
			t.Errorf("repo.Get ExternalContext = %q, want empty", got.ExternalContext)
		}
	})
}
