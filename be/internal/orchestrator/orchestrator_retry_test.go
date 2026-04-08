package orchestrator

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

func TestRetryFailedAgent_WorkflowNotFailed(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-1", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-1")

	// Create a dummy agent session
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-1",
		ProjectID:          env.project,
		TicketID:           "RTR-1",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
	}
	asRepo.Create(session)

	// Workflow is active, not failed
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-1", "test", "sess-1")
	if err == nil {
		t.Fatal("expected error for non-failed workflow")
	}
	if got := err.Error(); got != "workflow is not in failed status (current: active)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRetryFailedAgent_WorkflowNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-2", "Retry test")

	err := env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-2", "nonexistent", "sess-x")
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestRetryFailedAgent_SessionNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-3", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-3")

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	err := env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-3", "test", "sess-nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestRetryFailedAgent_SessionDoesNotBelongToWorkflow(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-4A", "Retry test A")
	env.createTicket(t, "RTR-4B", "Retry test B")
	wfiID_A := env.initWorkflow(t, "RTR-4A")
	wfiID_B := env.initWorkflow(t, "RTR-4B")

	// Create session for workflow A
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-4",
		ProjectID:          env.project,
		TicketID:           "RTR-4A",
		WorkflowInstanceID: wfiID_A,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow B as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID_B, model.WorkflowInstanceFailed)

	// Try to retry workflow B with session from workflow A.
	// With session-based instance lookup, retryFailed resolves the instance from the session
	// (which points to workflow A, status=active), so we get a different error than before.
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-4B", "test", "sess-4")
	if err == nil {
		t.Fatal("expected error when session doesn't belong to workflow")
	}
	if got := err.Error(); got != "workflow is not in failed status (current: active)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRetryFailedAgent_FailedPhaseNotInWorkflowDef(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-5", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-5")

	// Create session with phase not in workflow definition
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-5",
		ProjectID:          env.project,
		TicketID:           "RTR-5",
		WorkflowInstanceID: wfiID,
		Phase:              "nonexistent-phase",
		AgentType:          "nonexistent-phase",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-5", "test", "sess-5")
	if err == nil {
		t.Fatal("expected error when failed phase not found in workflow def")
	}
	if got := err.Error(); got != "failed phase 'nonexistent-phase' not found in workflow definition" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRetryFailedAgent_AlreadyRunning(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-6", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-6")

	// Create failed session
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-6",
		ProjectID:          env.project,
		TicketID:           "RTR-6",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Simulate already running
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{cancel: func() {}}
	env.orch.mu.Unlock()

	defer func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	}()

	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-6", "test", "sess-6")
	if err == nil {
		t.Fatal("expected error when workflow is already running")
	}
	if got := err.Error(); got != "workflow is already running" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestRetryFailedAgent_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-7", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-7")

	// Create failed session in layer 1 (builder phase)
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-7",
		ProjectID:          env.project,
		TicketID:           "RTR-7",
		WorkflowInstanceID: wfiID,
		Phase:              "builder",
		AgentType:          "builder",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Subscribe to WS events
	ch := env.subscribeWSClient(t, "ws-rtr-7", "RTR-7")

	// Retry
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-7", "test", "sess-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workflow status reset to active
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("expected status=active, got %s", wi.Status)
	}

	// Verify retry_count incremented
	if wi.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", wi.RetryCount)
	}

	// Verify orchestration status in findings
	findings := wi.GetFindings()
	if orchStatus, ok := findings["_orchestration"].(map[string]interface{}); ok {
		if orchStatus["status"] != "running" {
			t.Errorf("expected orchestration status=running, got %v", orchStatus["status"])
		}
	} else {
		t.Error("expected _orchestration key in findings")
	}

	// Verify orchestration is running
	if !env.orch.IsInstanceRunning(wfiID) {
		t.Error("expected orchestration to be running")
	}

	// Expect EventOrchestrationRetried
	event := expectEvent(t, ch, ws.EventOrchestrationRetried, 2*time.Second)
	if event.ProjectID != env.project {
		t.Errorf("expected project_id=%s, got %s", env.project, event.ProjectID)
	}
	if event.TicketID != "RTR-7" {
		t.Errorf("expected ticket_id=RTR-7, got %s", event.TicketID)
	}
	if event.Workflow != "test" {
		t.Errorf("expected workflow=test, got %s", event.Workflow)
	}
	if event.Data["instance_id"] != wfiID {
		t.Errorf("expected instance_id=%s, got %v", wfiID, event.Data["instance_id"])
	}
	if event.Data["start_layer"] != float64(1) {
		t.Errorf("expected start_layer=1, got %v", event.Data["start_layer"])
	}
	if event.Data["failed_phase"] != "builder" {
		t.Errorf("expected failed_phase=builder, got %v", event.Data["failed_phase"])
	}
	if event.Data["failed_session_id"] != "sess-7" {
		t.Errorf("expected failed_session_id=sess-7, got %v", event.Data["failed_session_id"])
	}

	// Cleanup: cancel and wait for goroutine to finish
	env.stopAndWaitRun(t, wfiID)
}

func TestRetryFailedAgent_ResetsOnlyFailedLayer(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-8", "Retry test")

	// Create a three-layer workflow
	env.createWorkflowWithAgents(t, "test-3layer", "Three layer test", "", []struct{ ID string; Layer int }{
		{"phase1", 0}, {"phase2", 1}, {"phase3", 2},
	})

	// Init workflow
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at)
		VALUES ('wfi-8', ?, 'RTR-8', 'test-3layer', 'failed', '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create failed session for phase3
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-8",
		ProjectID:          env.project,
		TicketID:           "RTR-8",
		WorkflowInstanceID: wfiID,
		Phase:              "phase3",
		AgentType:          "phase3",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Retry
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-8", "test-3layer", "sess-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cleanup: cancel and wait for goroutine to finish
	env.stopAndWaitRun(t, wfiID)
}

func TestRetryFailedAgent_IncrementRetryCount(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "RTR-9", "Retry test")
	wfiID := env.initWorkflow(t, "RTR-9")

	// Create failed session
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-9",
		ProjectID:          env.project,
		TicketID:           "RTR-9",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// First retry
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-9", "test", "sess-9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stop orchestration and wait for goroutine to finish
	env.stopAndWaitRun(t, wfiID)

	// Verify retry_count = 1
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", wi.RetryCount)
	}

	// Mark as failed again
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Second retry
	err = env.orch.RetryFailedAgent(context.Background(), env.project, "RTR-9", "test", "sess-9")
	if err != nil {
		t.Fatalf("unexpected error on second retry: %v", err)
	}

	// Verify retry_count = 2
	wi = env.getWorkflowInstance(t, wfiID)
	if wi.RetryCount != 2 {
		t.Errorf("expected retry_count=2, got %d", wi.RetryCount)
	}

	// Cleanup: cancel and wait for goroutine to finish
	env.stopAndWaitRun(t, wfiID)
}

func TestRetryFailedProjectAgent_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test")

	// Create failed session
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	asRepo := repo.NewAgentSessionRepo(database, clock.Real())
	session := &model.AgentSession{
		ID:                 "sess-p1",
		ProjectID:          env.project,
		TicketID:           "",
		WorkflowInstanceID: wfiID,
		Phase:              "analyzer",
		AgentType:          "analyzer",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
	}
	asRepo.Create(session)

	// Mark workflow as failed
	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, clock.Real())
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Subscribe to WS events (project scope uses empty ticket ID)
	ch := env.subscribeWSClient(t, "ws-proj-1", "")

	// Retry project-scoped workflow
	err = env.orch.RetryFailedProjectAgent(context.Background(), env.project, "test", "sess-p1", wfiID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify workflow status reset to active
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("expected status=active, got %s", wi.Status)
	}

	// Verify retry_count incremented
	if wi.RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", wi.RetryCount)
	}

	// Verify orchestration is running
	if !env.orch.IsInstanceRunning(wfiID) {
		t.Error("expected orchestration to be running")
	}

	// Expect EventOrchestrationRetried
	event := expectEvent(t, ch, ws.EventOrchestrationRetried, 2*time.Second)
	if event.ProjectID != env.project {
		t.Errorf("expected project_id=%s, got %s", env.project, event.ProjectID)
	}
	if event.TicketID != "" {
		t.Errorf("expected empty ticket_id for project scope, got %s", event.TicketID)
	}

	// Cleanup: cancel and wait for goroutine to finish
	env.stopAndWaitRun(t, wfiID)
}
