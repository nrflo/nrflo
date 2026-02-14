package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestCheckToResumeFindings_ValidToResumeKey tests that checkToResumeFindings
// returns true when to_resume key exists with non-empty string value
func TestCheckToResumeFindings_ValidToResumeKey(t *testing.T) {
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

// TestBuildSavePrompt_CorrectFormat tests that buildSavePrompt produces
// the correct format with all parameters
func TestBuildSavePrompt_CorrectFormat(t *testing.T) {
	tests := []struct {
		name         string
		ticketID     string
		agentType    string
		workflowName string
		modelID      string
	}{
		{
			name:         "standard parameters",
			ticketID:     "TICKET-123",
			agentType:    "implementor",
			workflowName: "feature",
			modelID:      "claude:sonnet",
		},
		{
			name:         "different model",
			ticketID:     "TICKET-456",
			agentType:    "qa-verifier",
			workflowName: "bugfix",
			modelID:      "claude:opus",
		},
		{
			name:         "haiku model",
			ticketID:     "TICKET-789",
			agentType:    "doc-updater",
			workflowName: "docs",
			modelID:      "claude:haiku",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildSavePrompt(tt.ticketID, tt.agentType, tt.workflowName, tt.modelID)

			// Verify it contains to_resume instruction
			if !containsHelper(prompt, "to_resume:") {
				t.Errorf("prompt should contain 'to_resume:' instruction")
			}

			// Verify it contains both findings add and agent continue commands
			if !containsHelper(prompt, "nrworkflow findings add") {
				t.Errorf("prompt should contain 'nrworkflow findings add'")
			}

			if !containsHelper(prompt, "nrworkflow agent continue") {
				t.Errorf("prompt should contain 'nrworkflow agent continue'")
			}

			// Verify it contains all required parameters
			if !containsHelper(prompt, tt.ticketID) {
				t.Errorf("prompt should contain ticket ID '%s'", tt.ticketID)
			}

			if !containsHelper(prompt, tt.agentType) {
				t.Errorf("prompt should contain agent type '%s'", tt.agentType)
			}

			if !containsHelper(prompt, tt.workflowName) {
				t.Errorf("prompt should contain workflow name '%s'", tt.workflowName)
			}

			if !containsHelper(prompt, tt.modelID) {
				t.Errorf("prompt should contain model ID '%s'", tt.modelID)
			}

			// Verify the order: findings add should come before agent continue
			addIdx := indexOfSubstring(prompt, "nrworkflow findings add")
			continueIdx := indexOfSubstring(prompt, "nrworkflow agent continue")
			if addIdx == -1 || continueIdx == -1 || addIdx >= continueIdx {
				t.Errorf("'findings add' should appear before 'agent continue'")
			}
		})
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
		projectID, "Test Project", time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Create workflow
	_, err = database.Exec(`
		INSERT INTO workflows (project_id, id, description, scope_type, phases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, workflowID, "Test workflow", "ticket",
		"[]", time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Create workflow instance
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, phase_order, phases, findings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, projectID, ticketID, workflowID, "ticket", "active",
		"[]", "{}", "{}", time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	spawner := New(Config{
		DataPath: dbPath,
		Pool:     db.WrapAsPool(database),
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

	sessionRepo := repo.NewAgentSessionRepo(env.database)
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
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return sessionID
}

func (env *contextSaveTestEnv) createSessionWithNullFindings(t *testing.T) string {
	t.Helper()

	sessionID := uuid.New().String()

	sessionRepo := repo.NewAgentSessionRepo(env.database)
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
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339), Valid: true},
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
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to create session with invalid JSON: %v", err)
	}

	return sessionID
}

// indexOfSubstring returns the index of the first occurrence of substr in s,
// or -1 if not found
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
