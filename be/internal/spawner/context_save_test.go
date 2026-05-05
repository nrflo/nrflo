package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"syscall"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestCheckToResumeFindings_ValidToResumeKey tests that checkToResumeFindings
// returns true when to_resume key exists with non-empty string value
func TestCheckToResumeFindings_ValidToResumeKey(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"to_resume": "This is my progress summary",
		"other_key": "other value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if !result {
		t.Errorf("expected checkToResumeFindings to return true when to_resume key exists")
	}
}

// TestCheckToResumeFindings_EmptyFindings tests that checkToResumeFindings
// returns false when findings are empty
func TestCheckToResumeFindings_EmptyFindings(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when findings are empty")
	}
}

// TestCheckToResumeFindings_NullFindings tests that checkToResumeFindings
// returns false when findings column is NULL
func TestCheckToResumeFindings_NullFindings(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithNullFindings(t)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when findings are NULL")
	}
}

// TestCheckToResumeFindings_MissingToResumeKey tests that checkToResumeFindings
// returns false when findings exist but to_resume key is missing
func TestCheckToResumeFindings_MissingToResumeKey(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"some_key":  "some value",
		"other_key": "other value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when to_resume key is missing")
	}
}

// TestCheckToResumeFindings_EmptyToResumeValue tests that checkToResumeFindings
// returns false when to_resume is empty string
func TestCheckToResumeFindings_EmptyToResumeValue(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"to_resume": "",
		"other_key": "value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when to_resume is empty string")
	}
}

// TestCheckToResumeFindings_NonStringToResumeValue tests that checkToResumeFindings
// returns false when to_resume value is not a string
func TestCheckToResumeFindings_NonStringToResumeValue(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"to_resume": 12345, // number instead of string
		"other_key": "value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when to_resume is not a string")
	}
}

// TestCheckToResumeFindings_ToResumeAsArray tests that checkToResumeFindings
// returns false when to_resume value is an array
func TestCheckToResumeFindings_ToResumeAsArray(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"to_resume": []string{"item1", "item2"},
		"other_key": "value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when to_resume is an array")
	}
}

// TestCheckToResumeFindings_ToResumeAsObject tests that checkToResumeFindings
// returns false when to_resume value is an object
func TestCheckToResumeFindings_ToResumeAsObject(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	findings := map[string]interface{}{
		"to_resume": map[string]string{"nested": "value"},
		"other_key": "value",
	}
	sessionID := env.createSessionWithFindings(t, findings)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when to_resume is an object")
	}
}

// TestCheckToResumeFindings_SessionNotFound tests that checkToResumeFindings
// returns false when session doesn't exist
func TestCheckToResumeFindings_SessionNotFound(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	proc := &processInfo{
		sessionID: uuid.New().String(), // Non-existent session
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when session not found")
	}
}

// TestCheckToResumeFindings_InvalidJSON tests that checkToResumeFindings
// returns false when findings contain invalid JSON
func TestCheckToResumeFindings_InvalidJSON(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithInvalidJSON(t)

	proc := &processInfo{
		sessionID: sessionID,
	}

	result := env.spawner.checkToResumeFindings(env.ctx, proc)

	if result {
		t.Errorf("expected checkToResumeFindings to return false when findings JSON is invalid")
	}
}

// Test environment helpers

type contextSaveTestEnv struct {
	database  *db.DB
	dbPath    string
	projectID string
	ticketID  string
	wfiID     string
	spawner   *Spawner
	ctx       context.Context
	cleanup   func()
}

func setupContextSaveTestEnv(t *testing.T) *contextSaveTestEnv {
	t.Helper()

	dbPath := "/tmp/test_context_save_" + uuid.New().String() + ".db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	projectID := "test-project"
	ticketID := "TEST-" + uuid.New().String()[:8]
	workflowID := "feature"
	wfiID := uuid.New().String()

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
		INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, workflowID, "Test workflow", "ticket",
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
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

	spawner := New(Config{
		DataPath: dbPath,
		Pool:     db.WrapAsPool(database),
		Clock:    clock.Real(),
	})

	return &contextSaveTestEnv{
		database:  database,
		dbPath:    dbPath,
		projectID: projectID,
		ticketID:  ticketID,
		wfiID:     wfiID,
		spawner:   spawner,
		ctx:       context.Background(),
		cleanup: func() {
			database.Close()
			os.Remove(dbPath)
		},
	}
}

func (env *contextSaveTestEnv) createSessionWithFindings(t *testing.T, findings map[string]interface{}) string {
	t.Helper()

	sessionID := uuid.New().String()
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet", Valid: true},
		Status:             model.AgentSessionRunning,
		Findings:           sql.NullString{String: string(findingsJSON), Valid: true},
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return sessionID
}

func (env *contextSaveTestEnv) createSessionWithNullFindings(t *testing.T) string {
	t.Helper()

	sessionID := uuid.New().String()

	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet", Valid: true},
		Status:             model.AgentSessionRunning,
		Findings:           sql.NullString{Valid: false}, // NULL findings
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return sessionID
}

func (env *contextSaveTestEnv) createSessionWithInvalidJSON(t *testing.T) string {
	t.Helper()

	sessionID := uuid.New().String()

	// Directly insert invalid JSON via raw SQL
	_, err := env.database.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, findings, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, env.projectID, env.ticketID, env.wfiID, "test-phase", "test-agent", "claude:sonnet", "running",
		"{invalid json}", // Invalid JSON
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create session with invalid JSON: %v", err)
	}

	return sessionID
}

// =============================================================================
// shouldUseAgentSave tests
// =============================================================================

// fakeBackend implements ExecutionBackend purely for shouldUseAgentSave tests
// (the function only reads Name()).
type fakeBackend struct{ name string }

func (b fakeBackend) Name() string                                                       { return b.name }
func (b fakeBackend) SupportsResume() bool                                                { return false }
func (b fakeBackend) SupportsTakeControl() bool                                           { return false }
func (b fakeBackend) RequiresPrompt() bool                                                { return false }
func (b fakeBackend) TracksContext() bool                                                 { return false }
func (b fakeBackend) ParsesStructuredOutput() bool                                        { return false }
func (b fakeBackend) Start(_ context.Context, _ *processInfo, _ *prepResult) error        { return nil }
func (b fakeBackend) Kill(_ context.Context, _ *processInfo, _ syscall.Signal) error      { return nil }

func TestShouldUseAgentSave_GlobalSettingForcesAgent(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: true, Clock: clock.Real()})
	proc := &processInfo{modelID: "claude:sonnet"}
	if !s.shouldUseAgentSave(proc) {
		t.Error("global setting=true must force agent save regardless of adapter")
	}
}

func TestShouldUseAgentSave_APIBackendForcesAgent(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{
		modelID: "claude:sonnet",
		backend: fakeBackend{name: "api"},
	}
	if !s.shouldUseAgentSave(proc) {
		t.Error("api backend must force agent save (no resume path for in-process API runs)")
	}
}

// TestShouldUseAgentSave_CodexForcesAgent is the regression test for the
// codex-interactive low-context fix: codex's adapter returns SupportsResume()=
// false, so the resume path would silently skip the save and relaunch with
// empty PREVIOUS_DATA. shouldUseAgentSave must route around that.
func TestShouldUseAgentSave_CodexForcesAgent(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{
		modelID: "codex:gpt-5.3-codex",
		backend: fakeBackend{name: "cli_interactive"},
	}
	if !s.shouldUseAgentSave(proc) {
		t.Error("codex must force agent save (SupportsResume=false would otherwise short-circuit)")
	}
}

func TestShouldUseAgentSave_ClaudeUsesResume(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{
		modelID: "claude:sonnet",
		backend: fakeBackend{name: "cli"},
	}
	if s.shouldUseAgentSave(proc) {
		t.Error("claude with default settings must use resume path; got forced agent save")
	}
}

func TestShouldUseAgentSave_OpencodeForcesAgent(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{
		modelID: "opencode:openai/gpt-5.4",
		backend: fakeBackend{name: "cli"},
	}
	if !s.shouldUseAgentSave(proc) {
		t.Error("opencode must force agent save (SupportsResume=false)")
	}
}

func TestShouldUseAgentSave_UnknownAdapterFallsThrough(t *testing.T) {
	t.Parallel()
	s := New(Config{ContextSaveViaAgent: false, Clock: clock.Real()})
	proc := &processInfo{modelID: "unknown:weird"}
	if s.shouldUseAgentSave(proc) {
		t.Error("unknown adapter must NOT force agent save (graceful fallback to resume; resume itself will warn)")
	}
}

