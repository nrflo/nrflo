package repo

import "be/internal/model"

// ListByInstance returns every node cursor for one workflow instance,
// ordered by node_id, for the stepwise cursor read model.
func (r *AgentStepCursorRepo) ListByInstance(instanceID string) ([]*model.AgentStepCursor, error) {
	rows, err := r.db.Query(`
		SELECT workflow_instance_id, node_id, steps_snapshot, revision, current_index, completed, rejections, created_at, updated_at
		FROM agent_step_cursors WHERE workflow_instance_id = ? ORDER BY node_id`,
		instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursors []*model.AgentStepCursor
	for rows.Next() {
		c, err := scanAgentStepCursor(rows)
		if err != nil {
			return nil, err
		}
		cursors = append(cursors, c)
	}
	return cursors, rows.Err()
}
