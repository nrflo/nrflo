package repo

import (
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// AgentStepCursorRepo handles agent_step_cursors CRUD.
type AgentStepCursorRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewAgentStepCursorRepo creates a new agent step cursor repository.
func NewAgentStepCursorRepo(database db.Querier, clk clock.Clock) *AgentStepCursorRepo {
	return &AgentStepCursorRepo{db: database, clock: clk}
}

// Insert creates the cursor row for (workflow_instance_id, node_id), seeded
// at revision 1 / current_index 0 / completed '[]'. A conflicting row is left
// untouched (ON CONFLICT DO NOTHING) so a relaunch/retry's snapshot call is a
// no-op rather than resetting in-progress step state.
func (r *AgentStepCursorRepo) Insert(c *model.AgentStepCursor) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	completed := c.Completed
	if completed == "" {
		completed = "[]"
	}
	rejections := c.Rejections
	if rejections == "" {
		rejections = "{}"
	}
	revision := c.Revision
	if revision == 0 {
		revision = 1
	}
	_, err := r.db.Exec(`
		INSERT INTO agent_step_cursors (workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_instance_id, node_id) DO NOTHING`,
		c.WorkflowInstanceID, c.NodeID, c.StepsSnapshot, revision, c.CurrentIndex, completed, rejections, now, now,
	)
	return err
}

// Get retrieves a cursor by (workflow_instance_id, node_id), returning
// sql.ErrNoRows when absent.
func (r *AgentStepCursorRepo) Get(instanceID, nodeID string) (*model.AgentStepCursor, error) {
	row := r.db.QueryRow(`
		SELECT workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at
		FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		instanceID, nodeID)
	return scanAgentStepCursor(row)
}

// RecordRejection increments the rejections[stepID] counter for
// (instanceID, nodeID) via json_set/json_extract (no CAS — an advisory
// counter, unlike Advance's guarded cursor mutation) and returns the new
// count via a read-back SELECT.
func (r *AgentStepCursorRepo) RecordRejection(instanceID, nodeID, stepID string) (int, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	path := "$." + stepID
	_, err := r.db.Exec(`
		UPDATE agent_step_cursors
		SET rejections = json_set(rejections, ?, COALESCE(json_extract(rejections, ?), 0) + 1), updated_at = ?
		WHERE workflow_instance_id = ? AND node_id = ?`,
		path, path, now, instanceID, nodeID,
	)
	if err != nil {
		return 0, err
	}
	var count int
	err = r.db.QueryRow(`SELECT COALESCE(json_extract(rejections, ?), 0) FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		path, instanceID, nodeID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Rejections decodes the rejections JSON map for (instanceID, nodeID).
func (r *AgentStepCursorRepo) Rejections(instanceID, nodeID string) (map[string]int, error) {
	c, err := r.Get(instanceID, nodeID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	if c.Rejections == "" {
		return counts, nil
	}
	if err := json.Unmarshal([]byte(c.Rejections), &counts); err != nil {
		return nil, err
	}
	return counts, nil
}

// Advance atomically moves the cursor forward by one step, guarded on the
// caller's expected (revision, current_index) — the same CAS idiom as
// PlanRepo.Approve. Returns false (no error) when the guard misses (stale
// revision or mismatched index), letting the caller treat that as an
// idempotent-replay case rather than a hard failure.
func (r *AgentStepCursorRepo) Advance(instanceID, nodeID string, expectedRevision, expectedIndex int, completedJSON string) (bool, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(`
		UPDATE agent_step_cursors
		SET current_index = current_index + 1, completed = ?, revision = revision + 1, updated_at = ?
		WHERE workflow_instance_id = ? AND node_id = ? AND revision = ? AND current_index = ?`,
		completedJSON, now, instanceID, nodeID, expectedRevision, expectedIndex,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

func scanAgentStepCursor(scanner interface{ Scan(...interface{}) error }) (*model.AgentStepCursor, error) {
	c := &model.AgentStepCursor{}
	var createdAt, updatedAt string
	err := scanner.Scan(&c.WorkflowInstanceID, &c.NodeID, &c.StepsSnapshot, &c.Revision, &c.CurrentIndex, &c.Completed, &c.Rejections, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return c, nil
}
