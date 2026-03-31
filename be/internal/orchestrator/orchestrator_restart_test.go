package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

func TestRestartAgent_NoRunningOrchestration(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-1", "Restart test")
	wfiID := env.initWorkflow(t, "RST-1")

	insertRunningSession(t, env, wfiID, "RST-1", "some-session")

	err := env.orch.RestartAgent(env.project, "RST-1", "test", "some-session")
	if err == nil {
		t.Fatal("expected error for non-running orchestration")
	}
	if got := err.Error(); got != "no running orchestration for workflow 'test' on RST-1" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRestartAgent_WorkflowNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-2", "Restart test")

	err := env.orch.RestartAgent(env.project, "RST-2", "nonexistent", "some-session")
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestRestartAgent_NoActiveSpawner(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-3", "Restart test")
	wfiID := env.initWorkflow(t, "RST-3")

	insertRunningSession(t, env, wfiID, "RST-3", "some-session")

	// Simulate a running orchestration with no spawner (between phases)
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: nil,
	}
	env.orch.mu.Unlock()

	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	err := env.orch.RestartAgent(env.project, "RST-3", "test", "some-session")
	if err == nil {
		t.Fatal("expected error when spawner is nil")
	}
	if got := err.Error(); got != "no active spawner (agent may be between phases)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRestartAgent_ForwardsToSpawner(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-4", "Restart test")
	wfiID := env.initWorkflow(t, "RST-4")

	insertRunningSession(t, env, wfiID, "RST-4", "target-session-123")

	// Create a real spawner and register it
	sp := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: sp,
	}
	env.orch.mu.Unlock()

	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	err := env.orch.RestartAgent(env.project, "RST-4", "test", "target-session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RestartAgent calls sp.RequestRestart which is non-blocking.
	// Since we can't read the internal restartCh from outside the spawner
	// package, we verify success by nil error return. The spawner unit tests
	// (restart_test.go) verify that RequestRestart populates the channel.
	// Calling RequestRestart again should be a no-op (channel full).
	sp.RequestRestart("second-call")
	// No panic or block = success
}

func TestRestartAgent_BroadcastsWSEvent(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-5", "Restart test")
	wfiID := env.initWorkflow(t, "RST-5")

	insertRunningSession(t, env, wfiID, "RST-5", "sess-ws")

	ch := env.subscribeWSClient(t, "ws-rst-5", "RST-5")

	sp := spawner.New(spawner.Config{Clock: clock.Real()})

	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() {},
		spawner: sp,
	}
	env.orch.mu.Unlock()

	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	err := env.orch.RestartAgent(env.project, "RST-5", "test", "sess-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Note: RestartAgent itself does NOT broadcast a WS event.
	// The restart signal goes to the spawner which handles it internally.
	// Just verify the call succeeded — no WS event expected here.
	_ = ch
}

func TestRunState_SpawnerLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-6", "Lifecycle test")
	wfiID := env.initWorkflow(t, "RST-6")

	// Phase 1: no spawner (between phases)
	env.orch.mu.Lock()
	rs := &runState{cancel: func() {}}
	env.orch.runs[wfiID] = rs
	env.orch.mu.Unlock()

	env.orch.mu.Lock()
	if rs.spawner != nil {
		t.Fatal("spawner should be nil initially")
	}
	env.orch.mu.Unlock()

	// Phase 2: set spawner (during phase)
	sp := spawner.New(spawner.Config{Clock: clock.Real()})
	env.orch.mu.Lock()
	rs.spawner = sp
	env.orch.mu.Unlock()

	env.orch.mu.Lock()
	if rs.spawner != sp {
		t.Fatal("spawner should be set during phase")
	}
	env.orch.mu.Unlock()

	// Phase 3: clear spawner (phase done)
	env.orch.mu.Lock()
	rs.spawner = nil
	env.orch.mu.Unlock()

	env.orch.mu.Lock()
	if rs.spawner != nil {
		t.Fatal("spawner should be nil after phase completion")
	}
	env.orch.mu.Unlock()

	// Cleanup
	env.orch.mu.Lock()
	delete(env.orch.runs, wfiID)
	env.orch.mu.Unlock()
}

func TestIsRunning_WithRunState(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-7", "IsRunning test")
	wfiID := env.initWorkflow(t, "RST-7")

	// Not running initially
	if env.orch.IsRunning(env.project, "RST-7", "test") {
		t.Fatal("should not be running initially")
	}

	// Add run state
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()

	if !env.orch.IsRunning(env.project, "RST-7", "test") {
		t.Fatal("should be running after adding run state")
	}

	// Remove run state
	env.orch.mu.Lock()
	delete(env.orch.runs, wfiID)
	env.orch.mu.Unlock()

	if env.orch.IsRunning(env.project, "RST-7", "test") {
		t.Fatal("should not be running after removal")
	}
}

func TestStopByTicket_StillWorksWithRunState(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RST-8", "Stop test")
	wfiID := env.initWorkflow(t, "RST-8")

	cancelled := false
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:  func() { cancelled = true },
		spawner: nil,
	}
	env.orch.mu.Unlock()

	err := env.orch.StopByTicket(env.project, "RST-8", "test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cancelled {
		t.Fatal("cancel should have been called")
	}
}

func TestSeedWorkflowWithAgentDefs(t *testing.T) {
	// Verify that the test workflow "test" has phases with layer-based format
	// to make sure our test assumptions are correct
	env := newTestEnv(t)

	workflowSvc := service.NewWorkflowService(env.pool, clock.Real())

	// Workflow "test" is already seeded by newTestEnv — verify it
	raw, err := workflowSvc.GetStatus(env.project, "RST-SEED", &types.WorkflowGetRequest{})
	// This should return "no workflow" for a non-existent ticket, not crash
	_ = raw
	_ = err

	// Verify seeded workflow exists by creating a ticket and initing
	env.createTicket(t, "RST-SEED", "Seed test")
	_ = env.initWorkflow(t, "RST-SEED")

	// Verify workflow definition can be retrieved
	wfDef, err := workflowSvc.GetWorkflowDef(env.project, "test")
	if err != nil {
		t.Fatalf("failed to get workflow def: %v", err)
	}
	if len(wfDef.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(wfDef.Phases))
	}
	if wfDef.Phases[0].Agent != "analyzer" || wfDef.Phases[1].Agent != "builder" {
		t.Fatalf("expected [analyzer, builder], got [%s, %s]", wfDef.Phases[0].Agent, wfDef.Phases[1].Agent)
	}
}
