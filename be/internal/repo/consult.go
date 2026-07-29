package repo

import (
	"database/sql"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// ConsultRepo is the only DB access path for consults rows (migration
// 000217) — the durable caller-linkage record consult children lack on
// agent_sessions.
type ConsultRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewConsultRepo creates a new ConsultRepo.
func NewConsultRepo(database db.Querier, clk clock.Clock) *ConsultRepo {
	return &ConsultRepo{db: database, clock: clk}
}

// Create inserts the seed row before the consultant is spawned. Status starts
// 'running'.
func (r *ConsultRepo) Create(c *model.Consult) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = r.clock.Now().UTC()
	}
	_, err := r.db.Exec(
		`INSERT INTO consults (id, caller_session_id, workflow_instance_id, project_id, consultant_id, question, child_session_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', 'running', ?)`,
		c.ID, c.CallerSessionID, c.WorkflowInstanceID, c.ProjectID, c.ConsultantID, c.Question,
		c.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

// SetChildSession records the spawned consultant's session id once known.
func (r *ConsultRepo) SetChildSession(id, sessionID string) error {
	_, err := r.db.Exec(`UPDATE consults SET child_session_id = ? WHERE id = ?`, sessionID, id)
	return err
}

// MarkTerminal sets status/error/completed_at, guarded on status='running' so
// a consult is finalized exactly once; returns whether this call won the
// guard (false means it was already terminal).
func (r *ConsultRepo) MarkTerminal(id, status, errMsg string) (bool, error) {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.Exec(
		`UPDATE consults SET status = ?, error = ?, completed_at = ? WHERE id = ? AND status = 'running'`,
		status, errMsg, now, id,
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

// ListByInstance returns every consult row seeded under a workflow instance,
// oldest first — used by trace sub-lane grouping.
func (r *ConsultRepo) ListByInstance(wfiID string) ([]*model.Consult, error) {
	rows, err := r.db.Query(
		`SELECT id, caller_session_id, workflow_instance_id, project_id, consultant_id, question, child_session_id, status, error, created_at, completed_at
		 FROM consults WHERE workflow_instance_id = ? ORDER BY created_at ASC`, wfiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Consult
	for rows.Next() {
		c, err := scanConsult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanConsult(scanner interface{ Scan(...interface{}) error }) (*model.Consult, error) {
	c := &model.Consult{}
	var createdAt string
	var completedAt sql.NullString
	err := scanner.Scan(&c.ID, &c.CallerSessionID, &c.WorkflowInstanceID, &c.ProjectID, &c.ConsultantID, &c.Question,
		&c.ChildSessionID, &c.Status, &c.Error, &createdAt, &completedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		c.CompletedAt = &t
	}
	return c, nil
}
