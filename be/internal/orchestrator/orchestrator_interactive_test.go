package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
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

	err := handlePlanModePostStep(sessionID, projectRoot, env.pool, wfiID, clock.Real())
	if err != nil {
		t.Fatalf("handlePlanModePostStep() error: %v", err)
	}

	// Verify user_instructions stored in findings
	wi := env.getWorkflowInstance(t, wfiID)
	findings := wi.GetFindings()
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

	err := handlePlanModePostStep("no-plan-session", "/some/root", env.pool, wfiID, clock.Real())
	if err == nil {
		t.Fatal("handlePlanModePostStep() should return error when no plan file found")
	}
	if !strings.Contains(err.Error(), "no plan file found") {
		t.Errorf("error = %q, want to contain 'no plan file found'", err.Error())
	}
}

// ── setupInteractivePreStep (plan mode) ───────────────────────────────────────

func TestSetupInteractivePreStep_PlanMode_CreatesSession(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-1", "Interactive pre-step plan test")
	wfiID := env.initWorkflow(t, "TKT-SIP-1")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{
			{Agent: "analyzer", Layer: 0},
			{Agent: "builder", Layer: 1},
		},
	}
	svcAgents := map[string]service.SpawnerAgentConfig{}
	workflows := map[string]spawner.WorkflowDef{}
	agents := map[string]spawner.AgentConfig{}

	var registeredSessionID, registeredCmd string
	var registeredArgs []string
	env.orch.OnRegisterPtyCommand = func(sid, cmd string, args []string) {
		registeredSessionID = sid
		registeredCmd = cmd
		registeredArgs = args
	}

	req := RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-SIP-1",
		WorkflowName: "test",
		PlanMode:     true,
	}

	pre, err := env.orch.setupInteractivePreStep(req, wi, svcWf, svcAgents, workflows, agents, t.TempDir(), nil, "")
	if err != nil {
		t.Fatalf("setupInteractivePreStep() error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })

	// Validate returned pre-step
	if pre.sessionID == "" {
		t.Error("expected non-empty sessionID")
	}
	if pre.waitCh == nil {
		t.Error("expected non-nil waitCh")
	}
	if pre.spawner == nil {
		t.Error("expected non-nil spawner")
	}

	// Validate OnRegisterPtyCommand invocation
	if registeredSessionID != pre.sessionID {
		t.Errorf("registered session ID = %q, want %q", registeredSessionID, pre.sessionID)
	}
	if registeredCmd != "claude" {
		t.Errorf("registered cmd = %q, want 'claude'", registeredCmd)
	}
	argsStr := strings.Join(registeredArgs, " ")
	if !strings.Contains(argsStr, "--dangerously-skip-permissions") {
		t.Errorf("args missing --dangerously-skip-permissions: %v", registeredArgs)
	}
	if !strings.Contains(argsStr, pre.sessionID) {
		t.Errorf("args missing session ID: %v", registeredArgs)
	}

	// Wait a bit for the DB write to be visible (pool commits synchronously, but let's be safe)
	var status, agentType string
	var queryErr error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		queryErr = env.pool.QueryRow(
			`SELECT status, agent_type FROM agent_sessions WHERE id = ?`,
			pre.sessionID,
		).Scan(&status, &agentType)
		if queryErr == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if queryErr != nil {
		t.Fatalf("failed to query session: %v", queryErr)
	}
	if status != "user_interactive" {
		t.Errorf("session status = %q, want 'user_interactive'", status)
	}
	if agentType != "planner" {
		t.Errorf("session agent_type = %q, want 'planner'", agentType)
	}
}

func TestSetupInteractivePreStep_PlanMode_NoRegisterPtyCommand_OK(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-NR", "No register callback test")
	wfiID := env.initWorkflow(t, "TKT-SIP-NR")
	wi := env.getWorkflowInstance(t, wfiID)

	svcWf := service.SpawnerWorkflowDef{
		Phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}},
	}

	// OnRegisterPtyCommand is nil — should not panic
	pre, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-NR", WorkflowName: "test", PlanMode: true},
		wi,
		svcWf,
		map[string]service.SpawnerAgentConfig{},
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("setupInteractivePreStep() with nil callback error: %v", err)
	}
	t.Cleanup(func() { pre.spawner.CompleteInteractive(pre.sessionID) })
}

func TestSetupInteractivePreStep_EmptyWorkflowReturnsError(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-SIP-EW", "Empty workflow test")
	wfiID := env.initWorkflow(t, "TKT-SIP-EW")
	wi := env.getWorkflowInstance(t, wfiID)

	// Interactive mode with empty phases should fail
	svcWf := service.SpawnerWorkflowDef{Phases: []service.SpawnerPhaseDef{}}

	_, err := env.orch.setupInteractivePreStep(
		RunRequest{ProjectID: env.project, TicketID: "TKT-SIP-EW", WorkflowName: "test", Interactive: true},
		wi,
		svcWf,
		map[string]service.SpawnerAgentConfig{},
		map[string]spawner.WorkflowDef{},
		map[string]spawner.AgentConfig{},
		t.TempDir(),
		nil,
		"",
	)
	if err == nil {
		t.Fatal("expected error for empty workflow phases in interactive mode")
	}
}

// ── Start() with PlanMode ─────────────────────────────────────────────────────

// createAgentDefForWorkflow inserts an agent definition for agentType in the "test" workflow.
func createAgentDefForWorkflow(t *testing.T, env *testEnv, agentType, prompt string) {
	t.Helper()
	agentSvc := service.NewAgentDefinitionService(env.pool, clock.Real(), service.NewCLIModelService(env.pool, clock.Real()))
	_, err := agentSvc.CreateAgentDef(env.project, "test", &types.AgentDefCreateRequest{
		ID:     agentType,
		Prompt: prompt,
		Model:  "sonnet",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition for %s: %v", agentType, err)
	}
}

func TestStart_PlanMode_ReturnsSessionIDAndPlanningStatus(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-START-PM", "Start plan mode test")

	var registeredSessionID string
	env.orch.OnRegisterPtyCommand = func(sid, cmd string, args []string) {
		registeredSessionID = sid
	}

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-START-PM",
		WorkflowName: "test",
		PlanMode:     true,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if result.SessionID == "" {
		t.Error("Start() with PlanMode=true returned empty SessionID")
	}
	if result.Status != "planning" {
		t.Errorf("Start() status = %q, want 'planning'", result.Status)
	}
	if result.InstanceID == "" {
		t.Error("Start() returned empty InstanceID")
	}
	if registeredSessionID != result.SessionID {
		t.Errorf("OnRegisterPtyCommand session ID = %q, want %q", registeredSessionID, result.SessionID)
	}

	// Verify DB session has user_interactive status and agent_type=planner
	var status, agentType string
	if err := env.pool.QueryRow(
		`SELECT status, agent_type FROM agent_sessions WHERE id = ?`,
		result.SessionID,
	).Scan(&status, &agentType); err != nil {
		t.Fatalf("failed to query session: %v", err)
	}
	if status != "user_interactive" {
		t.Errorf("session status = %q, want 'user_interactive'", status)
	}
	if agentType != "planner" {
		t.Errorf("session agent_type = %q, want 'planner'", agentType)
	}
}

func TestStart_Interactive_ReturnsSessionIDAndInteractiveStatus(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-START-INT", "Start interactive mode test")

	// Create agent definition for L0 agent "analyzer"
	createAgentDefForWorkflow(t, env, "analyzer", "You are an analyzer. Ticket: ${TICKET_ID}")

	var registeredSessionID string
	env.orch.OnRegisterPtyCommand = func(sid, cmd string, args []string) {
		registeredSessionID = sid
	}

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-START-INT",
		WorkflowName: "test",
		Interactive:  true,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if result.SessionID == "" {
		t.Error("Start() with Interactive=true returned empty SessionID")
	}
	if result.Status != "interactive" {
		t.Errorf("Start() status = %q, want 'interactive'", result.Status)
	}
	if registeredSessionID != result.SessionID {
		t.Errorf("OnRegisterPtyCommand session ID = %q, want %q", registeredSessionID, result.SessionID)
	}

	// Verify DB session exists with user_interactive status
	var status string
	if err := env.pool.QueryRow(
		`SELECT status FROM agent_sessions WHERE id = ?`,
		result.SessionID,
	).Scan(&status); err != nil {
		t.Fatalf("failed to query session: %v", err)
	}
	if status != "user_interactive" {
		t.Errorf("session status = %q, want 'user_interactive'", status)
	}
}

func TestStart_NormalMode_NoSessionID(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-START-NORM", "Normal start test")
	createAgentDefForWorkflow(t, env, "analyzer", "Analyze: ${TICKET_ID}")

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-START-NORM",
		WorkflowName: "test",
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if result.SessionID != "" {
		t.Errorf("Start() without interactive/plan should return empty SessionID, got %q", result.SessionID)
	}
	if result.Status != "started" {
		t.Errorf("Start() status = %q, want 'started'", result.Status)
	}
}

// ── RunResult JSON output ─────────────────────────────────────────────────────

func TestRunResult_SessionIDOmittedWhenEmpty(t *testing.T) {
	result := RunResult{InstanceID: "inst-1", Status: "started"}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if strings.Contains(string(data), "session_id") {
		t.Errorf("JSON should omit session_id when empty, got %s", string(data))
	}
}

func TestRunResult_SessionIDIncludedWhenSet(t *testing.T) {
	result := RunResult{InstanceID: "inst-1", SessionID: "sess-abc", Status: "planning"}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if m["session_id"] != "sess-abc" {
		t.Errorf("session_id = %q, want 'sess-abc'", m["session_id"])
	}
}

// ── Plan content stored as user_instructions after CompleteInteractive ─────────

// TestRunLoop_PlanMode_StoresUserInstructions verifies that when runLoop unblocks
// after an interactive pre-step in plan mode, it reads the plan file and stores
// its content as user_instructions in the workflow instance findings.
func TestRunLoop_PlanMode_StoresUserInstructions(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-LOOP-PM", "runLoop plan mode test")

	// Get the project's actual root_path from DB (used by runLoop as projectRoot)
	var projectRoot string
	if err := env.pool.QueryRow(`SELECT root_path FROM projects WHERE id = ?`, env.project).Scan(&projectRoot); err != nil {
		t.Fatalf("failed to get project root_path: %v", err)
	}

	planContent := "# Plan\n\nDo step 1 then step 2"

	// Start plan mode — runLoop blocks on waitCh
	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "TKT-LOOP-PM",
		WorkflowName: "test",
		PlanMode:     true,
	})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Set up HOME with plan file and session log (after we know the actual session ID)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	plansDir := filepath.Join(homeDir, ".claude", "plans")
	os.MkdirAll(plansDir, 0755)
	planFilename := "impl-plan.md"
	os.WriteFile(filepath.Join(plansDir, planFilename), []byte(planContent), 0644)

	encodedRoot := "-" + strings.ReplaceAll(strings.TrimPrefix(projectRoot, "/"), "/", "-")
	logDir := filepath.Join(homeDir, ".claude", "projects", encodedRoot)
	os.MkdirAll(logDir, 0755)
	logContent := fmt.Sprintf(`{"msg":"created %s"}`, planFilename)
	os.WriteFile(filepath.Join(logDir, result.SessionID+".jsonl"), []byte(logContent), 0644)

	// Complete the interactive session — runLoop unblocks and reads plan file
	if err := env.orch.CompleteInteractive(result.SessionID); err != nil {
		t.Fatalf("CompleteInteractive() error: %v", err)
	}

	// Poll until runLoop processes the plan (may fail on agent spawning, but plan should be stored first)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
		var wfiID string
		env.pool.QueryRow(
			`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)`,
			env.project, "TKT-LOOP-PM",
		).Scan(&wfiID)
		if wfiID != "" {
			wi, err := wfiRepo.Get(wfiID)
			if err == nil {
				findings := wi.GetFindings()
				if instructions, ok := findings["user_instructions"]; ok && instructions == planContent {
					return // success
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("user_instructions not stored in workflow instance findings after plan mode completion")
}

// ── Agent def service helper ──────────────────────────────────────────────────

// Verify NewAgentDefinitionService is importable and usable.
func TestNewAgentDefinitionService_IsAvailable(t *testing.T) {
	env := newTestEnv(t)
	svc := service.NewAgentDefinitionService(env.pool, clock.Real(), service.NewCLIModelService(env.pool, clock.Real()))
	if svc == nil {
		t.Fatal("NewAgentDefinitionService returned nil")
	}
}
