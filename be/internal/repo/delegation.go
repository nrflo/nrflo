package repo

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// DelegationRepo is the only DB access path for delegations rows (migration
// 000216) — the durable replacement for the `_delegation_<id>` finding.
type DelegationRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewDelegationRepo creates a new DelegationRepo.
func NewDelegationRepo(database db.Querier, clk clock.Clock) *DelegationRepo {
	return &DelegationRepo{db: database, clock: clk}
}

// Create inserts the seed row: worker_session_ids/spawn_errors are
// initialized to fanout-length arrays of "" so SetWorkerSlot's per-index
// json_set writes always have a slot to land on. Status starts 'running',
// fanout_done 0.
func (r *DelegationRepo) Create(d *model.Delegation) error {
	if d.CreatedAt.IsZero() {
		d.CreatedAt = r.clock.Now().UTC()
	}
	slots := make([]string, d.Fanout)
	blank, err := json.Marshal(slots)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`INSERT INTO delegations (id, caller_session_id, workflow_instance_id, project_id, tier, brief, fanout, worker_session_ids, spawn_errors, depth, fanout_done, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 'running', ?)`,
		d.ID, d.CallerSessionID, d.WorkflowInstanceID, d.ProjectID, d.Tier, d.Brief, d.Fanout,
		string(blank), string(blank), d.Depth, d.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

// Get retrieves a delegation row by id, returning sql.ErrNoRows when absent.
func (r *DelegationRepo) Get(id string) (*model.Delegation, error) {
	row := r.db.QueryRow(
		`SELECT id, caller_session_id, workflow_instance_id, project_id, tier, brief, fanout, worker_session_ids, spawn_errors, depth, fanout_done, status, created_at, completed_at, consumed_at, worktree_path, branch_name, base_commit, worktree_summary
		 FROM delegations WHERE id = ?`, id)
	return scanDelegation(row)
}

// SetWorktree persists the worktree path/branch/base commit chosen by
// prepareDelegateWorktree at fanout start, before any worker spawns.
func (r *DelegationRepo) SetWorktree(id, worktreePath, branchName, baseCommit string) error {
	_, err := r.db.Exec(
		`UPDATE delegations SET worktree_path = ?, branch_name = ?, base_commit = ? WHERE id = ?`,
		worktreePath, branchName, baseCommit, id,
	)
	return err
}

// SetWorktreeSummary persists finalizeDelegateWorktree's post-commit summary
// (changed files + diffstat) once the fanout's workers have all finished.
func (r *DelegationRepo) SetWorktreeSummary(id, summary string) error {
	_, err := r.db.Exec(`UPDATE delegations SET worktree_summary = ? WHERE id = ?`, summary, id)
	return err
}

// SetWorkerSlot writes one fanout worker's session id and spawn error into
// its slot via json_set, letting concurrent fanout workers each update their
// own index without a lost-update race (mirrors AgentStepCursorRepo's
// RecordRejection idiom).
func (r *DelegationRepo) SetWorkerSlot(id string, idx int, sessionID, spawnErr string) error {
	path := jsonIndexPath(idx)
	_, err := r.db.Exec(
		`UPDATE delegations SET worker_session_ids = json_set(worker_session_ids, ?, ?), spawn_errors = json_set(spawn_errors, ?, ?) WHERE id = ?`,
		path, sessionID, path, spawnErr, id,
	)
	return err
}

// MarkFanoutDone flips fanout_done once every worker slot has been written.
func (r *DelegationRepo) MarkFanoutDone(id string) error {
	_, err := r.db.Exec(`UPDATE delegations SET fanout_done = 1 WHERE id = ?`, id)
	return err
}

// MarkCompleted sets status/completed_at, guarded on completed_at IS NULL so
// the first observer of the terminal outcome (worker-fanout end or a
// GetDelegation poll) wins and later calls are no-ops. Completion is
// independent of consumption: an unconsumed delegation still flips out of
// 'running' the moment its workers finish.
func (r *DelegationRepo) MarkCompleted(id, status string) (bool, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE delegations SET status = ?, completed_at = ? WHERE id = ? AND completed_at IS NULL`,
		status, now, id,
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

// MarkConsumed sets consumed_at, guarded on consumed_at IS NULL so a
// delegation's result is handed to its caller exactly once; returns whether
// this call won the guard (false means it was already consumed).
func (r *DelegationRepo) MarkConsumed(id string) (bool, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE delegations SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL`,
		now, id,
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

// DepthForSession finds the delegation whose worker_session_ids contains
// sessionID and returns its depth — a session's own position in the
// delegate tree. Returns 0 when sessionID is not a worker of any delegation
// (a top-level, non-delegate caller).
func (r *DelegationRepo) DepthForSession(sessionID string) (int, error) {
	var depth int
	err := r.db.QueryRow(
		`SELECT d.depth FROM delegations d, json_each(d.worker_session_ids) je
		 WHERE je.value = ? LIMIT 1`, sessionID).Scan(&depth)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return depth, nil
}

// ListByCallerSession returns every delegation seeded by callerSessionID,
// oldest first — used by service.BuildSessionFlow to walk the delegate
// fanout edge of the flow graph.
func (r *DelegationRepo) ListByCallerSession(callerSessionID string) ([]*model.Delegation, error) {
	rows, err := r.db.Query(
		`SELECT id, caller_session_id, workflow_instance_id, project_id, tier, brief, fanout, worker_session_ids, spawn_errors, depth, fanout_done, status, created_at, completed_at, consumed_at, worktree_path, branch_name, base_commit, worktree_summary
		 FROM delegations WHERE caller_session_id = ? ORDER BY created_at ASC`, callerSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Delegation
	for rows.Next() {
		d, err := scanDelegation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func jsonIndexPath(idx int) string {
	return "$[" + strconv.Itoa(idx) + "]"
}

func scanDelegation(scanner interface{ Scan(...interface{}) error }) (*model.Delegation, error) {
	d := &model.Delegation{}
	var workerIDsJSON, spawnErrsJSON, createdAt string
	var completedAt, consumedAt sql.NullString
	err := scanner.Scan(&d.ID, &d.CallerSessionID, &d.WorkflowInstanceID, &d.ProjectID, &d.Tier, &d.Brief, &d.Fanout,
		&workerIDsJSON, &spawnErrsJSON, &d.Depth, &d.FanoutDone, &d.Status, &createdAt, &completedAt, &consumedAt,
		&d.WorktreePath, &d.BranchName, &d.BaseCommit, &d.Summary)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(workerIDsJSON), &d.WorkerSessionIDs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(spawnErrsJSON), &d.SpawnErrors); err != nil {
		return nil, err
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		d.CompletedAt = &t
	}
	if consumedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, consumedAt.String)
		d.ConsumedAt = &t
	}
	return d, nil
}
