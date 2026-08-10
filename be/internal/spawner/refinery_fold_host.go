package spawner

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// refineryFoldHiddenWorkflow is the hidden global workflow the console fold's
// one-off child spawns under — same shape as delegateHiddenWorkflow
// (delegate_record.go). Underscore-prefixed, so it is excluded from all
// listings (service.IsHiddenWorkflowName).
const refineryFoldHiddenWorkflow = "_refinery_fold"

// ensureRefineryFoldHostInstance returns an active hidden `_refinery_fold`
// instance for a console session's cli fold, lazily seeding the workflow def
// row and minting the instance on first use. Unlike createDelegateHostInstance
// (one instance per delegate call), folds recur every debounce window for the
// life of the chat, so the instance is keyed to the session and reused.
func (s *Spawner) ensureRefineryFoldHostInstance(pool *db.Pool, projectID, callerSessionID string) (string, error) {
	var id string
	err := pool.QueryRow(
		`SELECT id FROM workflow_instances
		 WHERE project_id = ? AND workflow_id = ? AND origin_session_id = ? AND status = ?
		 ORDER BY created_at DESC LIMIT 1`,
		projectID, refineryFoldHiddenWorkflow, callerSessionID, string(model.WorkflowInstanceActive),
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("lookup host instance: %w", err)
	}

	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)
	// The reserved global project normally exists (seeded at startup by
	// service.EnsureGlobalDynamicWorkflow); ensure it here too so the
	// workflows FK never fires on a not-yet-seeded database.
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, '', ?, ?)`,
		service.GlobalProjectID, "Global Workflows", now, now,
	); err != nil {
		return "", fmt.Errorf("seed global project: %w", err)
	}
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 0, 1, '{}', ?, ?)`,
		refineryFoldHiddenWorkflow, service.GlobalProjectID,
		"Hidden host for console refinery cli folds (caller has no bound workflow instance)",
		now, now,
	); err != nil {
		return "", fmt.Errorf("seed hidden workflow: %w", err)
	}

	wi := &model.WorkflowInstance{
		ID:              uuid.New().String(),
		ProjectID:       projectID,
		DefProjectID:    service.GlobalProjectID,
		WorkflowID:      refineryFoldHiddenWorkflow,
		ScopeType:       "project",
		Status:          model.WorkflowInstanceActive,
		Origin:          model.RunOriginConsole,
		OriginSessionID: callerSessionID,
	}
	if err := repo.NewWorkflowInstanceRepo(pool, s.config.Clock).Create(wi); err != nil {
		return "", fmt.Errorf("create host instance: %w", err)
	}
	return wi.ID, nil
}
