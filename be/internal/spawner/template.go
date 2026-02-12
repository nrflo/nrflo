package spawner

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// Preview generates the prompt without spawning
func (s *Spawner) Preview(agentType, ticketID, projectID, workflowName string) (string, error) {
	// Get model from config for preview
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
	return s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID)
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
// Returns placeholder text on error rather than failing the spawn.
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
// Returns placeholder text on error rather than failing the spawn.
func (s *Spawner) fetchUserInstructions(projectID, ticketID, workflowName string) string {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open DB for user instructions: %v\n", err)
		return "_No user instructions provided_"
	}
	defer pool.Close()

	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch workflow instance for %s/%s: %v\n", ticketID, workflowName, err)
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
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID string) (string, error) {
	promptContent, err := s.loadPromptContent(agentType, projectID, workflowName)
	if err != nil {
		return "", err
	}

	template := promptContent

	// Parse model from modelID
	_, model := parseModelID(modelID)
	if model == "" {
		model = "sonnet"
	}

	// Expand variables
	template = strings.ReplaceAll(template, "${AGENT}", agentType)
	template = strings.ReplaceAll(template, "${TICKET_ID}", ticketID)
	template = strings.ReplaceAll(template, "${WORKFLOW}", workflowName)
	template = strings.ReplaceAll(template, "${PARENT_SESSION}", parentSession)
	template = strings.ReplaceAll(template, "${CHILD_SESSION}", childSession)
	template = strings.ReplaceAll(template, "${MODEL_ID}", modelID)
	template = strings.ReplaceAll(template, "${MODEL}", model)

	// Expand ticket context variables (title, description, user instructions)
	if strings.Contains(template, "${TICKET_TITLE}") || strings.Contains(template, "${TICKET_DESCRIPTION}") {
		title, desc := s.fetchTicketInfo(projectID, ticketID)
		template = strings.ReplaceAll(template, "${TICKET_TITLE}", title)
		template = strings.ReplaceAll(template, "${TICKET_DESCRIPTION}", desc)
	}
	if strings.Contains(template, "${USER_INSTRUCTIONS}") {
		instructions := s.fetchUserInstructions(projectID, ticketID, workflowName)
		template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", instructions)
	}

	// Expand findings patterns (after variable substitution)
	template, err = s.expandFindings(template, projectID, ticketID, workflowName)
	if err != nil {
		// Log warning but don't fail - findings might not exist yet
		fmt.Fprintf(os.Stderr, "Warning: findings expansion: %v\n", err)
	}

	return template, nil
}

// expandFindings replaces #{FINDINGS:AGENT:KEY} patterns with actual findings data.
// Patterns:
//   - #{FINDINGS:agent}           - All findings for agent
//   - #{FINDINGS:agent:key}       - Single specific key
//   - #{FINDINGS:agent:key1,key2} - Multiple specific keys
func (s *Spawner) expandFindings(template, projectID, ticketID, workflowName string) (string, error) {
	// Pattern: #{FINDINGS:agent_type} or #{FINDINGS:agent_type:key(s)}
	re := regexp.MustCompile(`#\{FINDINGS:([^:}]+)(?::([^}]*))?\}`)

	var lastErr error
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		agentType := parts[1]
		var keys []string
		if len(parts) >= 3 && parts[2] != "" {
			// Split comma-separated keys
			keys = strings.Split(parts[2], ",")
			for i := range keys {
				keys[i] = strings.TrimSpace(keys[i])
			}
		}

		// Fetch findings
		findings, err := s.fetchFindings(projectID, ticketID, workflowName, agentType, keys)
		if err != nil {
			lastErr = err
			return s.formatFindingsError(agentType)
		}

		return s.formatFindings(agentType, findings, keys)
	})

	return result, lastErr
}

// fetchFindings retrieves findings from the database using the FindingsService
func (s *Spawner) fetchFindings(projectID, ticketID, workflowName, agentType string, keys []string) (interface{}, error) {
	pool, err := db.NewPool(s.config.DataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer pool.Close()

	findingsService := service.NewFindingsService(pool)

	req := &types.FindingsGetRequest{
		Workflow:  workflowName,
		AgentType: agentType,
		Keys:      keys,
	}

	return findingsService.Get(projectID, ticketID, req)
}

// formatFindings converts findings to human-readable text (YAML-like format)
func (s *Spawner) formatFindings(agentType string, findings interface{}, keys []string) string {
	if findings == nil {
		return s.formatFindingsError(agentType)
	}

	findingsMap, ok := findings.(map[string]interface{})
	if !ok {
		// Single value (when fetching a single key)
		return s.formatValue(findings, "")
	}

	if len(findingsMap) == 0 {
		return s.formatFindingsError(agentType)
	}

	// Check if this is a parallel agents result (keys are model IDs like "claude:opus")
	isParallel := false
	for k := range findingsMap {
		if strings.Contains(k, ":") {
			isParallel = true
			break
		}
	}

	if isParallel {
		return s.formatParallelFindings(agentType, findingsMap, keys)
	}

	return s.formatSingleAgentFindings(findingsMap)
}

// formatParallelFindings formats findings from multiple parallel agents
func (s *Spawner) formatParallelFindings(agentType string, findings map[string]interface{}, keys []string) string {
	var lines []string

	// Sort model keys for consistent output
	var modelKeys []string
	for k := range findings {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	for _, modelKey := range modelKeys {
		agentKey := agentType + ":" + modelKey
		v := findings[modelKey]

		if len(keys) == 1 {
			// Single key requested - compact format: "- agent:model: value"
			lines = append(lines, fmt.Sprintf("- %s: %s", agentKey, s.formatValue(v, "")))
		} else {
			// Multiple keys or all findings - expanded format
			lines = append(lines, fmt.Sprintf("- %s:", agentKey))
			if agentFindings, ok := v.(map[string]interface{}); ok {
				// Sort keys for consistent output
				var sortedKeys []string
				for k := range agentFindings {
					sortedKeys = append(sortedKeys, k)
				}
				sort.Strings(sortedKeys)
				for _, k := range sortedKeys {
					val := agentFindings[k]
					lines = append(lines, "  "+k+":"+s.formatValue(val, "  "))
				}
			} else {
				lines = append(lines, "  "+s.formatValue(v, "  "))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// formatSingleAgentFindings formats findings from a single agent as "key: value" lines
func (s *Spawner) formatSingleAgentFindings(findings map[string]interface{}) string {
	var lines []string

	// Sort keys for consistent output
	var keys []string
	for k := range findings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := findings[key]
		lines = append(lines, key+":"+s.formatValue(val, ""))
	}
	return strings.Join(lines, "\n")
}

// formatValue converts any value to YAML-like text (never JSON)
func (s *Spawner) formatValue(v interface{}, indent string) string {
	switch val := v.(type) {
	case string:
		// Simple string value - add space after colon if inline
		if indent == "" {
			return " " + val
		}
		return " " + val
	case []interface{}:
		// Array - bullet list
		var lines []string
		for _, item := range val {
			itemStr := s.formatValue(item, indent+"  ")
			// Remove leading space for array items
			itemStr = strings.TrimPrefix(itemStr, " ")
			lines = append(lines, indent+"  - "+itemStr)
		}
		return "\n" + strings.Join(lines, "\n")
	case map[string]interface{}:
		// Object - nested key: value
		var lines []string
		// Sort keys for consistent output
		var keys []string
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, indent+"  "+k+":"+s.formatValue(val[k], indent+"  "))
		}
		return "\n" + strings.Join(lines, "\n")
	case float64:
		// JSON numbers come as float64
		if val == float64(int(val)) {
			return fmt.Sprintf(" %d", int(val))
		}
		return fmt.Sprintf(" %v", val)
	case bool:
		return fmt.Sprintf(" %v", val)
	case nil:
		return " null"
	default:
		return fmt.Sprintf(" %v", val)
	}
}

// formatFindingsError returns a placeholder for missing findings
func (s *Spawner) formatFindingsError(agentType string) string {
	return fmt.Sprintf("_No findings yet available from %s_", agentType)
}
