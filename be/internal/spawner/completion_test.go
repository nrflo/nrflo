package spawner

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"

	"github.com/google/uuid"
)

// testEnv holds the test environment
type testEnv struct {
	database   *db.DB
	dbPath     string
	sessionID  string
	projectID  string
	ticketID   string
	workflowID string
	wfiID      string
	spawner    *Spawner
	cleanup    func()
}

// setupTestEnv creates a complete test environment with database, workflow, and spawner
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dbPath := "/tmp/test_completion_" + uuid.New().String() + ".db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	projectID := "test-project"
	workflowID := "feature"
	wfiID := uuid.New().String()
	ticketID := "TEST-" + uuid.New().String()[:8]

	// Create project
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		projectID, "Test Project", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow
	_, err = database.Exec(`
		INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, workflowID, "Test workflow", "ticket",
		"[]", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Create workflow instance
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, projectID, ticketID, workflowID, "ticket", "active",
		"{}", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create spawner
	hub := ws.NewHub(clock.Real())
	spawner := New(Config{
		DataPath: dbPath,
		WSHub:    hub,
		Pool:               db.WrapAsPool(database),
		Clock:              clock.Real(),
	})

	return &testEnv{
		database:   database,
		dbPath:     dbPath,
		sessionID:  uuid.New().String(),
		projectID:  projectID,
		ticketID:   ticketID,
		workflowID: workflowID,
		wfiID:      wfiID,
		spawner:    spawner,
		cleanup: func() {
			database.Close()
			os.Remove(dbPath)
		},
	}
}

// createSession creates a test agent session
func (env *testEnv) createSession(t *testing.T, modelID string) *model.AgentSession {
	t.Helper()

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 env.sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return session
}

// TestHandleCompletion_ExitZeroNoExplicitCompletion tests the fallback case
// where agent exits cleanly (code 0) but does not call agent complete/fail/continue
func TestHandleCompletion_ExitZeroNoExplicitCompletion(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	// Command that exits with code 0
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-10 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	// Verify result in database
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "pass" {
		t.Errorf("expected result='pass', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "implicit" {
		t.Errorf("expected result_reason='implicit', got %v", updatedSession.ResultReason)
	}

	if updatedSession.Status != model.AgentSessionCompleted {
		t.Errorf("expected status='completed', got %v", updatedSession.Status)
	}

	if proc.finalStatus != "PASS" {
		t.Errorf("expected finalStatus='PASS', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_ExitZeroIgnoresExplicitPass tests that exit 0 is always
// implicit pass — explicit pass in DB is ignored (agents should not call agent complete)
func TestHandleCompletion_ExitZeroIgnoresExplicitPass(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	// Even if result='pass' is set in DB, handleCompletion should treat exit 0 as implicit
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateResult(env.sessionID, "pass", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-5 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "pass" {
		t.Errorf("expected result='pass', got %v", updatedSession.Result)
	}

	// Should be implicit, not explicit — exit 0 = implicit pass regardless of DB state
	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "implicit" {
		t.Errorf("expected result_reason='implicit', got %v", updatedSession.ResultReason)
	}

	if proc.finalStatus != "PASS" {
		t.Errorf("expected finalStatus='PASS', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_ExitZeroWithExplicitFail tests that explicit fail
// is detected during the grace period
func TestHandleCompletion_ExitZeroWithExplicitFail(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:haiku")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateResult(env.sessionID, "fail", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:haiku",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-3 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "fail" {
		t.Errorf("expected result='fail', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "explicit" {
		t.Errorf("expected result_reason='explicit', got %v", updatedSession.ResultReason)
	}

	if updatedSession.Status != model.AgentSessionFailed {
		t.Errorf("expected status='failed', got %v", updatedSession.Status)
	}

	if proc.finalStatus != "FAIL" {
		t.Errorf("expected finalStatus='FAIL', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_ExitZeroWithExplicitContinue tests that explicit continue
// is detected during the grace period
func TestHandleCompletion_ExitZeroWithExplicitContinue(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateResult(env.sessionID, "continue", "explicit"); err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-7 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "continue" {
		t.Errorf("expected result='continue', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "explicit" {
		t.Errorf("expected result_reason='explicit', got %v", updatedSession.ResultReason)
	}

	if updatedSession.Status != model.AgentSessionContinued {
		t.Errorf("expected status='continued', got %v", updatedSession.Status)
	}

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("expected finalStatus='CONTINUE', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_NonZeroExitCode tests that non-zero exit code
// results in immediate fail with exit_code reason
func TestHandleCompletion_NonZeroExitCode(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:opus")

	// Command that exits with non-zero code
	cmd := exec.Command("false")
	_ = cmd.Run() // Ignore error, we expect it to fail

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:opus",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-2 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.Result.Valid || updatedSession.Result.String != "fail" {
		t.Errorf("expected result='fail', got %v", updatedSession.Result)
	}

	if !updatedSession.ResultReason.Valid || updatedSession.ResultReason.String != "exit_code" {
		t.Errorf("expected result_reason='exit_code', got %v", updatedSession.ResultReason)
	}

	if updatedSession.Status != model.AgentSessionFailed {
		t.Errorf("expected status='failed', got %v", updatedSession.Status)
	}

	if proc.finalStatus != "FAIL" {
		t.Errorf("expected finalStatus='FAIL', got %v", proc.finalStatus)
	}
}

// TestHandleCompletion_EndedAtTimestamp tests that ended_at timestamp is set
func TestHandleCompletion_EndedAtTimestamp(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet")

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run test command: %v", err)
	}

	beforeCompletion := time.Now().UTC()
	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-10 * time.Second),
	}

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	updatedSession, err := sessionRepo.Get(env.sessionID)
	if err != nil {
		t.Fatalf("failed to get updated session: %v", err)
	}

	if !updatedSession.EndedAt.Valid {
		t.Errorf("expected ended_at to be set")
	}

	endedAt, err := time.Parse(time.RFC3339Nano, updatedSession.EndedAt.String)
	if err != nil {
		t.Fatalf("failed to parse ended_at: %v", err)
	}

	if endedAt.Before(beforeCompletion) {
		t.Errorf("ended_at should be after completion started")
	}

	if time.Since(endedAt) > 5*time.Second {
		t.Errorf("ended_at should be recent, got %v ago", time.Since(endedAt))
	}
}
