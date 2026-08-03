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

// dispatchFieldCapBytes bounds input/output before persisting — a single
// misbehaving tool (e.g. a bulk read_document dump) must never blow up the
// audit table's row size.
const dispatchFieldCapBytes = 8192

// DispatchRepo handles CRUD for tool_dispatches
type DispatchRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewDispatchRepo creates a new DispatchRepo
func NewDispatchRepo(database db.Querier, clk clock.Clock) *DispatchRepo {
	return &DispatchRepo{db: database, clock: clk}
}

// Insert records a tool dispatch event, capping input/output to
// dispatchFieldCapBytes.
func (r *DispatchRepo) Insert(d *model.ToolDispatch) error {
	newID, err := dispatchIDGen.Generate()
	if err != nil {
		return fmt.Errorf("generate id: %w", err)
	}
	d.ID = newID

	now := r.clock.Now().UTC()
	d.CreatedAt = now

	input := capDispatchField(d.Input)
	output := capDispatchFieldPtr(d.Output)

	_, err = r.db.Exec(`
		INSERT INTO tool_dispatches
			(id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, source, session_kind, workflow_instance_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.SessionID, d.ToolName,
		input, output, d.Status, d.ErrorMsg, d.DurationMs,
		d.Source, d.SessionKind, d.WorkflowInstanceID,
		now.Format(time.RFC3339Nano),
	)
	return err
}

// capDispatchField truncates s to dispatchFieldCapBytes.
func capDispatchField(s string) string {
	if len(s) <= dispatchFieldCapBytes {
		return s
	}
	return s[:dispatchFieldCapBytes]
}

// capDispatchFieldPtr caps a nullable field in place, returning nil unchanged.
func capDispatchFieldPtr(s *string) *string {
	if s == nil || len(*s) <= dispatchFieldCapBytes {
		return s
	}
	capped := (*s)[:dispatchFieldCapBytes]
	return &capped
}
