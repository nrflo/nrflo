package repo

import (
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
)

var dispatchIDGen = id.New("disp")

// DispatchRepo handles CRUD for tool_dispatches
type DispatchRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewDispatchRepo creates a new DispatchRepo
func NewDispatchRepo(database db.Querier, clk clock.Clock) *DispatchRepo {
	return &DispatchRepo{db: database, clock: clk}
}

// Insert records a tool dispatch event
func (r *DispatchRepo) Insert(d *model.ToolDispatch) error {
	newID, err := dispatchIDGen.Generate()
	if err != nil {
		return fmt.Errorf("generate id: %w", err)
	}
	d.ID = newID

	now := r.clock.Now().UTC()
	d.CreatedAt = now

	_, err = r.db.Exec(`
		INSERT INTO tool_dispatches
			(id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.SessionID, d.ToolName,
		d.Input, d.Output, d.Status, d.ErrorMsg, d.DurationMs,
		now.Format(time.RFC3339Nano),
	)
	return err
}
