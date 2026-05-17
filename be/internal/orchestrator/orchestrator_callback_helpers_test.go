package orchestrator

import (
	"encoding/json"
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
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0, datetime('now'), datetime('now'))
		RETURNING id`, id, env.project, ticketID, workflowID).Scan(&wfiID)
	if err != nil {
		t.Fatalf("insertWFI: %v", err)
	}
	return wfiID
}

// insertWFIWithFindings inserts a workflow instance with preset findings seeded via FindingRepo.
func insertWFIWithFindings(t *testing.T, env *testEnv, id, ticketID, workflowID, findingsJSON string) string {
	t.Helper()
	wfiID := insertWFI(t, env, id, ticketID, workflowID)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(findingsJSON), &raw); err != nil {
		t.Fatalf("insertWFIWithFindings: unmarshal: %v", err)
	}
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	for k, v := range raw {
		if err := findingRepo.Upsert("workflow_instance", wfiID, k, v, repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
			t.Fatalf("insertWFIWithFindings: upsert key %q: %v", k, err)
		}
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
	}); err != nil {
		t.Fatalf("createSession %s: %v", id, err)
	}
}
