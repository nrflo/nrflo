package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/repo"
)

// ── HasRunningTicketWorkflows unit tests ─────────────────────────────────────

// TestHasRunningTicketWorkflows_Empty returns false when no runs are active.
func TestHasRunningTicketWorkflows_Empty(t *testing.T) {
	env := newTestEnv(t)
	if env.orch.HasRunningTicketWorkflows(env.project) {
		t.Error("HasRunningTicketWorkflows() = true, want false (no active runs)")
	}
}

// TestHasRunningTicketWorkflows_TicketScopedRun returns true when a ticket-scoped
// instance is in the runs map.
func TestHasRunningTicketWorkflows_TicketScopedRun(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "HRT-1", "ticket for HasRunning")
	wfiID := env.initWorkflow(t, "HRT-1")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	if !env.orch.HasRunningTicketWorkflows(env.project) {
		t.Error("HasRunningTicketWorkflows() = false, want true (ticket-scoped run active)")
	}
}

// TestHasRunningTicketWorkflows_ProjectScopedRun returns false when only a
// project-scoped instance (TicketID="") is in the runs map.
func TestHasRunningTicketWorkflows_ProjectScopedRun(t *testing.T) {
	env := newTestEnv(t)

	// Insert a project-scoped instance directly (ticket_id = '').
	wfiID := uuid.New().String()
	_, err := env.pool.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		wfiID, env.project, "", "test", "active", "project")
	if err != nil {
		t.Fatalf("failed to insert project-scoped instance: %v", err)
	}

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	if env.orch.HasRunningTicketWorkflows(env.project) {
		t.Error("HasRunningTicketWorkflows() = true, want false (only project-scoped run active)")
	}
}

// TestHasRunningTicketWorkflows_DifferentProject returns false when the active
// ticket run belongs to a different project.
func TestHasRunningTicketWorkflows_DifferentProject(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "HRT-2", "ticket for cross-project check")
	wfiID := env.initWorkflow(t, "HRT-2")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	// Query against a different project — must not match.
	if env.orch.HasRunningTicketWorkflows("other-project-id") {
		t.Error("HasRunningTicketWorkflows(otherProject) = true, want false")
	}
}

// ── Concurrent ticket guard in Start() ───────────────────────────────────────

// assertNoConcurrentError fails the test if err contains the concurrent-guard message.
func assertNoConcurrentError(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), "concurrent ticket workflows") {
		t.Errorf("unexpected concurrent-guard error: %v", err)
	}
}

// TestConcurrentTicketGuard_BlocksDifferentTicket verifies that starting a ticket
// workflow when another ticket's workflow is running (same project, worktrees off)
// returns the concurrent-guard error.
func TestConcurrentTicketGuard_BlocksDifferentTicket(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CG-A", "first ticket")
	env.createTicket(t, "CG-B", "second ticket")

	wfiID := env.initWorkflow(t, "CG-A")

	// Simulate CG-A's workflow already running.
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "CG-B",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	if err == nil {
		t.Fatal("Start() returned nil error, want concurrent-guard error")
	}
	if !strings.Contains(err.Error(), "concurrent ticket workflows") {
		t.Errorf("error = %q, want it to contain 'concurrent ticket workflows'", err.Error())
	}
	if !strings.Contains(err.Error(), "use force to override") {
		t.Errorf("error = %q, want it to contain 'use force to override'", err.Error())
	}
}

// TestConcurrentTicketGuard_ForceBypassesGuard verifies that Force=true allows
// a second ticket workflow to start even when another is running.
func TestConcurrentTicketGuard_ForceBypassesGuard(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CGF-A", "first ticket force")
	env.createTicket(t, "CGF-B", "second ticket force")

	wfiID := env.initWorkflow(t, "CGF-A")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "CGF-B",
		WorkflowName: "test",
		ScopeType:    "ticket",
		Force:        true,
	})
	// Force bypasses the guard — the guard error must NOT appear.
	assertNoConcurrentError(t, err)
}

// TestConcurrentTicketGuard_WorktreesEnabled_NotBlocked verifies that when
// UseGitWorktrees=true the guard never fires, even with a ticket workflow running.
func TestConcurrentTicketGuard_WorktreesEnabled_NotBlocked(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CGW-A", "first ticket wt")
	env.createTicket(t, "CGW-B", "second ticket wt")

	wfiID := env.initWorkflow(t, "CGW-A")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	// Enable worktrees for the test project.
	projectRepo := repo.NewProjectRepo(env.pool, clock.Real())
	trueVal := true
	if err := projectRepo.Update(env.project, &repo.ProjectUpdateFields{UseGitWorktrees: &trueVal}); err != nil {
		t.Fatalf("failed to enable worktrees: %v", err)
	}

	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "CGW-B",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	assertNoConcurrentError(t, err)
}

// TestConcurrentTicketGuard_ProjectScopedRequest_NotBlocked verifies that
// project-scoped run requests are never subject to the concurrent ticket guard.
func TestConcurrentTicketGuard_ProjectScopedRequest_NotBlocked(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CGP-A", "ticket while project runs")
	wfiID := env.initWorkflow(t, "CGP-A")

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	// Promote "test" workflow to project scope so InitProjectWorkflow works.
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type='project' WHERE LOWER(id)=LOWER(?)`, "test")
	if err != nil {
		t.Fatalf("failed to set workflow scope: %v", err)
	}

	_, err = env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "test",
		ScopeType:    "project",
	})
	assertNoConcurrentError(t, err)
}

// TestConcurrentTicketGuard_NoRunning_Passes verifies that the guard is a no-op
// when no ticket workflows are running.
func TestConcurrentTicketGuard_NoRunning_Passes(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CGN-1", "no running workflows")

	// No entries in o.runs — guard should not trigger.
	_, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "CGN-1",
		WorkflowName: "test",
		ScopeType:    "ticket",
	})
	assertNoConcurrentError(t, err)
}
