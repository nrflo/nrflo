package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// TestGetStatus_DerivedFromAgentSessions verifies that the workflow status
// response derives phases from agent_sessions rather than the phases JSON column.
func TestGetStatus_DerivedFromAgentSessions(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DERIVE-1", "Derive test ticket")
	env.InitWorkflow(t, "DERIVE-1")
	wfiID := env.GetWorkflowInstanceID(t, "DERIVE-1", "test")

	// Insert an agent_session with completed status for analyzer
	env.InsertAgentSession(t, "sess-analyzer", "DERIVE-1", wfiID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "sess-analyzer", "pass")

	// getWorkflowStatus normalizes Go types via JSON round-trip so we can type-assert safely
	status, err := getWorkflowStatus(t, env, "DERIVE-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	builderPhase, _ := phases["builder"].(map[string]interface{})

	if analyzerPhase["status"] != "completed" {
		t.Errorf("analyzer.status = %v, want completed", analyzerPhase["status"])
	}
	if analyzerPhase["result"] != "pass" {
		t.Errorf("analyzer.result = %v, want pass", analyzerPhase["result"])
	}
	if builderPhase["status"] != "pending" {
		t.Errorf("builder.status = %v, want pending", builderPhase["status"])
	}
}

// TestGetStatus_CurrentPhase_DerivedFromAgentSessions verifies current_phase
// is derived from running agent_sessions.
func TestGetStatus_CurrentPhase_DerivedFromAgentSessions(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DERIVE-2", "Derive current phase test")
	env.InitWorkflow(t, "DERIVE-2")
	wfiID := env.GetWorkflowInstanceID(t, "DERIVE-2", "test")

	// Insert running session for analyzer
	env.InsertAgentSession(t, "sess-run", "DERIVE-2", wfiID, "analyzer", "analyzer", "")

	status, err := getWorkflowStatus(t, env, "DERIVE-2", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	currentPhase, _ := status["current_phase"].(string)
	if currentPhase != "analyzer" {
		t.Errorf("current_phase = %q, want analyzer", currentPhase)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	if analyzerPhase["status"] != "in_progress" {
		t.Errorf("analyzer.status = %v, want in_progress", analyzerPhase["status"])
	}
}

// TestGetStatus_NoCurrent_WhenNoRunningSession verifies current_phase is ""
// when no agent sessions are running.
func TestGetStatus_NoCurrent_WhenNoRunningSession(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "DERIVE-3", "No current phase test")
	env.InitWorkflow(t, "DERIVE-3")
	wfiID := env.GetWorkflowInstanceID(t, "DERIVE-3", "test")

	// Only a completed session, no running
	env.InsertAgentSession(t, "sess-done", "DERIVE-3", wfiID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "sess-done", "pass")

	status, err := getWorkflowStatus(t, env, "DERIVE-3", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	currentPhase, _ := status["current_phase"].(string)
	if currentPhase != "" {
		t.Errorf("current_phase = %q, want empty string", currentPhase)
	}
}

// TestParallelAgents_BothShowInProgress verifies the race condition fix:
// two agents running in the same layer both show in_progress simultaneously.
// Before the fix, parallel agents would overwrite each other's phase status
// in the phases JSON blob, causing one to revert to pending.
func TestParallelAgents_BothShowInProgress(t *testing.T) {
	env := NewTestEnv(t)

	phasesRaw, _ := json.Marshal([]map[string]interface{}{
		{"agent": "agent-a", "layer": 0},
		{"agent": "agent-b", "layer": 0},
		{"agent": "finalizer", "layer": 1},
	})
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "parallel-test",
		Description: "Two agents in layer 0",
		Phases:      phasesRaw,
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	env.CreateTicket(t, "PARALLEL-1", "Parallel agents test")
	_, err = env.WorkflowSvc.Init(env.ProjectID, "PARALLEL-1", &types.WorkflowInitRequest{
		Workflow: "parallel-test",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	wfiID := env.GetWorkflowInstanceID(t, "PARALLEL-1", "parallel-test")

	// Simulate both agents starting simultaneously
	env.InsertAgentSession(t, "sess-a", "PARALLEL-1", wfiID, "agent-a", "agent-a", "")
	env.InsertAgentSession(t, "sess-b", "PARALLEL-1", wfiID, "agent-b", "agent-b", "")

	status, err := getWorkflowStatus(t, env, "PARALLEL-1", &types.WorkflowGetRequest{
		Workflow: "parallel-test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	agentA, _ := phases["agent-a"].(map[string]interface{})
	agentB, _ := phases["agent-b"].(map[string]interface{})
	finalizer, _ := phases["finalizer"].(map[string]interface{})

	if agentA["status"] != "in_progress" {
		t.Errorf("agent-a.status = %v, want in_progress", agentA["status"])
	}
	if agentB["status"] != "in_progress" {
		t.Errorf("agent-b.status = %v, want in_progress (race condition fix broken)", agentB["status"])
	}
	if finalizer["status"] != "pending" {
		t.Errorf("finalizer.status = %v, want pending", finalizer["status"])
	}
}

// TestParallelAgents_OneCompletedOneRunning verifies correct status when
// one parallel agent completes while the other is still running.
func TestParallelAgents_OneCompletedOneRunning(t *testing.T) {
	env := NewTestEnv(t)

	phasesRaw, _ := json.Marshal([]map[string]interface{}{
		{"agent": "agent-a", "layer": 0},
		{"agent": "agent-b", "layer": 0},
	})
	_, err := env.WorkflowSvc.CreateWorkflowDef(env.ProjectID, &types.WorkflowDefCreateRequest{
		ID:          "parallel-mixed",
		Description: "Parallel mixed test",
		Phases:      phasesRaw,
	})
	if err != nil {
		t.Fatalf("failed to create workflow def: %v", err)
	}

	env.CreateTicket(t, "PARALLEL-2", "Parallel mixed test")
	_, err = env.WorkflowSvc.Init(env.ProjectID, "PARALLEL-2", &types.WorkflowInitRequest{
		Workflow: "parallel-mixed",
	})
	if err != nil {
		t.Fatalf("failed to init workflow: %v", err)
	}

	wfiID := env.GetWorkflowInstanceID(t, "PARALLEL-2", "parallel-mixed")

	env.InsertAgentSession(t, "sess-a", "PARALLEL-2", wfiID, "agent-a", "agent-a", "")
	env.CompleteAgentSession(t, "sess-a", "pass")
	env.InsertAgentSession(t, "sess-b", "PARALLEL-2", wfiID, "agent-b", "agent-b", "")

	status, err := getWorkflowStatus(t, env, "PARALLEL-2", &types.WorkflowGetRequest{
		Workflow: "parallel-mixed",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	agentA, _ := phases["agent-a"].(map[string]interface{})
	agentB, _ := phases["agent-b"].(map[string]interface{})

	if agentA["status"] != "completed" {
		t.Errorf("agent-a.status = %v, want completed", agentA["status"])
	}
	if agentB["status"] != "in_progress" {
		t.Errorf("agent-b.status = %v, want in_progress", agentB["status"])
	}
}

// TestGetStatus_SkippedPhaseInferred verifies that a phase with no agent session
// but a later layer that does have sessions is inferred as skipped.
func TestGetStatus_SkippedPhaseInferred(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "SKIP-1", "Skipped phase test")
	env.InitWorkflow(t, "SKIP-1")
	wfiID := env.GetWorkflowInstanceID(t, "SKIP-1", "test")

	// Only builder (layer 1) has a session — analyzer (layer 0) should be inferred as skipped
	env.InsertAgentSession(t, "sess-builder", "SKIP-1", wfiID, "builder", "builder", "")
	env.CompleteAgentSession(t, "sess-builder", "pass")

	status, err := getWorkflowStatus(t, env, "SKIP-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})
	builderPhase, _ := phases["builder"].(map[string]interface{})

	if analyzerPhase["status"] != "skipped" {
		t.Errorf("analyzer.status = %v, want skipped", analyzerPhase["status"])
	}
	if analyzerPhase["result"] != "skipped" {
		t.Errorf("analyzer.result = %v, want skipped", analyzerPhase["result"])
	}
	if builderPhase["status"] != "completed" {
		t.Errorf("builder.status = %v, want completed", builderPhase["status"])
	}
}

// TestGetStatus_ContinuedSessionsExcluded verifies that continued sessions
// are excluded from derivation (treated as pending), and the subsequent
// running session shows the correct state.
func TestGetStatus_ContinuedSessionsExcluded(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "CONT-1", "Continued exclusion test")
	env.InitWorkflow(t, "CONT-1")
	wfiID := env.GetWorkflowInstanceID(t, "CONT-1", "test")

	// Insert a continued session (old restart)
	env.InsertAgentSession(t, "sess-old", "CONT-1", wfiID, "analyzer", "analyzer", "")
	env.Pool.Exec(`UPDATE agent_sessions SET status = 'continued', result = NULL WHERE id = ?`, "sess-old")

	// Advance clock so new session has a later timestamp
	env.Clock.Advance(time.Second)

	// New running session
	env.InsertAgentSession(t, "sess-new", "CONT-1", wfiID, "analyzer", "analyzer", "")

	status, err := getWorkflowStatus(t, env, "CONT-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})

	if analyzerPhase["status"] != "in_progress" {
		t.Errorf("analyzer.status = %v, want in_progress", analyzerPhase["status"])
	}
}

// TestDeriveWorkflowProgress_WithSessions verifies that DeriveWorkflowProgress
// counts completed phases correctly from agent_sessions.
func TestDeriveWorkflowProgress_WithSessions(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PROG-1", "Progress test ticket")
	env.InitWorkflow(t, "PROG-1")
	wfiID := env.GetWorkflowInstanceID(t, "PROG-1", "test")

	// Complete analyzer, leave builder pending
	env.InsertAgentSession(t, "sess-analyzer", "PROG-1", wfiID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "sess-analyzer", "pass")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("ListActiveByProject failed: %v", err)
	}

	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)
	wp, ok := progress[strings.ToLower("PROG-1")]
	if !ok {
		t.Fatal("expected progress entry for PROG-1")
	}

	if wp.TotalPhases != 2 {
		t.Errorf("TotalPhases = %d, want 2", wp.TotalPhases)
	}
	if wp.CompletedPhases != 1 {
		t.Errorf("CompletedPhases = %d, want 1", wp.CompletedPhases)
	}
	if wp.WorkflowName != "test" {
		t.Errorf("WorkflowName = %q, want test", wp.WorkflowName)
	}
	if wp.Status != "active" {
		t.Errorf("Status = %q, want active", wp.Status)
	}
	if wp.CurrentPhase != "" {
		t.Errorf("CurrentPhase = %q, want empty (no running session)", wp.CurrentPhase)
	}
}

// TestDeriveWorkflowProgress_CurrentPhase verifies DeriveWorkflowProgress
// captures the running phase correctly and counts skipped phases as completed.
func TestDeriveWorkflowProgress_CurrentPhase(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PROG-2", "Progress current phase test")
	env.InitWorkflow(t, "PROG-2")
	wfiID := env.GetWorkflowInstanceID(t, "PROG-2", "test")

	// Only builder running (layer 1); analyzer (layer 0) has no session.
	// analyzer is inferred as skipped because builder's layer (1) > analyzer's layer (0).
	env.InsertAgentSession(t, "sess-builder", "PROG-2", wfiID, "builder", "builder", "")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("ListActiveByProject failed: %v", err)
	}

	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)
	wp, ok := progress[strings.ToLower("PROG-2")]
	if !ok {
		t.Fatal("expected progress entry for PROG-2")
	}

	if wp.CurrentPhase != "builder" {
		t.Errorf("CurrentPhase = %q, want builder", wp.CurrentPhase)
	}
	// analyzer inferred as skipped (status=skipped), builder is in_progress
	// skipped counts toward completion: 1 (analyzer skipped)
	if wp.CompletedPhases != 1 {
		t.Errorf("CompletedPhases = %d, want 1 (analyzer inferred skipped)", wp.CompletedPhases)
	}
}

// TestDeriveWorkflowProgress_AllCompleted verifies full completion progress.
func TestDeriveWorkflowProgress_AllCompleted(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "PROG-3", "Full progress test")
	env.InitWorkflow(t, "PROG-3")
	wfiID := env.GetWorkflowInstanceID(t, "PROG-3", "test")

	env.InsertAgentSession(t, "s1", "PROG-3", wfiID, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "s1", "pass")
	env.InsertAgentSession(t, "s2", "PROG-3", wfiID, "builder", "builder", "")
	env.CompleteAgentSession(t, "s2", "pass")

	// Mark workflow as completed (not returned by ListActiveByProject)
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted)

	wi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get wfi failed: %v", err)
	}

	instances := map[string]*model.WorkflowInstance{
		strings.ToLower("PROG-3"): wi,
	}
	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)

	wp, ok := progress[strings.ToLower("PROG-3")]
	if !ok {
		t.Fatal("expected progress entry for PROG-3")
	}

	if wp.CompletedPhases != 2 {
		t.Errorf("CompletedPhases = %d, want 2", wp.CompletedPhases)
	}
	if wp.TotalPhases != 2 {
		t.Errorf("TotalPhases = %d, want 2", wp.TotalPhases)
	}
}

// TestDeriveWorkflowProgress_MultipleTickets verifies DeriveWorkflowProgress
// correctly maps progress for multiple workflow instances.
func TestDeriveWorkflowProgress_MultipleTickets(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "MULTI-1", "Multi ticket 1")
	env.CreateTicket(t, "MULTI-2", "Multi ticket 2")
	env.InitWorkflow(t, "MULTI-1")
	env.InitWorkflow(t, "MULTI-2")

	wfi1 := env.GetWorkflowInstanceID(t, "MULTI-1", "test")
	wfi2 := env.GetWorkflowInstanceID(t, "MULTI-2", "test")

	// MULTI-1: analyzer completed (builder still pending)
	env.InsertAgentSession(t, "s1", "MULTI-1", wfi1, "analyzer", "analyzer", "")
	env.CompleteAgentSession(t, "s1", "pass")

	// MULTI-2: analyzer running (layer 0) and builder running (layer 1)
	// Both agents have sessions → no skipped inference
	env.InsertAgentSession(t, "s2", "MULTI-2", wfi2, "analyzer", "analyzer", "")
	env.InsertAgentSession(t, "s3", "MULTI-2", wfi2, "builder", "builder", "")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool, clock.Real())
	instances, err := wfiRepo.ListActiveByProject(env.ProjectID)
	if err != nil {
		t.Fatalf("ListActiveByProject failed: %v", err)
	}

	progress := env.WorkflowSvc.DeriveWorkflowProgress(instances)

	wp1, ok1 := progress[strings.ToLower("MULTI-1")]
	wp2, ok2 := progress[strings.ToLower("MULTI-2")]

	if !ok1 {
		t.Fatal("expected progress for MULTI-1")
	}
	if !ok2 {
		t.Fatal("expected progress for MULTI-2")
	}

	if wp1.CompletedPhases != 1 {
		t.Errorf("MULTI-1 CompletedPhases = %d, want 1", wp1.CompletedPhases)
	}
	// MULTI-2: both agents have sessions (running) → no phases are completed
	if wp2.CompletedPhases != 0 {
		t.Errorf("MULTI-2 CompletedPhases = %d, want 0 (both in_progress)", wp2.CompletedPhases)
	}
}

// TestGetStatus_PhaseOrder_FromWorkflowDef verifies that phase_order in the
// API response reflects the workflow definition order, not the phases JSON column.
func TestGetStatus_PhaseOrder_FromWorkflowDef(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "ORDER-1", "Phase order test")
	env.InitWorkflow(t, "ORDER-1")

	status, err := getWorkflowStatus(t, env, "ORDER-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phaseOrder, _ := status["phase_order"].([]interface{})
	if len(phaseOrder) != 2 {
		t.Fatalf("expected 2 phases in phase_order, got %d", len(phaseOrder))
	}
	if phaseOrder[0] != "analyzer" {
		t.Errorf("phase_order[0] = %v, want analyzer", phaseOrder[0])
	}
	if phaseOrder[1] != "builder" {
		t.Errorf("phase_order[1] = %v, want builder", phaseOrder[1])
	}
}

// TestGetStatus_FailedSession_ShowsCompletedFail verifies that a failed agent session
// results in completed/fail phase status.
func TestGetStatus_FailedSession_ShowsCompletedFail(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "FAIL-1", "Failed session test")
	env.InitWorkflow(t, "FAIL-1")
	wfiID := env.GetWorkflowInstanceID(t, "FAIL-1", "test")

	env.InsertAgentSession(t, "sess-fail", "FAIL-1", wfiID, "analyzer", "analyzer", "")
	now := env.Clock.Now().UTC().Format(time.RFC3339Nano)
	env.Pool.Exec(`UPDATE agent_sessions SET status = 'failed', result = 'fail', ended_at = ?, updated_at = ? WHERE id = ?`,
		now, now, "sess-fail")

	status, err := getWorkflowStatus(t, env, "FAIL-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	phases, _ := status["phases"].(map[string]interface{})
	analyzerPhase, _ := phases["analyzer"].(map[string]interface{})

	if analyzerPhase["status"] != "completed" {
		t.Errorf("analyzer.status = %v, want completed", analyzerPhase["status"])
	}
	if analyzerPhase["result"] != "fail" {
		t.Errorf("analyzer.result = %v, want fail", analyzerPhase["result"])
	}
}
