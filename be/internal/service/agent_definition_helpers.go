package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// validateLayerConfigForWorkflow validates fan-in rules for a workflow when adding/updating an agent definition.
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

// DeleteAgentDef deletes an agent definition
func (s *AgentDefinitionService) DeleteAgentDef(projectID, workflowID, id string) error {
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
