package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// WorkflowChainRepo handles workflow_chains CRUD.
type WorkflowChainRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewWorkflowChainRepo creates a new workflow chain repository.
func NewWorkflowChainRepo(database db.Querier, clk clock.Clock) *WorkflowChainRepo {
	return &WorkflowChainRepo{db: database, clock: clk}
}

const wfChainCols = `id, project_id, name, description, created_at, updated_at`

func scanWorkflowChain(row interface{ Scan(...interface{}) error }) (*model.WorkflowChain, error) {
	c := &model.WorkflowChain{}
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Description, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return c, nil
}

// CreateChain inserts a new workflow chain.
func (r *WorkflowChainRepo) CreateChain(c *model.WorkflowChain) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	c.UpdatedAt = c.CreatedAt
	_, err := r.db.Exec(
		`INSERT INTO workflow_chains (`+wfChainCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
		strings.ToLower(c.ID),
		strings.ToLower(c.ProjectID),
		c.Name,
		c.Description,
		now,
		now,
	)
	return err
}

// GetChain retrieves a workflow chain by project ID and chain ID.
func (r *WorkflowChainRepo) GetChain(projectID, id string) (*model.WorkflowChain, error) {
	row := r.db.QueryRow(
		`SELECT `+wfChainCols+` FROM workflow_chains WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, id)
	c, err := scanWorkflowChain(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow chain not found: %s", id)
	}
	return c, err
}

// ListChains returns all workflow chains for a project ordered by created_at ASC.
func (r *WorkflowChainRepo) ListChains(projectID string) ([]*model.WorkflowChain, error) {
	rows, err := r.db.Query(
		`SELECT `+wfChainCols+` FROM workflow_chains WHERE LOWER(project_id) = LOWER(?) ORDER BY created_at ASC`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chains []*model.WorkflowChain
	for rows.Next() {
		c, err := scanWorkflowChain(rows)
		if err != nil {
			return nil, err
		}
		chains = append(chains, c)
	}
	return chains, rows.Err()
}

// UpdateChain updates name and description of a workflow chain.
func (r *WorkflowChainRepo) UpdateChain(c *model.WorkflowChain) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE workflow_chains SET name=?, description=?, updated_at=? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		c.Name, c.Description, now, c.ProjectID, c.ID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow chain not found: %s", c.ID)
	}
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return nil
}

// DeleteChain deletes a workflow chain and cascades to its steps.
func (r *WorkflowChainRepo) DeleteChain(projectID, id string) error {
	result, err := r.db.Exec(
		`DELETE FROM workflow_chains WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow chain not found: %s", id)
	}
	return nil
}

// WorkflowChainStepRepo handles workflow_chain_steps operations.
type WorkflowChainStepRepo struct {
	clock clock.Clock
	db    db.Querier
}

// NewWorkflowChainStepRepo creates a new workflow chain step repository.
func NewWorkflowChainStepRepo(database db.Querier, clk clock.Clock) *WorkflowChainStepRepo {
	return &WorkflowChainStepRepo{db: database, clock: clk}
}

const wfChainStepCols = `id, project_id, chain_id, position, workflow_name, scope_type, base_instructions, require_ticket_handoff, created_at, updated_at`

func scanWorkflowChainStep(row interface{ Scan(...interface{}) error }) (*model.WorkflowChainStep, error) {
	s := &model.WorkflowChainStep{}
	var requireHandoff int
	var createdAt, updatedAt string
	if err := row.Scan(&s.ID, &s.ProjectID, &s.ChainID, &s.Position, &s.WorkflowName, &s.ScopeType, &s.BaseInstructions, &requireHandoff, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.RequireTicketHandoff = requireHandoff != 0
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return s, nil
}

// UpsertStep inserts or updates a workflow chain step.
func (r *WorkflowChainStepRepo) UpsertStep(s *model.WorkflowChainStep) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	if s.CreatedAt.IsZero() {
		s.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	}
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, now)
	_, err := r.db.Exec(
		`INSERT INTO workflow_chain_steps (`+wfChainStepCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(id) DO UPDATE SET
            position=excluded.position,
            workflow_name=excluded.workflow_name,
            scope_type=excluded.scope_type,
            base_instructions=excluded.base_instructions,
            require_ticket_handoff=excluded.require_ticket_handoff,
            updated_at=excluded.updated_at`,
		s.ID, s.ProjectID, s.ChainID, s.Position, s.WorkflowName, s.ScopeType,
		s.BaseInstructions, boolToInt(s.RequireTicketHandoff), now, now,
	)
	return err
}

// DeleteStep deletes a workflow chain step by ID.
func (r *WorkflowChainStepRepo) DeleteStep(id string) error {
	result, err := r.db.Exec(`DELETE FROM workflow_chain_steps WHERE LOWER(id) = LOWER(?)`, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow chain step not found: %s", id)
	}
	return nil
}

// ListSteps returns all steps for a chain ordered by position ASC.
func (r *WorkflowChainStepRepo) ListSteps(chainID string) ([]*model.WorkflowChainStep, error) {
	rows, err := r.db.Query(
		`SELECT `+wfChainStepCols+` FROM workflow_chain_steps WHERE LOWER(chain_id) = LOWER(?) ORDER BY position ASC`,
		chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []*model.WorkflowChainStep
	for rows.Next() {
		s, err := scanWorkflowChainStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// BulkReorder assigns each step in stepIDsInOrder its index as the new position.
func (r *WorkflowChainStepRepo) BulkReorder(chainID string, stepIDsInOrder []string) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for i, stepID := range stepIDsInOrder {
		_, err := tx.Exec(
			`UPDATE workflow_chain_steps SET position=?, updated_at=? WHERE LOWER(id) = LOWER(?) AND LOWER(chain_id) = LOWER(?)`,
			i, now, stepID, chainID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
