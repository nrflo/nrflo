package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/repo"
)

// validateLayerConfigForWorkflow validates layer bounds for a workflow when adding/updating an agent definition.
// agentID is the agent being created or updated; newLayer is its proposed layer value.
func (s *AgentDefinitionService) validateLayerConfigForWorkflow(projectID, workflowID, agentID string, newLayer int) error {
	// Load all existing agent definitions for this workflow
	rows, err := s.pool.Query(`
		SELECT id, layer FROM agent_definitions
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		projectID, workflowID)
	if err != nil {
		return fmt.Errorf("failed to load agent definitions: %w", err)
	}
	defer rows.Close()

	var phases []PhaseDef
	found := false
	for rows.Next() {
		var id string
		var layer int
		if err := rows.Scan(&id, &layer); err != nil {
			return err
		}
		if strings.EqualFold(id, agentID) {
			// Replace with the new layer value
			phases = append(phases, PhaseDef{ID: id, Agent: id, Layer: newLayer})
			found = true
		} else {
			phases = append(phases, PhaseDef{ID: id, Agent: id, Layer: layer})
		}
	}
	// If not found, this is a new agent being created
	if !found {
		phases = append(phases, PhaseDef{ID: agentID, Agent: agentID, Layer: newLayer})
	}

	return validateLayerConfig(phases)
}

// validateTagInGroups checks that tag is present in the workflow's groups JSON string
func validateTagInGroups(tag, groupsStr string) error {
	var groups []string
	if groupsStr != "" {
		json.Unmarshal([]byte(groupsStr), &groups)
	}
	for _, g := range groups {
		if g == tag {
			return nil
		}
	}
	return fmt.Errorf("tag '%s' is not in workflow groups %v", tag, groups)
}

// validatePolicyNotViolatedByLayerChange returns an error if reducing agentCount in
// layer would violate any existing quorum policy. Called before delete or layer move.
func (s *AgentDefinitionService) validatePolicyNotViolatedByLayerChange(projectID, workflowID string, layer, remainingCount int) error {
	r := repo.NewWorkflowLayerPolicyRepo(s.pool, s.clock)
	rows, err := r.ListByWorkflow(projectID, workflowID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Layer != layer {
			continue
		}
		if err := ValidateLayerPolicy(row.PassPolicy, remainingCount); err != nil {
			return fmt.Errorf("layer %d has policy %q but would have only %d agent(s): %w", layer, row.PassPolicy, remainingCount, err)
		}
	}
	return nil
}

// DeleteAgentDef deletes an agent definition
func (s *AgentDefinitionService) DeleteAgentDef(projectID, workflowID, id string) error {
	// Find the agent's current layer
	var currentLayer int
	err := s.pool.QueryRow(
		"SELECT layer FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id).Scan(&currentLayer)
	if err != nil {
		return fmt.Errorf("agent definition not found: %s", id)
	}

	// Count remaining agents in this layer after deletion
	var remaining int
	s.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_definitions
		 WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)
		   AND layer = ? AND LOWER(id) != LOWER(?)`,
		projectID, workflowID, currentLayer, id).Scan(&remaining)

	if err := s.validatePolicyNotViolatedByLayerChange(projectID, workflowID, currentLayer, remaining); err != nil {
		return err
	}

	result, err := s.pool.Exec(
		"DELETE FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, workflowID, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("agent definition not found: %s", id)
	}
	return nil
}
