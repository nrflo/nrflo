package service

import (
	"encoding/json"
	"fmt"
)

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
