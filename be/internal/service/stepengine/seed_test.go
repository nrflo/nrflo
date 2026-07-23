package stepengine

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// seedProjectAndWorkflow inserts the minimal projects/workflows/
// workflow_instances chain agent_step_cursors' FK needs. rootPath is written
// to projects.root_path (used as the worktree-root fallback when
// worktreePath is empty).
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

// seedSession inserts an agent_sessions row attributed to nodeID — required
// before FindingRepo.GetByNode reports the node as known.
func seedSession(t *testing.T, pool *db.Pool, sessionID, projectID, wfiID, nodeID string) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, prompt, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'ph', ?, 'ag', 'completed', 'p', ?, ?)`,
		sessionID, projectID, wfiID, nodeID, now, now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

// seedFinding writes a session-scope finding through the real repo (keeps
// history-tracking + denorm columns consistent with production writes).
func seedFinding(t *testing.T, pool *db.Pool, wfiID, sessionID, key string, value interface{}) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal finding %q: %v", key, err)
	}
	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	denorm := repo.Denorm{WorkflowInstanceID: wfiID}
	actor := repo.Actor{Source: "agent"}
	if err := findingRepo.Upsert("session", sessionID, key, b, denorm, actor); err != nil {
		t.Fatalf("Upsert finding %q: %v", key, err)
	}
}
