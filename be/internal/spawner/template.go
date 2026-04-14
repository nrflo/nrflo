package spawner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// findingsPattern matches #{FINDINGS:agent_type} or #{FINDINGS:agent_type:key(s)}
var findingsPattern = regexp.MustCompile(`#\{FINDINGS:([^:}]+)(?::([^}]*))?\}`)

// projectFindingsPattern matches #{PROJECT_FINDINGS:key} or #{PROJECT_FINDINGS:k1,k2}
var projectFindingsPattern = regexp.MustCompile(`#\{PROJECT_FINDINGS:([^}]+)\}`)

// Preview generates the prompt without spawning
func (s *Spawner) Preview(agentType, ticketID, projectID, workflowName string) (string, error) {
	model := "opus"
	if agentCfg, ok := s.config.Agents[agentType]; ok {
		if agentCfg.Model != "" {
			model = agentCfg.Model
		}
	}
	cliName := s.cliForModel(model)
	modelID := fmt.Sprintf("%s:%s", cliName, model)
	return s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID, "", "", nil)
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
// Falls back to system_agent_definitions when project-scoped lookup fails.
func (s *Spawner) loadPromptContent(agentType, projectID, workflowName string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("failed to get database pool")
	}

	// Try project-scoped agent definition first
	adRepo := repo.NewAgentDefinitionRepo(pool, s.config.Clock)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err == nil {
		if def.Prompt == "" {
			return "", fmt.Errorf("agent definition '%s' has empty prompt", agentType)
		}
		return def.Prompt, nil
	}

	// Fallback to system agent definition
	svc := service.NewSystemAgentDefinitionService(pool, s.config.Clock)
	sysDef, sysErr := svc.Get(agentType)
	if sysErr == nil {
		if sysDef.Prompt == "" {
			return "", fmt.Errorf("system agent definition '%s' has empty prompt", agentType)
		}
		return sysDef.Prompt, nil
	}

	// Both lookups failed — return original project-scoped error
	return "", fmt.Errorf("agent definition not found: %s (workflow=%s). Create via 'nrflow agent def create %s -w %s --prompt-file=<path>'", agentType, workflowName, agentType, workflowName)
}

// fetchTicketInfo returns the ticket title and description for template expansion.
func (s *Spawner) fetchTicketInfo(projectID, ticketID string) (title, description string) {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for ticket info")
		return ticketID, "_No description available_"
	}

	ticketRepo := repo.NewTicketRepo(pool, s.config.Clock)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		logger.Warn(context.Background(), "failed to fetch ticket", "ticket_id", ticketID, "error", err)
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

// fetchUserInstructionsRaw returns user_instructions from the workflow instance findings.
// Returns "" on miss. Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchUserInstructionsRaw(projectID, ticketID, workflowName, wfiID string) string {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for user instructions")
		return ""
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
		return ""
	}
	findings := wi.GetFindings()
	if instructions, ok := findings["user_instructions"]; ok {
		if str, ok := instructions.(string); ok && str != "" {
			return str
		}
	}
	return ""
}

// fetchCallbackRaw returns raw callback instructions and from_agent from workflow instance findings.
// Returns ("", "") on miss. Uses wfiID directly when available; falls back to ticket-based lookup.
func (s *Spawner) fetchCallbackRaw(projectID, ticketID, workflowName, wfiID string) (instructions string, fromAgent string) {
	pool := s.pool()
	if pool == nil {
		logger.Warn(context.Background(), "no database pool for callback instructions")
		return "", ""
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
		return "", ""
	}
	findings := wi.GetFindings()
	callbackRaw, ok := findings["_callback"]
	if !ok {
		return "", ""
	}
	callbackMap, ok := callbackRaw.(map[string]interface{})
	if !ok {
		return "", ""
	}
	instr, _ := callbackMap["instructions"].(string)
	if instr == "" {
		return "", ""
	}
	from, _ := callbackMap["from_agent"].(string)
	return instr, from
}

// LoadTemplate is the public wrapper around loadTemplate. It loads and expands
// an agent template from DB. Used by the orchestrator to build PTY command prompts.
func (s *Spawner) LoadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, phase, wfiID string, extraVars map[string]string) (string, error) {
	return s.loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, phase, wfiID, extraVars)
}

// loadTemplate loads and expands an agent template from DB.
// wfiID is optional — when set, used for instance-specific lookups (user instructions, callbacks).
// extraVars is optional — when set, expanded after standard ${VAR} substitution.
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, phase, wfiID string, extraVars map[string]string) (string, error) {
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

	// Expand extra variables (caller-injected, e.g. BRANCH_NAME, DEFAULT_BRANCH)
	for k, v := range extraVars {
		template = strings.ReplaceAll(template, "${"+k+"}", v)
	}

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
	// Strip legacy placeholders (clean break — any stray ones become empty)
	template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", "")
	template = strings.ReplaceAll(template, "${CALLBACK_INSTRUCTIONS}", "")
	template = strings.ReplaceAll(template, "${PREVIOUS_DATA}", "")

	// Build prepend blocks (order: user-instructions → low-context → callback)
	var prepend []string
	if ui := s.fetchUserInstructionsRaw(projectID, ticketID, workflowName, wfiID); ui != "" {
		prepend = append(prepend, s.expandInjectable("user-instructions", map[string]string{"USER_INSTRUCTIONS": ui}))
	}
	prevData, _ := s.fetchPreviousDataAndReason(projectID, ticketID, workflowName, agentType, modelID, phase, wfiID)
	if prevData != "" {
		prepend = append(prepend, s.expandInjectable("low-context", map[string]string{"PREVIOUS_DATA": prevData}))
	}
	if cbInstr, cbFrom := s.fetchCallbackRaw(projectID, ticketID, workflowName, wfiID); cbInstr != "" {
		prepend = append(prepend, s.expandInjectable("callback", map[string]string{
			"CALLBACK_INSTRUCTIONS": cbInstr,
			"CALLBACK_FROM_AGENT":   cbFrom,
		}))
	}
	if len(prepend) > 0 {
		template = strings.Join(prepend, "\n") + "\n" + template
	}

	// Expand findings patterns (after variable substitution)
	template, err = s.expandFindings(template, projectID, ticketID, workflowName, wfiID)
	if err != nil {
		logger.Warn(context.Background(), "findings expansion failed", "error", err)
	}

	// Expand project findings patterns
	template, err = s.expandProjectFindings(template, projectID)
	if err != nil {
		logger.Warn(context.Background(), "project findings expansion failed", "error", err)
	}

	template += "\n#No changes needed signal: If no changes needed execute - nrflow findings add no-op:no-op"

	return template, nil
}
