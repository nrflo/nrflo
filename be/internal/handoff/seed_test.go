package handoff

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

func testClock() clock.Clock { return clock.Real() }

// seedProjectAndWorkflow inserts the minimal projects/workflows/
// workflow_instances chain a session FK needs. rootPath is written to
// projects.root_path (used as the repoRoot fallback when worktreePath is
// empty).
func seedProjectAndWorkflow(t *testing.T, pool *db.Pool, projectID, wfiID, rootPath, worktreePath string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, projectID, rootPath, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, 'wf', '', 'project', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, worktree_path, created_at, updated_at)
		 VALUES (?, ?, '', 'wf', 'active', 'project', ?, ?, ?)`,
		wfiID, projectID, worktreePath, now, now,
	); err != nil {
		t.Fatalf("seed workflow_instance: %v", err)
	}
}

// seedSession inserts an agent_sessions row with the given ancestor chain
// link and task-anchor prompt.
func seedSession(t *testing.T, pool *db.Pool, sessionID, projectID, wfiID, nodeID, prompt, ancestorSessionID string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, prompt, ancestor_session_id, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'ph', ?, 'ag', 'completed', ?, ?, ?, ?)`,
		sessionID, projectID, wfiID, nodeID, prompt, nullIfEmpty(ancestorSessionID), now, now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// seedMessage is one row to write via seedMessages, oldest-first.
type seedMessage struct {
	category string
	content  string
	payload  string
}

// seedMessages inserts agent_messages rows in order via the real repo (keeps
// seq assignment identical to production writes).
func seedMessages(t *testing.T, pool *db.Pool, sessionID string, msgs []seedMessage) {
	t.Helper()
	msgRepo := repo.NewAgentMessageRepo(pool, testClock())
	entries := make([]repo.MessageEntry, len(msgs))
	for i, m := range msgs {
		entries[i] = repo.MessageEntry{Content: m.content, Category: m.category, Payload: m.payload}
	}
	if err := msgRepo.InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("seed messages: %v", err)
	}
}

// seedFinding writes a session-scope finding through the real repo (keeps
// history-tracking + denorm columns consistent with production writes).
func seedFinding(t *testing.T, pool *db.Pool, wfiID, sessionID, agentType, key string, value interface{}) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal finding %q: %v", key, err)
	}
	findingRepo := repo.NewFindingRepo(pool, testClock())
	denorm := repo.Denorm{WorkflowInstanceID: wfiID, AgentType: agentType}
	actor := repo.Actor{Source: "agent"}
	if err := findingRepo.Upsert("session", sessionID, key, b, denorm, actor); err != nil {
		t.Fatalf("Upsert finding %q: %v", key, err)
	}
}

// seedTicket inserts a ticket row via the real repo.
func seedTicket(t *testing.T, pool *db.Pool, projectID, ticketID, title string) {
	t.Helper()
	ticketRepo := repo.NewTicketRepo(pool, testClock())
	tk := &model.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     title,
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(tk); err != nil {
		t.Fatalf("create ticket %s: %v", ticketID, err)
	}
}
