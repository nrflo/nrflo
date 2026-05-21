package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// setWFIStatus directly sets the workflow instance status in the DB.
func setWFIStatus(t *testing.T, env *testEnv, wfiID string, status model.WorkflowInstanceStatus) {
	t.Helper()
	_, err := env.pool.Exec(`UPDATE workflow_instances SET status = ? WHERE id = ?`, string(status), wfiID)
	if err != nil {
		t.Fatalf("setWFIStatus(%s, %s): %v", wfiID, status, err)
	}
}

// seedPauseFinding upserts a _pause finding with the given resume_layer for ContinueWorkflow tests.
func seedPauseFinding(t *testing.T, env *testEnv, wfiID string, resumeLayer int) {
	t.Helper()
	val, _ := json.Marshal(map[string]interface{}{
		"paused_after_layer": resumeLayer - 1,
		"resume_layer":       resumeLayer,
		"event":              map[string]interface{}{},
		"timestamp":          "2026-01-01T00:00:00Z",
	})
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	if err := findingRepo.Upsert("workflow_instance", wfiID, "_pause", val,
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"}); err != nil {
		t.Fatalf("seedPauseFinding: %v", err)
	}
}

// TestContinueWorkflow_NotWaiting_Error verifies error when status != waiting.
func TestContinueWorkflow_NotWaiting_Error(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-E1", "not waiting")
	wfiID := env.initWorkflow(t, "CON-E1") // status = active

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-E1", "test", wfiID, "")
	if err == nil {
		t.Fatalf("ContinueWorkflow on active instance: want error, got nil")
	}
}

// TestContinueWorkflow_AlreadyRunning_Error verifies error when instance is already in o.runs.
func TestContinueWorkflow_AlreadyRunning_Error(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-E2", "already running")
	wfiID := env.initWorkflow(t, "CON-E2")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	seedPauseFinding(t, env, wfiID, 1)

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-E2", "test", wfiID, "")
	if err == nil {
		t.Fatalf("ContinueWorkflow on already-running instance: want error, got nil")
	}
}

// TestContinueWorkflow_HappyPath verifies status→active and EventWorkflowResumed on success.
func TestContinueWorkflow_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-HP1", "happy path")
	wfiID := env.initWorkflow(t, "CON-HP1")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	seedPauseFinding(t, env, wfiID, 1)
	ch := env.subscribeWSClient(t, "ws-con-hp1", "CON-HP1")

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-HP1", "test", wfiID, "")
	if err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}

	// Status is set synchronously before goroutine launch.
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status = %v, want active immediately after ContinueWorkflow", wi.Status)
	}

	// EventWorkflowResumed is broadcast synchronously before goroutine launch.
	ev := expectEvent(t, ch, ws.EventWorkflowResumed, 2*time.Second)
	if ev.Data["instance_id"] != wfiID {
		t.Errorf("event instance_id = %v, want %v", ev.Data["instance_id"], wfiID)
	}

	env.stopAndWaitRun(t, wfiID)
}

// TestContinueWorkflow_WithInstructions verifies instructions are stored in user_instructions finding.
func TestContinueWorkflow_WithInstructions(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-I1", "with instructions")
	wfiID := env.initWorkflow(t, "CON-I1")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	seedPauseFinding(t, env, wfiID, 1)

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-I1", "test", wfiID, "extra context")
	if err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}

	findings := getWFIFindings(t, env, wfiID)
	raw, ok := findings["user_instructions"]
	if !ok {
		t.Fatalf("user_instructions finding absent after ContinueWorkflow with instructions")
	}
	instrStr, _ := raw.(string)
	if instrStr != "extra context" {
		t.Errorf("user_instructions = %q, want %q", instrStr, "extra context")
	}

	env.stopAndWaitRun(t, wfiID)
}

// TestContinueWorkflow_AppendInstructions verifies existing instructions are preserved with separator.
func TestContinueWorkflow_AppendInstructions(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-I2", "append instructions")
	wfiID := env.initWorkflow(t, "CON-I2")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	seedPauseFinding(t, env, wfiID, 1)

	existingVal, _ := json.Marshal("previous instructions")
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	_ = findingRepo.Upsert("workflow_instance", wfiID, "user_instructions", existingVal,
		repo.Denorm{WorkflowInstanceID: wfiID}, repo.Actor{Source: "orchestrator"})

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-I2", "test", wfiID, "new instruction")
	if err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}

	findings := getWFIFindings(t, env, wfiID)
	instrStr, _ := findings["user_instructions"].(string)
	const want = "previous instructions\n---\nnew instruction"
	if instrStr != want {
		t.Errorf("user_instructions = %q, want %q", instrStr, want)
	}

	env.stopAndWaitRun(t, wfiID)
}

// TestContinueWorkflow_NoInstructions verifies no user_instructions finding when instructions is empty.
func TestContinueWorkflow_NoInstructions(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "CON-I3", "no instructions")
	wfiID := env.initWorkflow(t, "CON-I3")
	setWFIStatus(t, env, wfiID, model.WorkflowInstanceWaiting)
	seedPauseFinding(t, env, wfiID, 1)

	err := env.orch.ContinueWorkflow(context.Background(), env.project, "CON-I3", "test", wfiID, "")
	if err != nil {
		t.Fatalf("ContinueWorkflow: %v", err)
	}

	findings := getWFIFindings(t, env, wfiID)
	if _, ok := findings["user_instructions"]; ok {
		t.Errorf("user_instructions finding present with empty instructions, want absent")
	}

	env.stopAndWaitRun(t, wfiID)
}
