package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	ptyPkg "be/internal/pty"
	"be/internal/service"
)

// ── Start() with PlanMode ─────────────────────────────────────────────────────

func TestStart_PlanMode_ReturnsSessionIDAndPlanningStatus(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-START-PM", "Start plan mode test")

	var registeredSessionID string
	env.orch.OnRegisterPtyCommand = func(sid string, l ptyPkg.Launch) {
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

	var registeredSessionID string
	env.orch.OnRegisterPtyCommand = func(sid string, l ptyPkg.Launch) {
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
		var wfiID string
		env.pool.QueryRow(
			`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?)`,
			env.project, "TKT-LOOP-PM",
		).Scan(&wfiID)
		if wfiID != "" {
			findings := getWFIFindings(t, env, wfiID)
			if instructions, ok := findings["user_instructions"]; ok && instructions == planContent {
				return // success
			}
		}
		runtime.Gosched()
	}
	t.Error("user_instructions not stored in workflow instance findings after plan mode completion")
}

// ── Agent def service helper ──────────────────────────────────────────────────

// Verify NewAgentDefinitionService is importable and usable.
func TestNewAgentDefinitionService_IsAvailable(t *testing.T) {
	env := newTestEnv(t)
	svc := service.NewAgentDefinitionService(env.pool, clock.Real(), service.NewModelService(env.pool, clock.Real()), nil)
	if svc == nil {
		t.Fatal("NewAgentDefinitionService returned nil")
	}
}
