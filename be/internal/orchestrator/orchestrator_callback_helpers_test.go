package orchestrator

import (
	"database/sql"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// insertWFI inserts a workflow instance and returns its ID.
func insertWFI(t *testing.T, env *testEnv, id, ticketID, workflowID string) string {
	t.Helper()
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, id, env.project, ticketID, workflowID).Scan(&wfiID)
	if err != nil {
		t.Fatalf("insertWFI: %v", err)
	}
	return wfiID
}

// insertWFIWithFindings inserts a workflow instance with preset findings.
func insertWFIWithFindings(t *testing.T, env *testEnv, id, ticketID, workflowID, findingsJSON string) string {
	t.Helper()
	var wfiID string
	err := env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', ?, 0, datetime('now'), datetime('now'))
		RETURNING id`, id, env.project, ticketID, workflowID, findingsJSON).Scan(&wfiID)
	if err != nil {
		t.Fatalf("insertWFIWithFindings: %v", err)
	}
	return wfiID
}

// openAsRepo opens a fresh AgentSessionRepo using the test db path.
func openAsRepo(t *testing.T, env *testEnv) (*db.DB, *repo.AgentSessionRepo) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("openAsRepo: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database, repo.NewAgentSessionRepo(database, clock.Real())
}

// createSession inserts an agent session with the given status.
func createSession(t *testing.T, asRepo *repo.AgentSessionRepo, id, projectID, ticketID, wfiID, phase string, status model.AgentSessionStatus) {
	t.Helper()
	if err := asRepo.Create(&model.AgentSession{
		ID:                 id,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              phase,
		AgentType:          phase,
		Status:             status,
		Findings:           sql.NullString{String: `{"key":"val"}`, Valid: true},
	}); err != nil {
		t.Fatalf("createSession %s: %v", id, err)
	}
}
