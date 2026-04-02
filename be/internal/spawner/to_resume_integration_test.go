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

// TestToResumeEndToEnd tests the complete flow:
// 1. Agent session is marked as continued with to_resume in findings
// 2. New agent is spawned and fetchPreviousData retrieves the to_resume value
// 3. Template expansion injects the to_resume data into ${PREVIOUS_DATA}
func TestToResumeEndToEnd(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// Step 1: Simulate the first agent saving its progress under to_resume
	firstSessionID := uuid.New().String()
	progressSummary := "I analyzed the codebase and found that we need to modify files X, Y, and Z. " +
		"The implementation approach is to use pattern A because of constraint B. " +
		"I completed 60% of the work and the remaining tasks are C and D."

	findings := map[string]interface{}{
		"to_resume":      progressSummary,
		"files_analyzed": []string{"X", "Y", "Z"},
		"other_data":     "this should not be included in PREVIOUS_DATA",
	}

	env.createContinuedSessionWithFindings(t, firstSessionID, findings)

	// Step 2: Fetch previous data (simulating what happens when spawning the next agent)
	previousData := env.spawner.fetchPreviousData(
		env.projectID,
		env.ticketID,
		env.workflowID,
		env.agentType,
		env.modelID,
		env.phase,
		"",
	)

	// Verify only the to_resume value is returned
	if previousData != progressSummary {
		t.Errorf("expected to_resume summary, got %q", previousData)
	}

	// Verify other findings are NOT included
	if containsHelper(previousData, "files_analyzed") {
		t.Errorf("previousData should not contain other findings keys")
	}
	if containsHelper(previousData, "other_data") {
		t.Errorf("previousData should not contain other findings keys")
	}

	// Step 3: Verify the data would be injected into template
	// (This simulates what template expansion does with ${PREVIOUS_DATA})
	template := "You are continuing work from a previous session.\n\nPrevious context:\n${PREVIOUS_DATA}\n\nContinue from where you left off."
	expectedInjection := "You are continuing work from a previous session.\n\nPrevious context:\n" + progressSummary + "\n\nContinue from where you left off."

	// Simple string replacement to simulate template expansion
	actualInjection := replaceTemplateVar(template, "${PREVIOUS_DATA}", previousData)

	if actualInjection != expectedInjection {
		t.Errorf("template injection failed.\nExpected:\n%s\n\nGot:\n%s", expectedInjection, actualInjection)
	}
}

// TestToResumeMultipleRestarts tests that multiple restarts work correctly,
// with each restart saving to to_resume and the next agent reading from it
func TestToResumeMultipleRestarts(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// First restart: agent saves initial progress
	session1 := uuid.New().String()
	findings1 := map[string]interface{}{
		"to_resume": "First agent completed task A and B",
	}
	env.createContinuedSessionWithTime(t, session1, findings1, time.Now().Add(-2*time.Hour))

	// Verify first restart data can be fetched
	data1 := env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, env.modelID, env.phase, "")
	if data1 != "First agent completed task A and B" {
		t.Errorf("expected first restart data, got %q", data1)
	}

	// Second restart: new agent saves updated progress
	session2 := uuid.New().String()
	findings2 := map[string]interface{}{
		"to_resume": "Second agent completed task C and D, now working on E",
	}
	env.createContinuedSessionWithTime(t, session2, findings2, time.Now().Add(-1*time.Hour))

	// Verify second restart data is now returned (most recent)
	data2 := env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, env.modelID, env.phase, "")
	if data2 != "Second agent completed task C and D, now working on E" {
		t.Errorf("expected second restart data, got %q", data2)
	}

	// Third restart: another agent saves final progress
	session3 := uuid.New().String()
	findings3 := map[string]interface{}{
		"to_resume": "Third agent completed task E, ready for final review",
	}
	env.createContinuedSessionWithTime(t, session3, findings3, time.Now())

	// Verify third restart data is now returned (most recent)
	data3 := env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, env.modelID, env.phase, "")
	if data3 != "Third agent completed task E, ready for final review" {
		t.Errorf("expected third restart data, got %q", data3)
	}
}

// TestToResumeIsolationBetweenAgents tests that to_resume data is isolated
// between different agents/models/phases
func TestToResumeIsolationBetweenAgents(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// Create continued sessions for different agents
	session1 := uuid.New().String()
	findings1 := map[string]interface{}{
		"to_resume": "implementor progress",
	}
	env.createContinuedSessionForAgent(t, session1, findings1, "implementor", "claude:opus", env.phase)

	session2 := uuid.New().String()
	findings2 := map[string]interface{}{
		"to_resume": "test-writer progress",
	}
	env.createContinuedSessionForAgent(t, session2, findings2, "test-writer", "claude:sonnet", env.phase)

	session3 := uuid.New().String()
	findings3 := map[string]interface{}{
		"to_resume": "same agent different model",
	}
	env.createContinuedSessionForAgent(t, session3, findings3, env.agentType, "claude:haiku", env.phase)

	session4 := uuid.New().String()
	findings4 := map[string]interface{}{
		"to_resume": "same agent different phase",
	}
	env.createContinuedSessionForAgent(t, session4, findings4, env.agentType, env.modelID, "different-phase")

	// Verify each agent/model/phase combination gets the correct data
	data := env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, "implementor", "claude:opus", env.phase, "")
	if data != "implementor progress" {
		t.Errorf("implementor should get its own data, got %q", data)
	}

	data = env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, "test-writer", "claude:sonnet", env.phase, "")
	if data != "test-writer progress" {
		t.Errorf("test-writer should get its own data, got %q", data)
	}

	data = env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, "claude:haiku", env.phase, "")
	if data != "same agent different model" {
		t.Errorf("different model should get its own data, got %q", data)
	}

	data = env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, env.modelID, "different-phase", "")
	if data != "same agent different phase" {
		t.Errorf("different phase should get its own data, got %q", data)
	}

	// Verify the original agent/model/phase gets empty (no continued session for it)
	data = env.spawner.fetchPreviousData(env.projectID, env.ticketID, env.workflowID, env.agentType, env.modelID, env.phase, "")
	if data != "" {
		t.Errorf("original agent/model/phase should have no data, got %q", data)
	}
}

// TestSavePromptStructure tests the structure of the save prompt uses env-var-based CLI
func TestSavePromptStructure(t *testing.T) {
	prompt := buildSavePrompt()

	// Verify the prompt has the correct structure (env-var-based, no positional args)
	tests := []struct {
		name     string
		contains string
		want     bool // true = must contain, false = must NOT contain
	}{
		{"has to_resume key", "to_resume", true},
		{"has findings add command", "nrflow findings add to_resume", true},
		{"has agent continue command", "nrflow agent continue", true},
		{"no -w flag", "-w ", false},
		{"no --model flag", "--model ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := containsHelper(prompt, tt.contains)
			if tt.want && !found {
				t.Errorf("prompt should contain %q, got: %s", tt.contains, prompt)
			}
			if !tt.want && found {
				t.Errorf("prompt should NOT contain %q (obsolete), got: %s", tt.contains, prompt)
			}
		})
	}
}

// Test environment helpers for end-to-end tests

type e2eTestEnv struct {
	database   *db.DB
	dbPath     string
	projectID  string
	ticketID   string
	workflowID string
	wfiID      string
	agentType  string
	modelID    string
	phase      string
	spawner    *Spawner
	cleanup    func()
}

func setupE2ETestEnv(t *testing.T) *e2eTestEnv {
	t.Helper()

	dbPath := "/tmp/test_to_resume_e2e_" + uuid.New().String() + ".db"
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	projectID := "test-project"
	ticketID := "TEST-" + uuid.New().String()[:8]
	workflowID := "feature"
	wfiID := uuid.New().String()
	agentType := "test-agent"
	modelID := "claude:sonnet"
	phase := "test-phase"

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

	spawner := New(Config{
		DataPath: dbPath,
		Pool:     db.WrapAsPool(database),
		Clock:    clock.Real(),
	})

	return &e2eTestEnv{
		database:   database,
		dbPath:     dbPath,
		projectID:  projectID,
		ticketID:   ticketID,
		workflowID: workflowID,
		wfiID:      wfiID,
		agentType:  agentType,
		modelID:    modelID,
		phase:      phase,
		spawner:    spawner,
		cleanup: func() {
			database.Close()
			os.Remove(dbPath)
		},
	}
}

func (env *e2eTestEnv) createContinuedSessionWithFindings(t *testing.T, sessionID string, findings map[string]interface{}) {
	t.Helper()
	env.createContinuedSessionForAgent(t, sessionID, findings, env.agentType, env.modelID, env.phase)
}

func (env *e2eTestEnv) createContinuedSessionWithTime(t *testing.T, sessionID string, findings map[string]interface{}, endedAt time.Time) {
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
		Phase:              env.phase,
		AgentType:          env.agentType,
		ModelID:            sql.NullString{String: env.modelID, Valid: true},
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

func (env *e2eTestEnv) createContinuedSessionForAgent(t *testing.T, sessionID string, findings map[string]interface{}, agentType, modelID, phase string) {
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
		Phase:              phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		Findings:           sql.NullString{String: string(findingsJSON), Valid: true},
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
		EndedAt:            sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}
}

func replaceTemplateVar(template, varName, value string) string {
	result := ""
	for i := 0; i < len(template); {
		if i <= len(template)-len(varName) && template[i:i+len(varName)] == varName {
			result += value
			i += len(varName)
		} else {
			result += string(template[i])
			i++
		}
	}
	return result
}
