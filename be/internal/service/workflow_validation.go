package service

import (
	"fmt"
	"strings"
)

// validateLayerConfig validates layer-based phase configuration rules:
// - layer must be >= 0
func validateLayerConfig(phases []PhaseDef) error {
	// Validate layer values
	for _, p := range phases {
		if p.Layer < 0 {
			return fmt.Errorf("agent '%s': layer must be >= 0, got %d", p.Agent, p.Layer)
		}
	}

	return nil
}

// ticketTemplateVars are template variables that require ticket context
var ticketTemplateVars = []string{"${TICKET_ID}", "${TICKET_TITLE}", "${TICKET_DESCRIPTION}"}

// ValidateProjectScope checks that agent prompts don't use ticket-specific template variables
// when scope_type is "project". Loads agent definitions from DB to check their prompts.
func ValidateProjectScope(pool interface {
	QueryRow(string, ...interface{}) interface{ Scan(...interface{}) error }
}, projectID, workflowID string, phases []PhaseDef) error {
	for _, phase := range phases {
		var prompt string
		err := pool.QueryRow(`
			SELECT prompt FROM agent_definitions
			WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			projectID, workflowID, phase.Agent).Scan(&prompt)
		if err != nil {
			continue // Agent definition may not exist yet
		}
		for _, v := range ticketTemplateVars {
			if strings.Contains(prompt, v) {
				return fmt.Errorf("agent '%s' uses %s in its prompt, which is not available for project-scoped workflows", phase.Agent, v)
			}
		}
	}
	return nil
}

// ValidateScopeType validates that scope_type is a valid value
func ValidateScopeType(scopeType string) error {
	if scopeType != "" && scopeType != "ticket" && scopeType != "project" {
		return fmt.Errorf("scope_type must be 'ticket' or 'project', got '%s'", scopeType)
	}
	return nil
}

// ValidateGroups validates that groups is an array of non-empty, unique strings
func ValidateGroups(groups []string) error {
	seen := make(map[string]bool, len(groups))
	for _, g := range groups {
		if strings.TrimSpace(g) == "" {
			return fmt.Errorf("groups must not contain empty strings")
		}
		if seen[g] {
			return fmt.Errorf("duplicate group: '%s'", g)
		}
		seen[g] = true
	}
	return nil
}
