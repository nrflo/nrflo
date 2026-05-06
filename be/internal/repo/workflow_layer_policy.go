package repo

import (
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// WorkflowLayerPolicyRepo handles workflow layer policy CRUD operations.
type WorkflowLayerPolicyRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewWorkflowLayerPolicyRepo creates a new workflow layer policy repository.
func NewWorkflowLayerPolicyRepo(database db.Querier, clk clock.Clock) *WorkflowLayerPolicyRepo {
	return &WorkflowLayerPolicyRepo{db: database, clock: clk}
}

// Upsert inserts or updates a layer policy row.
func (r *WorkflowLayerPolicyRepo) Upsert(policy *model.WorkflowLayerPolicy) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(`
		INSERT INTO workflow_layer_policies (project_id, workflow_id, layer, pass_policy, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, workflow_id, layer) DO UPDATE SET
			pass_policy = excluded.pass_policy,
			updated_at  = excluded.updated_at`,
		strings.ToLower(policy.ProjectID),
		strings.ToLower(policy.WorkflowID),
		policy.Layer,
		policy.PassPolicy,
		now,
		now,
	)
	return err
}

// Delete removes a layer policy row. Returns nil if the row did not exist.
func (r *WorkflowLayerPolicyRepo) Delete(projectID, workflowID string, layer int) error {
	_, err := r.db.Exec(
		`DELETE FROM workflow_layer_policies
		 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND layer = ?`,
		projectID, workflowID, layer,
	)
	return err
}

// ListByWorkflow returns all layer policies for a workflow ordered by layer ASC.
func (r *WorkflowLayerPolicyRepo) ListByWorkflow(projectID, workflowID string) ([]*model.WorkflowLayerPolicy, error) {
	rows, err := r.db.Query(`
		SELECT project_id, workflow_id, layer, pass_policy, created_at, updated_at
		FROM workflow_layer_policies
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		ORDER BY layer ASC`,
		projectID, workflowID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*model.WorkflowLayerPolicy
	for rows.Next() {
		p := &model.WorkflowLayerPolicy{}
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ProjectID, &p.WorkflowID, &p.Layer, &p.PassPolicy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		policies = append(policies, p)
	}
	return policies, nil
}
