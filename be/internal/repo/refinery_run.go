package repo

import (
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// RefineryRunRepo handles the append-only refinery_runs fold-footprint log.
type RefineryRunRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewRefineryRunRepo creates a new RefineryRunRepo.
func NewRefineryRunRepo(database db.Querier, clk clock.Clock) *RefineryRunRepo {
	return &RefineryRunRepo{db: database, clock: clk}
}

// Insert writes one fold-attempt row, stamping folded_at from the repo's
// clock when the caller left it zero.
func (r *RefineryRunRepo) Insert(run *model.RefineryRun) error {
	if run.FoldedAt.IsZero() {
		run.FoldedAt = r.clock.Now().UTC()
	}
	_, err := r.db.Exec(
		`INSERT INTO refinery_runs (session_id, workflow_instance_id, node_id, project_id, provider, model, prompt_tokens, output_tokens, status, error, fold_count, folded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.SessionID, run.WorkflowInstanceID, run.NodeID, run.ProjectID, run.Provider, run.Model,
		run.PromptTokens, run.OutputTokens, run.Status, run.Error, run.FoldCount,
		run.FoldedAt.Format(time.RFC3339Nano),
	)
	return err
}

// ListRecent returns up to limit refinery_runs rows, newest first, folded_at
// >= since when since is non-zero.
func (r *RefineryRunRepo) ListRecent(limit int, since time.Time) ([]*model.RefineryRun, error) {
	query := `SELECT id, session_id, workflow_instance_id, node_id, project_id, provider, model, prompt_tokens, output_tokens, status, error, fold_count, folded_at
		 FROM refinery_runs`
	args := []interface{}{}
	if !since.IsZero() {
		query += ` WHERE folded_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	query += ` ORDER BY folded_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []*model.RefineryRun{}
	for rows.Next() {
		run := &model.RefineryRun{}
		var foldedAt string
		if err := rows.Scan(&run.ID, &run.SessionID, &run.WorkflowInstanceID, &run.NodeID, &run.ProjectID,
			&run.Provider, &run.Model, &run.PromptTokens, &run.OutputTokens, &run.Status, &run.Error,
			&run.FoldCount, &foldedAt); err != nil {
			return nil, err
		}
		run.FoldedAt, _ = time.Parse(time.RFC3339Nano, foldedAt)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
