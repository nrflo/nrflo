package repo

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// WorkflowChainRunRepo handles workflow_chain_runs and workflow_chain_run_steps lifecycle.
type WorkflowChainRunRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewWorkflowChainRunRepo creates a new workflow chain run repository.
func NewWorkflowChainRunRepo(database db.Querier, clk clock.Clock) *WorkflowChainRunRepo {
	return &WorkflowChainRunRepo{db: database, clock: clk}
}

const wfChainRunCols = `id, project_id, chain_id, status, initial_instructions, triggered_by, current_position, started_at, completed_at, created_at, updated_at`

func scanWorkflowChainRun(row interface{ Scan(...interface{}) error }) (*model.WorkflowChainRun, error) {
	r := &model.WorkflowChainRun{}
	var startedAt, completedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&r.ID, &r.ProjectID, &r.ChainID, &r.Status, &r.InitialInstructions, &r.TriggeredBy, &r.CurrentPosition, &startedAt, &completedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		r.StartedAt = &t
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		r.CompletedAt = &t
	}
	return r, nil
}

// CreateRun inserts a new workflow chain run with status=pending.
func (r *WorkflowChainRunRepo) CreateRun(run *model.WorkflowChainRun) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	run.UpdatedAt = run.CreatedAt
	_, err := r.db.Exec(
		`INSERT INTO workflow_chain_runs (`+wfChainRunCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ProjectID, run.ChainID, run.Status, run.InitialInstructions, run.TriggeredBy,
		run.CurrentPosition, nil, nil, now, now,
	)
	return err
}

// GetRun retrieves a workflow chain run by ID.
func (r *WorkflowChainRunRepo) GetRun(runID string) (*model.WorkflowChainRun, error) {
	row := r.db.QueryRow(
		`SELECT `+wfChainRunCols+` FROM workflow_chain_runs WHERE id = ?`, runID)
	run, err := scanWorkflowChainRun(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow chain run not found: %s", runID)
	}
	return run, err
}

// ListRuns returns workflow chain runs for a project, optionally filtered by status.
func (r *WorkflowChainRunRepo) ListRuns(projectID, status string) ([]*model.WorkflowChainRun, error) {
	query := `SELECT ` + wfChainRunCols + ` FROM workflow_chain_runs WHERE LOWER(project_id) = LOWER(?)`
	args := []interface{}{projectID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*model.WorkflowChainRun
	for rows.Next() {
		run, err := scanWorkflowChainRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// UpdateRunStatus transitions a run to a new status, setting timestamps as appropriate.
func (r *WorkflowChainRunRepo) UpdateRunStatus(runID, status string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	var result sql.Result
	var err error

	switch status {
	case "running":
		result, err = r.db.Exec(
			`UPDATE workflow_chain_runs SET status=?, started_at=COALESCE(started_at, ?), updated_at=? WHERE id=?`,
			status, now, now, runID)
	case "completed", "failed", "canceled":
		result, err = r.db.Exec(
			`UPDATE workflow_chain_runs SET status=?, completed_at=?, updated_at=? WHERE id=?`,
			status, now, now, runID)
	default:
		result, err = r.db.Exec(
			`UPDATE workflow_chain_runs SET status=?, updated_at=? WHERE id=?`,
			status, now, runID)
	}
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow chain run not found: %s", runID)
	}
	return nil
}

// SetCurrentPosition updates current_position of a run.
func (r *WorkflowChainRunRepo) SetCurrentPosition(runID string, position int) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_chain_runs SET current_position=?, updated_at=? WHERE id=?`,
		position, now, runID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow chain run not found: %s", runID)
	}
	return nil
}

// MaterializeRunSteps inserts run steps derived from chain step definitions in a single transaction.
// Returns the created run step rows.
func (r *WorkflowChainRunRepo) MaterializeRunSteps(runID string, steps []*model.WorkflowChainStep) ([]*model.WorkflowChainRunStep, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	created := make([]*model.WorkflowChainRunStep, 0, len(steps))
	for _, s := range steps {
		rs := &model.WorkflowChainRunStep{
			ID:           uuid.New().String(),
			ChainRunID:   runID,
			Position:     s.Position,
			WorkflowName: s.WorkflowName,
			ScopeType:    s.ScopeType,
			Status:       "pending",
		}
		rs.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
		rs.UpdatedAt = rs.CreatedAt

		_, err := tx.Exec(
			`INSERT INTO workflow_chain_run_steps (id, chain_run_id, position, workflow_name, scope_type, workflow_instance_id, ticket_id, instructions_used, status, started_at, ended_at, created_at, updated_at)
             VALUES (?, ?, ?, ?, ?, NULL, NULL, '', 'pending', NULL, NULL, ?, ?)`,
			rs.ID, rs.ChainRunID, rs.Position, rs.WorkflowName, rs.ScopeType, now, now,
		)
		if err != nil {
			return nil, err
		}
		created = append(created, rs)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// GetNextPendingStep returns the lowest-position pending step for a run, or nil if none.
func (r *WorkflowChainRunRepo) GetNextPendingStep(runID string) (*model.WorkflowChainRunStep, error) {
	row := r.db.QueryRow(
		`SELECT id, chain_run_id, position, workflow_name, scope_type, workflow_instance_id, ticket_id, instructions_used, status, started_at, ended_at, created_at, updated_at
         FROM workflow_chain_run_steps
         WHERE chain_run_id = ? AND status = 'pending'
         ORDER BY position ASC LIMIT 1`, runID)
	rs, err := scanWorkflowChainRunStep(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rs, err
}

// UpdateRunStepStatus transitions a run step to a new status, setting timestamps as appropriate.
func (r *WorkflowChainRunRepo) UpdateRunStepStatus(runStepID, status string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	var result sql.Result
	var err error

	switch status {
	case "running":
		result, err = r.db.Exec(
			`UPDATE workflow_chain_run_steps SET status=?, started_at=?, updated_at=? WHERE id=?`,
			status, now, now, runStepID)
	case "completed", "failed", "skipped", "canceled":
		result, err = r.db.Exec(
			`UPDATE workflow_chain_run_steps SET status=?, ended_at=?, updated_at=? WHERE id=?`,
			status, now, now, runStepID)
	default:
		result, err = r.db.Exec(
			`UPDATE workflow_chain_run_steps SET status=?, updated_at=? WHERE id=?`,
			status, now, runStepID)
	}
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow chain run step not found: %s", runStepID)
	}
	return nil
}

// SetRunStepInstance assigns workflow_instance_id, ticket_id, and instructions_used to a run step.
func (r *WorkflowChainRunRepo) SetRunStepInstance(runStepID, instanceID, ticketID, instructionsUsed string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_chain_run_steps SET workflow_instance_id=?, ticket_id=?, instructions_used=?, updated_at=? WHERE id=?`,
		instanceID, ticketID, instructionsUsed, now, runStepID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow chain run step not found: %s", runStepID)
	}
	return nil
}

// GetActiveRuns returns all workflow chain runs with status=running across all projects.
func (r *WorkflowChainRunRepo) GetActiveRuns() ([]*model.WorkflowChainRun, error) {
	rows, err := r.db.Query(
		`SELECT `+wfChainRunCols+` FROM workflow_chain_runs WHERE status = 'running' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*model.WorkflowChainRun
	for rows.Next() {
		run, err := scanWorkflowChainRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func scanWorkflowChainRunStep(row interface{ Scan(...interface{}) error }) (*model.WorkflowChainRunStep, error) {
	rs := &model.WorkflowChainRunStep{}
	var createdAt, updatedAt string
	if err := row.Scan(
		&rs.ID, &rs.ChainRunID, &rs.Position, &rs.WorkflowName, &rs.ScopeType,
		&rs.WorkflowInstanceID, &rs.TicketID, &rs.InstructionsUsed, &rs.Status,
		&rs.StartedAt, &rs.EndedAt, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	rs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	rs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return rs, nil
}
