package service

// revalidateLayerChange validates a def's proposed new layer (layer >= 0,
// per-workflow layer policy) and, when the layer actually moves, ensures the
// old layer's pass policy still holds for whatever remains in it.
func (s *AgentDefinitionService) revalidateLayerChange(projectID, workflowID, id string, newLayer int) error {
	if err := s.validateLayerConfigForWorkflow(projectID, workflowID, id, newLayer); err != nil {
		return err
	}

	var oldLayer int
	if scanErr := s.pool.QueryRow(
		"SELECT layer FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&oldLayer); scanErr != nil || oldLayer == newLayer {
		return nil
	}

	var remaining int
	s.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_definitions
		 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		   AND layer = ? AND LOWER(id) != LOWER(?) AND consultant = 0 AND node_role = 'static'`,
		projectID, workflowID, oldLayer, id).Scan(&remaining)
	return s.validatePolicyNotViolatedByLayerChange(projectID, workflowID, oldLayer, remaining)
}
