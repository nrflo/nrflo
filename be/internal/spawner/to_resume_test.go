package spawner

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestFetchPreviousData_WithToResumeKey tests that fetchPreviousData extracts
// the to_resume key from a continued session's findings
func TestFetchPreviousData_WithToResumeKey(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Create a continued session with to_resume in findings
	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": "This is the summary of all my progress and findings so far",
		"other_key": "should be ignored",
	}
	env.createContinuedSession(t, continuedSessionID, findings)

	// Fetch previous data
	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	expected := "This is the summary of all my progress and findings so far"
	if result != expected {
		t.Errorf("expected to_resume value, got %q", result)
	}
}

// TestFetchPreviousData_WithoutToResumeKey tests that fetchPreviousData returns
// empty string when findings don't have a to_resume key
func TestFetchPreviousData_WithoutToResumeKey(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Create a continued session WITHOUT to_resume in findings
	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"some_key":  "some value",
		"other_key": "other value",
	}
	env.createContinuedSession(t, continuedSessionID, findings)

	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string when to_resume key is missing, got %q", result)
	}
}

// TestFetchPreviousData_EmptyToResumeValue tests that empty to_resume value
// returns empty string
func TestFetchPreviousData_EmptyToResumeValue(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": "",
		"other_key": "value",
	}
	env.createContinuedSession(t, continuedSessionID, findings)

	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string when to_resume is empty, got %q", result)
	}
}

// TestFetchPreviousData_NonStringToResume tests that non-string to_resume value
// returns empty string
func TestFetchPreviousData_NonStringToResume(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": 123, // number instead of string
		"other_key": "value",
	}
	env.createContinuedSession(t, continuedSessionID, findings)

	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string when to_resume is not a string, got %q", result)
	}
}

// TestFetchPreviousData_NoFindings tests that empty findings return empty string
func TestFetchPreviousData_NoFindings(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{}
	env.createContinuedSession(t, continuedSessionID, findings)

	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string when findings are empty, got %q", result)
	}
}

// TestFetchPreviousData_NoContinuedSession tests that no continued session
// returns empty string
func TestFetchPreviousData_NoContinuedSession(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Don't create any continued session
	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string when no continued session exists, got %q", result)
	}
}

// TestFetchPreviousData_LatestContinuedSession tests that fetchPreviousData
// returns data from the most recent continued session when multiple exist
func TestFetchPreviousData_LatestContinuedSession(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Create first continued session (older)
	older := uuid.New().String()
	findingsOlder := map[string]interface{}{
		"to_resume": "older summary",
	}
	env.createContinuedSessionWithTime(t, older, findingsOlder, time.Now().Add(-1*time.Hour))

	// Create second continued session (newer)
	newer := uuid.New().String()
	findingsNewer := map[string]interface{}{
		"to_resume": "newer summary",
	}
	env.createContinuedSessionWithTime(t, newer, findingsNewer, time.Now())

	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "newer summary" {
		t.Errorf("expected latest to_resume value 'newer summary', got %q", result)
	}
}

// TestFetchPreviousData_DifferentAgentType tests that fetchPreviousData only
// retrieves data for matching agent type
func TestFetchPreviousData_DifferentAgentType(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Create continued session for different agent type
	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": "different agent summary",
	}
	env.createContinuedSessionForAgent(t, continuedSessionID, findings, "other-agent")

	// Query for test-agent (should not find it)
	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string for different agent type, got %q", result)
	}
}

// TestFetchPreviousData_DifferentModelID tests that fetchPreviousData only
// retrieves data for matching model ID
func TestFetchPreviousData_DifferentModelID(t *testing.T) {
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	// Create continued session with different model
	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": "opus model summary",
	}
	env.createContinuedSessionWithModel(t, continuedSessionID, findings, "claude:opus")

	// Query for claude:sonnet (should not find it)
	result := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	if result != "" {
		t.Errorf("expected empty string for different model ID, got %q", result)
	}
}

// TestFetchPreviousData_ProjectScope tests fetchPreviousData for project-scoped workflows
func TestFetchPreviousData_ProjectScope(t *testing.T) {
	env := setupToResumeTestEnvProjectScope(t)
	defer env.cleanup()

	continuedSessionID := uuid.New().String()
	findings := map[string]interface{}{
		"to_resume": "project scope summary",
	}
	env.createContinuedSession(t, continuedSessionID, findings)

	result := env.spawner.fetchPreviousData(
		env.projectID,
		"", // empty ticket ID for project scope
		env.workflowID,
		"test-agent",
		"claude:sonnet",
		"test-phase",
		"",
	)

	expected := "project scope summary"
	if result != expected {
		t.Errorf("expected to_resume value for project scope, got %q", result)
	}
}

// TestSavePromptFormat tests that the save prompt uses env-var-based CLI commands
func TestSavePromptFormat(t *testing.T) {
	savePrompt := buildSavePrompt()

	// Verify it contains the correct CLI commands
	if !contains(savePrompt, "nrflow findings add to_resume") {
		t.Errorf("savePrompt should contain 'nrflow findings add to_resume', got: %s", savePrompt)
	}

	if !contains(savePrompt, "nrflow agent continue") {
		t.Errorf("savePrompt should contain 'nrflow agent continue', got: %s", savePrompt)
	}

	// Verify it does NOT contain obsolete flags
	if contains(savePrompt, "-w ") {
		t.Errorf("savePrompt should NOT contain '-w' flag (obsolete), got: %s", savePrompt)
	}
	if contains(savePrompt, "--model ") {
		t.Errorf("savePrompt should NOT contain '--model' flag (obsolete), got: %s", savePrompt)
	}
}

// Test helper functions

// toResumeTestEnv holds the test environment for to_resume tests
type toResumeTestEnv struct {
	database   *db.DB
	dbPath     string
	projectID  string
	ticketID   string
	workflowID string
	wfiID      string
	spawner    *Spawner
	cleanup    func()
}

// setupToResumeTestEnv creates a test environment for to_resume tests
func setupToResumeTestEnv(t *testing.T) *toResumeTestEnv {
	t.Helper()
	return setupToResumeTestEnvInternal(t, "ticket")
}

// setupToResumeTestEnvProjectScope creates a test environment for project-scoped workflows
func setupToResumeTestEnvProjectScope(t *testing.T) *toResumeTestEnv {
	t.Helper()
	return setupToResumeTestEnvInternal(t, "project")
}

// setupToResumeTestEnvInternal creates a test environment with specified scope
func setupToResumeTestEnvInternal(t *testing.T, scopeType string) *toResumeTestEnv {
	t.Helper()

	dbPath := "/tmp/test_to_resume_" + uuid.New().String() + ".db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	projectID := "test-project"
	workflowID := "feature"
	wfiID := uuid.New().String()
	ticketID := ""
	if scopeType == "ticket" {
		ticketID = "TEST-" + uuid.New().String()[:8]
	}

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
		projectID, workflowID, "Test workflow", scopeType,
		"[]", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Create workflow instance
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, projectID, ticketID, workflowID, scopeType, "active",
		"{}", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	spawner := New(Config{
		DataPath: dbPath,
		Pool:     db.WrapAsPool(database),
		Clock:    clock.Real(),
	})

	return &toResumeTestEnv{
		database:   database,
		dbPath:     dbPath,
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

// createContinuedSession creates a continued agent session with findings
func (env *toResumeTestEnv) createContinuedSession(t *testing.T, sessionID string, findings map[string]interface{}) {
	t.Helper()
	env.createContinuedSessionWithTime(t, sessionID, findings, time.Now())
}

// createContinuedSessionWithTime creates a continued session with specific ended_at time
func (env *toResumeTestEnv) createContinuedSessionWithTime(t *testing.T, sessionID string, findings map[string]interface{}, endedAt time.Time) {
	t.Helper()
	env.createContinuedSessionFull(t, sessionID, findings, "test-agent", "claude:sonnet", endedAt)
}

// createContinuedSessionForAgent creates a continued session for a specific agent type
func (env *toResumeTestEnv) createContinuedSessionForAgent(t *testing.T, sessionID string, findings map[string]interface{}, agentType string) {
	t.Helper()
	env.createContinuedSessionFull(t, sessionID, findings, agentType, "claude:sonnet", time.Now())
}

// createContinuedSessionWithModel creates a continued session with a specific model ID
func (env *toResumeTestEnv) createContinuedSessionWithModel(t *testing.T, sessionID string, findings map[string]interface{}, modelID string) {
	t.Helper()
	env.createContinuedSessionFull(t, sessionID, findings, "test-agent", modelID, time.Now())
}

// createContinuedSessionFull creates a continued session with all parameters
func (env *toResumeTestEnv) createContinuedSessionFull(t *testing.T, sessionID string, findings map[string]interface{}, agentType, modelID string, endedAt time.Time) {
	t.Helper()

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
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		Findings:           sql.NullString{String: string(findingsJSON), Valid: true},
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
		EndedAt:            sql.NullString{String: endedAt.UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
