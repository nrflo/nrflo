package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/spawner"
)

// ── buildPlanPrompt ───────────────────────────────────────────────────────────

func TestBuildPlanPrompt_ContainsTicketID(t *testing.T) {
	req := RunRequest{TicketID: "TKT-999"}
	prompt := buildPlanPrompt(req)
	if !strings.Contains(prompt, "TKT-999") {
		t.Errorf("buildPlanPrompt() missing ticket ID in %q", prompt)
	}
}

func TestBuildPlanPrompt_ContainsInstructions(t *testing.T) {
	req := RunRequest{Instructions: "Implement the foo feature carefully"}
	prompt := buildPlanPrompt(req)
	if !strings.Contains(prompt, "Implement the foo feature carefully") {
		t.Errorf("buildPlanPrompt() missing instructions in %q", prompt)
	}
}

func TestBuildPlanPrompt_NoTicketID_NoTicketLine(t *testing.T) {
	req := RunRequest{}
	prompt := buildPlanPrompt(req)
	if strings.Contains(prompt, "Ticket:") {
		t.Errorf("buildPlanPrompt() should not include 'Ticket:' when TicketID is empty, got %q", prompt)
	}
}

func TestBuildPlanPrompt_NoInstructions_NoInstructionsLine(t *testing.T) {
	req := RunRequest{TicketID: "TKT-X"}
	prompt := buildPlanPrompt(req)
	if strings.Contains(prompt, "Instructions:") {
		t.Errorf("buildPlanPrompt() should not include 'Instructions:' when empty, got %q", prompt)
	}
}

// ── waitForInteractivePreStep ─────────────────────────────────────────────────

func TestWaitForInteractivePreStep_CompletedNormally(t *testing.T) {
	sp := spawner.New(spawner.Config{Clock: clock.Real()})
	sessionID := "wait-normal-session"
	waitCh := sp.RegisterInteractiveWait(sessionID)

	pre := &interactivePreStep{
		sessionID: sessionID,
		waitCh:    waitCh,
		spawner:   sp,
	}

	go func() {
		sp.CompleteInteractive(sessionID)
	}()

	ctx := context.Background()
	result := waitForInteractivePreStep(ctx, pre)
	if !result {
		t.Error("waitForInteractivePreStep() = false, want true when session completes normally")
	}
}

func TestWaitForInteractivePreStep_CancelledByContext(t *testing.T) {
	sp := spawner.New(spawner.Config{Clock: clock.Real()})
	sessionID := "wait-cancel-session"
	waitCh := sp.RegisterInteractiveWait(sessionID)

	pre := &interactivePreStep{
		sessionID: sessionID,
		waitCh:    waitCh,
		spawner:   sp,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := waitForInteractivePreStep(ctx, pre)
	if result {
		t.Error("waitForInteractivePreStep() = true, want false when context is cancelled")
	}
}

// ── handlePlanModePostStep ────────────────────────────────────────────────────

// setupPlanModeHome creates a fake HOME directory with a plan file and session log.
// Returns the plan content that should be stored.
func setupPlanModeHome(t *testing.T, sessionID, projectRoot, planContent string) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	plansDir := filepath.Join(homeDir, ".claude", "plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	planFile := "test-plan.md"
	if err := os.WriteFile(filepath.Join(plansDir, planFile), []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	encodedRoot := "-" + strings.ReplaceAll(strings.TrimPrefix(projectRoot, "/"), "/", "-")
	logDir := filepath.Join(homeDir, ".claude", "projects", encodedRoot)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("failed to create log dir: %v", err)
	}

	logContent := fmt.Sprintf(`{"msg":"plan created: %s"}`, planFile)
	if err := os.WriteFile(filepath.Join(logDir, sessionID+".jsonl"), []byte(logContent), 0644); err != nil {
		t.Fatalf("failed to write session log: %v", err)
	}
}

func TestHandlePlanModePostStep_StoresPlanContent(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PM-1", "Plan mode post step test")
	wfiID := env.initWorkflow(t, "TKT-PM-1")

	planContent := "# My Plan\n\nStep 1: Analyze\nStep 2: Implement"
	sessionID := "plan-post-session"
	projectRoot := "/test/project/root"
	setupPlanModeHome(t, sessionID, projectRoot, planContent)

	// Insert planner session so UpdateStatusToInteractiveCompleted can find it.
	now := clock.Real().Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, result_reason, pid, context_left, ancestor_session_id,
			spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-project', '', ?, 'planning', 'planner', 'user_interactive',
			NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, wfiID, now, now, now); err != nil {
		t.Fatalf("insert planner session: %v", err)
	}

	adapter := &spawner.ClaudeAdapter{}
	planCapture := spawner.PlanCaptureOptions{SessionID: sessionID, WorkDir: projectRoot}
	err := handlePlanModePostStep(adapter, planCapture, env.pool, wfiID, clock.Real())
	if err != nil {
		t.Fatalf("handlePlanModePostStep() error: %v", err)
	}

	// Verify user_instructions stored in findings
	findings := getWFIFindings(t, env, wfiID)
	gotInstructions, ok := findings["user_instructions"]
	if !ok {
		t.Fatal("user_instructions not found in workflow instance findings")
	}
	if gotInstructions != planContent {
		t.Errorf("user_instructions = %q, want %q", gotInstructions, planContent)
	}
}

func TestHandlePlanModePostStep_NoPlanFileReturnsError(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-PM-2", "Plan mode no file test")
	wfiID := env.initWorkflow(t, "TKT-PM-2")

	// Set HOME to a temp dir with no plan file
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	os.MkdirAll(filepath.Join(homeDir, ".claude", "plans"), 0755)

	adapter := &spawner.ClaudeAdapter{}
	planCapture := spawner.PlanCaptureOptions{SessionID: "no-plan-session", WorkDir: "/some/root"}
	err := handlePlanModePostStep(adapter, planCapture, env.pool, wfiID, clock.Real())
	if err == nil {
		t.Fatal("handlePlanModePostStep() should return error when no plan file found")
	}
	if !strings.Contains(err.Error(), "no plan file found") {
		t.Errorf("error = %q, want to contain 'no plan file found'", err.Error())
	}
}
