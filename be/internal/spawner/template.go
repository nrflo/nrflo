package spawner

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"be/internal/db"
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
	return s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID, "")
}

// loadAgentDefinition loads the full agent definition from the DB.
// Returns nil if not found (caller should fall back to defaults).
func (s *Spawner) loadAgentDefinition(agentType, projectID, workflowName string) *model.AgentDefinition {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return nil
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err != nil {
		return nil
	}
	return def
}

// loadPromptContent loads the prompt content for an agent from the DB.
func (s *Spawner) loadPromptContent(agentType, projectID, workflowName string) (string, error) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		return "", fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database)
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
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open DB for ticket info: %v\n", err)
		return ticketID, "_No description available_"
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database)
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
// ticketID can be empty for project-scoped workflows.
func (s *Spawner) fetchUserInstructions(projectID, ticketID, workflowName string) string {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open DB for user instructions: %v\n", err)
		return "_No user instructions provided_"
	}
	defer pool.Close()

	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	var wi *model.WorkflowInstance
	if ticketID == "" {
		wi, err = wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
	} else {
		wi, err = wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	}
	if err != nil {
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

// loadTemplate loads and expands an agent template from DB.
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, phase string) (string, error) {
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
		instructions := s.fetchUserInstructions(projectID, ticketID, workflowName)
		template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", instructions)
	}

	// Expand ${PREVIOUS_DATA} — injects previous run's findings for continuation
	if strings.Contains(template, "${PREVIOUS_DATA}") {
		prevData := s.fetchPreviousData(projectID, ticketID, workflowName, agentType, modelID, phase)
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
