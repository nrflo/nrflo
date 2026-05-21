package orchestrator

import (
	"context"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// TestPauseAfterLayer_E2E_SkippedLayers verifies the full pause→continue→complete flow
// without real CLI execution: both layers are skipped via skip_tags, pause_after is set
// on layer 0, so Start→L0-skipped→paused(waiting); then ContinueWorkflow→L1-skipped→completed.
func TestPauseAfterLayer_E2E_SkippedLayers(t *testing.T) {
	env := newTestEnv(t)

	// Tag both agent definitions so skip_tags can match.
	_, err := env.pool.Exec(
		`UPDATE agent_definitions SET tag = 'e2e-skip' WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = 'test'`,
		env.project)
	if err != nil {
		t.Fatalf("tag agent_definitions: %v", err)
	}

	// Set pause_after=true for layer 0 (the first layer, by layer NUMBER).
	layerPolicySvc := service.NewWorkflowLayerPolicyService(env.pool, clock.Real())
	if err := layerPolicySvc.SetLayerPauseAfter(env.project, "test", 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter: %v", err)
	}

	env.createTicket(t, "E2E-P1", "pause e2e")
	ch := env.subscribeWSClient(t, "ws-e2e-p1", "E2E-P1")

	// Start the orchestration — launches runLoop in a goroutine.
	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "E2E-P1",
		WorkflowName: "test",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	wfiID := result.InstanceID

	// Set skip_tags on the newly created WFI.
	// The goroutine does several DB reads before shouldSkipLayer, so this wins the race.
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	if err := wfiRepo.UpdateSkipTags(wfiID, `["e2e-skip"]`); err != nil {
		t.Fatalf("UpdateSkipTags: %v", err)
	}

	// Wait for L0 to be skipped and the pause to occur.
	expectEvent(t, ch, ws.EventWorkflowPaused, 5*time.Second)

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaiting {
		t.Fatalf("status after pause = %v, want waiting", wi.Status)
	}

	// Verify _pause finding has correct resume_layer.
	findings := getWFIFindings(t, env, wfiID)
	pauseM, ok := findings["_pause"].(map[string]interface{})
	if !ok {
		t.Fatalf("_pause finding absent after EventWorkflowPaused")
	}
	if int(pauseM["resume_layer"].(float64)) != 1 {
		t.Errorf("resume_layer = %v, want 1", pauseM["resume_layer"])
	}

	// Resume: ContinueWorkflow re-enters runLoop at layer index for layer 1.
	err = env.orch.ContinueWorkflow(context.Background(), env.project, "E2E-P1", "test", wfiID, "")
	if err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}

	expectEvent(t, ch, ws.EventWorkflowResumed, 2*time.Second)

	// L1 is also skipped (skip_tags still set) → no pause on last layer → markCompleted.
	expectEvent(t, ch, ws.EventOrchestrationCompleted, 5*time.Second)

	wi = env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceCompleted {
		t.Errorf("final status = %v, want completed", wi.Status)
	}
}

// TestPauseAfterLayer_WorktreeSurvives guards the load-bearing worktreeHandled=true
// requirement on the pause path: the on-disk worktree must survive a pause (status
// waiting) and only be merged/cleaned at final completion.
func TestPauseAfterLayer_WorktreeSurvives(t *testing.T) {
	env := newTestEnv(t)

	// Point the project at a real git repo with worktrees enabled.
	gitRepo := setupGitRepo(t)
	t.Cleanup(func() { os.RemoveAll(gitRepo) })
	trueVal := true
	mainBranch := "main"
	projectRepo := repo.NewProjectRepo(env.pool, clock.Real())
	if err := projectRepo.Update(env.project, &repo.ProjectUpdateFields{
		RootPath:        &gitRepo,
		DefaultBranch:   &mainBranch,
		UseGitWorktrees: &trueVal,
	}); err != nil {
		t.Fatalf("enable worktrees: %v", err)
	}

	// Tag both agents so skip_tags matches, and pause after layer 0.
	if _, err := env.pool.Exec(
		`UPDATE agent_definitions SET tag = 'wt-skip' WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = 'test'`,
		env.project); err != nil {
		t.Fatalf("tag agent_definitions: %v", err)
	}
	layerPolicySvc := service.NewWorkflowLayerPolicyService(env.pool, clock.Real())
	if err := layerPolicySvc.SetLayerPauseAfter(env.project, "test", 0, true); err != nil {
		t.Fatalf("SetLayerPauseAfter: %v", err)
	}

	env.createTicket(t, "WT-P1", "worktree pause")
	ch := env.subscribeWSClient(t, "ws-wt-p1", "WT-P1")

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "WT-P1",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	wfiID := result.InstanceID

	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	if err := wfiRepo.UpdateSkipTags(wfiID, `["wt-skip"]`); err != nil {
		t.Fatalf("UpdateSkipTags: %v", err)
	}

	expectEvent(t, ch, ws.EventWorkflowPaused, 5*time.Second)

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaiting {
		t.Fatalf("status after pause = %v, want waiting", wi.Status)
	}

	// The worktree must still exist on disk while waiting.
	if !wi.WorktreePath.Valid || wi.WorktreePath.String == "" {
		t.Fatalf("WorktreePath not persisted on paused instance")
	}
	worktreePath := wi.WorktreePath.String
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		t.Fatalf("worktree must survive pause, but stat failed: %v", statErr)
	}

	// Resume → L1 skipped → completed → worktree merged & removed.
	if err := env.orch.ContinueWorkflow(context.Background(), env.project, "WT-P1", "test", wfiID, ""); err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}
	expectEvent(t, ch, ws.EventOrchestrationCompleted, 5*time.Second)

	if wi = env.getWorkflowInstance(t, wfiID); wi.Status != model.WorkflowInstanceCompleted {
		t.Errorf("final status = %v, want completed", wi.Status)
	}
	if _, statErr := os.Stat(worktreePath); !os.IsNotExist(statErr) {
		t.Errorf("worktree should be removed after completion, stat err = %v", statErr)
	}
}
