package repo

import (
	"database/sql"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// InstanceNodeRepo handles workflow_instance_nodes (immutable, insert-only)
// and workflow_instance_layer_policies CRUD.
type InstanceNodeRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewInstanceNodeRepo creates a new InstanceNodeRepo.
func NewInstanceNodeRepo(database db.Querier, clk clock.Clock) *InstanceNodeRepo {
	return &InstanceNodeRepo{db: database, clock: clk}
}

// InsertNodes inserts materialized plan nodes within an existing transaction.
// Insert-only — nodes are immutable once materialized, so there is no
// update/delete counterpart.
func (r *InstanceNodeRepo) InsertNodes(tx *sql.Tx, instanceID string, nodes []model.InstanceNode) error {
	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	for _, n := range nodes {
		if _, err := tx.Exec(
			`INSERT INTO workflow_instance_nodes (instance_id, node_id, layer, agent_type, instructions, plan_revision, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			instanceID, n.NodeID, n.Layer, n.AgentType, n.Instructions, n.PlanRevision, now,
		); err != nil {
			return err
		}
	}
	return nil
}

// InsertLayerPolicies inserts materialized layer policies within an existing
// transaction.
func (r *InstanceNodeRepo) InsertLayerPolicies(tx *sql.Tx, instanceID string, policies map[int]string) error {
	for layer, policy := range policies {
		if _, err := tx.Exec(
			`INSERT INTO workflow_instance_layer_policies (instance_id, layer, pass_policy) VALUES (?, ?, ?)`,
			instanceID, layer, policy,
		); err != nil {
			return err
		}
	}
	return nil
}

// List returns every materialized node for an instance, ordered by layer
// then node_id.
func (r *InstanceNodeRepo) List(instanceID string) ([]model.InstanceNode, error) {
	rows, err := r.db.Query(
		`SELECT instance_id, node_id, layer, agent_type, instructions, plan_revision, created_at
		 FROM workflow_instance_nodes WHERE instance_id = ? ORDER BY layer ASC, node_id ASC`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.InstanceNode
	for rows.Next() {
		var n model.InstanceNode
		var createdAt string
		if err := rows.Scan(&n.InstanceID, &n.NodeID, &n.Layer, &n.AgentType, &n.Instructions, &n.PlanRevision, &createdAt); err != nil {
			return nil, err
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, n)
	}
	return out, nil
}

// ListLayerPolicies returns the materialized layer -> pass_policy map for an
// instance.
func (r *InstanceNodeRepo) ListLayerPolicies(instanceID string) (map[int]string, error) {
	rows, err := r.db.Query(
		`SELECT layer, pass_policy FROM workflow_instance_layer_policies WHERE instance_id = ?`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]string)
	for rows.Next() {
		var layer int
		var policy string
		if err := rows.Scan(&layer, &policy); err != nil {
			return nil, err
		}
		out[layer] = policy
	}
	return out, nil
}
