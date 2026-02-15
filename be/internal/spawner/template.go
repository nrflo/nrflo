package spawner

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"be/internal/model"
	"be/internal/repo"
)

// findingsPattern matches #{FINDINGS:agent_type} or #{FINDINGS:agent_type:key(s)}
var findingsPattern = regexp.MustCompile(`#\{FINDINGS:([^:}]+)(?::([^}]*))?\}`)

// Preview generates the prompt without spawning
func (s *Spawner) Preview(agentType, ticketID, projectID, workflowName string) (string, error) {
	model := "opus"
	cliName := s.config.DefaultCLI
	if cliName == "" {
		cliName = "claude"
	}
	if agentCfg, ok := s.config.Agents[agentType]; ok {
		if agentCfg.Model != "" {
			model = agentCfg.Model
		}
	}
	modelID := fmt.Sprintf("%s:%s", cliName, model)
	return s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID, "", "")
}

// loadAgentDefinition loads the full agent definition from the DB.
// Returns nil if not found (caller should fall back to defaults).
func (s *Spawner) loadAgentDefinition(agentType, projectID, workflowName string) *model.AgentDefinition {
	pool := s.pool()
	if pool == nil {
		return nil
	}

	adRepo := repo.NewAgentDefinitionRepo(pool, s.config.Clock)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err != nil {
		return nil
	}
	return def
}

// loadPromptContent loads the prompt content for an agent from the DB.
func (s *Spawner) loadPromptContent(agentType, projectID, workflowName string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("failed to get database pool")
	}

	adRepo := repo.NewAgentDefinitionRepo(pool, s.config.Clock)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err != nil {
		return "", fmt.Errorf("agent definition not found: %s (workflow=%s). Create via 'nrworkflow agent def create %s -w %s --prompt-file=<path>'", agentType, workflowName, agentType, workflowName)
	}
	if def.Prompt == "" {
		return "", fmt.Errorf("agent definition '%s' has empty prompt", agentType)
	}
	return def.Prompt, nil
}

// fetchTicketInfo returns the ticket title and description for template expansion.
func (s *Spawner) fetchTicketInfo(projectID, ticketID string) (title, description string) {
	pool := s.pool()
	if pool == nil {
		fmt.Fprintf(os.Stderr, "Warning: no database pool for ticket info\n")
		return ticketID, "_No description available_"
	}

	ticketRepo := repo.NewTicketRepo(pool, s.config.Clock)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch ticket %s: %v\n", ticketID, err)
		return ticketID, "_No description available_"
	}
	title = ticket.Title
	if ticket.Description.Valid && ticket.Description.String != "" {
		description = ticket.Description.String
	} else {
		description = "_No description available_"
	}
	return title, description
}

// fetchUserInstructions returns user_instructions from the workflow instance findings.
// Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchUserInstructions(projectID, ticketID, workflowName, wfiID string) string {
	pool := s.pool()
	if pool == nil {
		fmt.Fprintf(os.Stderr, "Warning: no database pool for user instructions\n")
		return "_No user instructions provided_"
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	var wi *model.WorkflowInstance
	var err error
	if wfiID != "" {
		wi, err = wfiRepo.Get(wfiID)
	} else if ticketID != "" {
		wi, err = wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	} else {
		// Fallback: get most recent active project instance
		var instances []*model.WorkflowInstance
		instances, err = wfiRepo.ListActiveByProjectAndWorkflow(projectID, workflowName)
		if err == nil && len(instances) > 0 {
			wi = instances[len(instances)-1]
		}
	}
	if err != nil || wi == nil {
		return "_No user instructions provided_"
	}
	findings := wi.GetFindings()
	if instructions, ok := findings["user_instructions"]; ok {
		if str, ok := instructions.(string); ok && str != "" {
			return str
		}
	}
	return "_No user instructions provided_"
}

// fetchCallbackInstructions returns callback instructions from the workflow instance findings.
// Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchCallbackInstructions(projectID, ticketID, workflowName, wfiID string) string {
	pool := s.pool()
	if pool == nil {
		fmt.Fprintf(os.Stderr, "Warning: no database pool for callback instructions\n")
		return "_No callback instructions_"
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	var wi *model.WorkflowInstance
	var err error
	if wfiID != "" {
		wi, err = wfiRepo.Get(wfiID)
	} else if ticketID != "" {
		wi, err = wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	} else {
		var instances []*model.WorkflowInstance
		instances, err = wfiRepo.ListActiveByProjectAndWorkflow(projectID, workflowName)
		if err == nil && len(instances) > 0 {
			wi = instances[len(instances)-1]
		}
	}
	if err != nil || wi == nil {
		return "_No callback instructions_"
	}
	findings := wi.GetFindings()
	callbackRaw, ok := findings["_callback"]
	if !ok {
		return "_No callback instructions_"
	}
	callbackMap, ok := callbackRaw.(map[string]interface{})
	if !ok {
		return "_No callback instructions_"
	}
	instructions, _ := callbackMap["instructions"].(string)
	if instructions == "" {
		return "_No callback instructions_"
	}
	result := "## Callback Instructions\n\nThis agent is being re-run due to a callback from a later stage.\n\n"
	if fromAgent, ok := callbackMap["from_agent"].(string); ok && fromAgent != "" {
		result += "Callback triggered by: " + fromAgent + "\n\n"
	}
	result += instructions
	return result
}

// loadTemplate loads and expands an agent template from DB.
// wfiID is optional — when set, used for instance-specific lookups (user instructions, callbacks).
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, phase, wfiID string) (string, error) {
	promptContent, err := s.loadPromptContent(agentType, projectID, workflowName)
	if err != nil {
		return "", err
	}

	template := promptContent

	_, model := parseModelID(modelID)
	if model == "" {
		model = "sonnet"
	}

	// Expand variables
	template = strings.ReplaceAll(template, "${AGENT}", agentType)
	template = strings.ReplaceAll(template, "${TICKET_ID}", ticketID)
	template = strings.ReplaceAll(template, "${PROJECT_ID}", projectID)
	template = strings.ReplaceAll(template, "${WORKFLOW}", workflowName)
	template = strings.ReplaceAll(template, "${PARENT_SESSION}", parentSession)
	template = strings.ReplaceAll(template, "${CHILD_SESSION}", childSession)
	template = strings.ReplaceAll(template, "${MODEL_ID}", modelID)
	template = strings.ReplaceAll(template, "${MODEL}", model)

	// Expand ticket context variables (skip DB fetch for project scope)
	if strings.Contains(template, "${TICKET_TITLE}") || strings.Contains(template, "${TICKET_DESCRIPTION}") {
		if ticketID != "" {
			title, desc := s.fetchTicketInfo(projectID, ticketID)
			template = strings.ReplaceAll(template, "${TICKET_TITLE}", title)
			template = strings.ReplaceAll(template, "${TICKET_DESCRIPTION}", desc)
		} else {
			template = strings.ReplaceAll(template, "${TICKET_TITLE}", "")
			template = strings.ReplaceAll(template, "${TICKET_DESCRIPTION}", "")
		}
	}
	if strings.Contains(template, "${USER_INSTRUCTIONS}") {
		instructions := s.fetchUserInstructions(projectID, ticketID, workflowName, wfiID)
		template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", instructions)
	}
	if strings.Contains(template, "${CALLBACK_INSTRUCTIONS}") {
		cbInstructions := s.fetchCallbackInstructions(projectID, ticketID, workflowName, wfiID)
		template = strings.ReplaceAll(template, "${CALLBACK_INSTRUCTIONS}", cbInstructions)
	}

	// Expand ${PREVIOUS_DATA} — injects previous run's findings for continuation
	if strings.Contains(template, "${PREVIOUS_DATA}") {
		prevData := s.fetchPreviousData(projectID, ticketID, workflowName, agentType, modelID, phase, wfiID)
		if prevData != "" {
			prevData = "This is a continuation of a previous run. Here is what was completed:\n" + prevData
		}
		template = strings.ReplaceAll(template, "${PREVIOUS_DATA}", prevData)
	}

	// Expand findings patterns (after variable substitution)
	template, err = s.expandFindings(template, projectID, ticketID, workflowName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: findings expansion: %v\n", err)
	}

	return template, nil
}
