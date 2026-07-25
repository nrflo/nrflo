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

// layerFindingsPattern matches #{LAYER_FINDINGS:N} or #{PRIOR_LAYER_FINDINGS}
var layerFindingsPattern = regexp.MustCompile(`#\{LAYER_FINDINGS:(\d+)\}|#\{PRIOR_LAYER_FINDINGS\}`)

// nodeFindingsPattern matches #{NODE_FINDINGS:node_id} or #{NODE_FINDINGS:node_id:key(s)}
var nodeFindingsPattern = regexp.MustCompile(`#\{NODE_FINDINGS:([^:}]+)(?::([^}]*))?\}`)

// Preview generates the prompt without spawning
func (s *Spawner) Preview(agentType, ticketID, projectID, workflowName string) (string, error) {
	model := "opus-5"
	if agentCfg, ok := s.config.Agents[agentType]; ok {
		if agentCfg.Model != "" {
			model = agentCfg.Model
		}
	}
	cliName := s.cliForModel(model)
	modelID := fmt.Sprintf("%s:%s", cliName, model)
	currentLayer := 0
	if def := s.loadAgentDefinition(agentType, projectID, workflowName); def != nil {
		currentLayer = def.Layer
	}
	body, _, _, err := s.loadTemplate(agentType, ticketID, projectID, "preview-parent", "preview-child", workflowName, modelID, "" /* nodeID: unset for Preview */, "", nil, currentLayer)
	return body, err
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
// Falls back to system_agent_definitions when project-scoped lookup fails or
// when the per-project prompt is empty (empty = inherit from system definition).
func (s *Spawner) loadPromptContent(agentType, projectID, workflowName string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("failed to get database pool")
	}

	// Try project-scoped agent definition first; fall through to system
	// agent definition when not found or when prompt is empty (inherit).
	adRepo := repo.NewAgentDefinitionRepo(pool, s.config.Clock)
	def, err := adRepo.Get(projectID, workflowName, agentType)
	if err == nil && def.Prompt != "" {
		return def.Prompt, nil
	}

	// Fallback to system agent definition
	svc := service.NewSystemAgentDefinitionService(pool, s.config.Clock, service.NewModelService(pool, s.config.Clock))
	sysDef, sysErr := svc.Get(agentType)
	if sysErr == nil {
		if sysDef.Prompt == "" {
			return "", fmt.Errorf("system agent definition '%s' has empty prompt", agentType)
		}
		return sysDef.Prompt, nil
	}

	// Both lookups failed — return original project-scoped error
	return "", fmt.Errorf("agent definition not found: %s (workflow=%s). Create via 'nrflo agent def create %s -w %s --prompt-file=<path>'", agentType, workflowName, agentType, workflowName)
}

// LoadTemplate is the public wrapper around loadTemplate. It loads and expands
// an agent template from DB. Used by the orchestrator to build PTY command prompts.
// Returns (body, suffix, systemPromptOverride, error). The suffix is the rendered
// system-prompt-suffix injectable for --append-system-prompt-file; systemPromptOverride
// is non-empty only when the model has CLIType=="claude" and claude_system_prompt_override_enabled is on.
// currentLayer is the agent's layer number (0 for system agents and L0 interactive starts).
func (s *Spawner) LoadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, nodeID, wfiID string, extraVars map[string]string, currentLayer int) (string, string, string, error) {
	return s.loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, nodeID, wfiID, extraVars, currentLayer)
}

// loadTemplate loads and expands an agent template from DB.
// wfiID is optional — when set, used for instance-specific lookups (user instructions, callbacks).
// nodeID is the execution slot id (empty for Preview/interactive-L0 starts, which have no node).
// extraVars is optional — when set, expanded after standard ${VAR} substitution.
// currentLayer is the agent's layer number; used to resolve #{LAYER_FINDINGS:N} and #{PRIOR_LAYER_FINDINGS}.
// Returns (body, suffix, systemPromptOverride, error). The suffix is the rendered
// system-prompt-suffix injectable; systemPromptOverride is non-empty only when the model
// is an Anthropic CLI model and the global claude_system_prompt_override_enabled setting is on.
func (s *Spawner) loadTemplate(agentType, ticketID, projectID, parentSession, childSession, workflowName, modelID, nodeID, wfiID string, extraVars map[string]string, currentLayer int) (string, string, string, error) {
	promptContent, err := s.loadPromptContent(agentType, projectID, workflowName)
	if err != nil {
		return "", "", "", err
	}

	template := promptContent

	// Resolve the agent def once — threaded into both the system-prompt
	// override resolution and the stepwise appender below instead of each
	// doing its own lookup.
	def := s.loadAgentDefinition(agentType, projectID, workflowName)

	_, model := parseModelID(modelID)
	if model == "" {
		model = "sonnet-5"
	}

	// nodeVar falls back to agentType when nodeID is unset (Preview, interactive L0 starts).
	nodeVar := nodeID
	if nodeVar == "" {
		nodeVar = agentType
	}

	// Build the standard vars map (used for both template body and suffix expansion)
	stdVars := stdTemplateVars(agentType, nodeID, ticketID, projectID, workflowName, parentSession, childSession, modelID, extraVars)

	// Expand variables
	template = strings.ReplaceAll(template, "${AGENT}", agentType)
	template = strings.ReplaceAll(template, "${NODE_ID}", nodeVar)
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

	// Render the system-prompt-suffix injectable using the same vars
	suffix := s.expandInjectable("system-prompt-suffix", stdVars)

	// Compute system-prompt override: agent def's system_template_id wins when
	// set and renders non-empty; else the existing claude_system_prompt_override_enabled gate.
	systemPromptOverride := s.resolveSystemPromptOverride(def, model, stdVars)

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
	// A static agent's template never receives NODE_INSTRUCTIONS (only
	// materialized plan nodes do, via ExtraVars) — blank an unset placeholder
	// rather than leave the literal in the rendered prompt.
	template = strings.ReplaceAll(template, "${NODE_INSTRUCTIONS}", "")

	// Strip legacy placeholders (clean break — any stray ones become empty)
	template = strings.ReplaceAll(template, "${USER_INSTRUCTIONS}", "")
	template = strings.ReplaceAll(template, "${CALLBACK_INSTRUCTIONS}", "")
	template = strings.ReplaceAll(template, "${PREVIOUS_DATA}", "")

	// Build prepend blocks (order: user-instructions → low-context → callback)
	var prepend []string
	if ui := s.fetchUserInstructionsRaw(projectID, ticketID, workflowName, wfiID); ui != "" {
		prepend = append(prepend, s.expandInjectable("user-instructions", map[string]string{"USER_INSTRUCTIONS": ui}))
	}
	prevData, _ := s.fetchPreviousDataAndReason(projectID, ticketID, workflowName, agentType, modelID, nodeID, wfiID)
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

	// Expand layer findings patterns (after variable substitution, before agent findings)
	template, err = s.expandLayerFindings(template, currentLayer, projectID, wfiID)
	if err != nil {
		logger.Warn(context.Background(), "layer findings expansion failed", "error", err)
	}

	// Expand node findings patterns (before #{FINDINGS:...}). Per-match failures
	// (unknown node, missing key) expand to "" and warn; there is nothing to
	// propagate.
	template = s.expandNodeFindings(template, wfiID)

	// Expand artifact patterns
	template, err = s.expandArtifacts(template, projectID, wfiID)
	if err != nil {
		logger.Warn(context.Background(), "artifact expansion failed", "error", err)
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

	// Append the stepwise guidance + step outline + current step instruction
	// block. No-op (returns template unchanged) for full-mode/nil defs.
	template = s.appendStepwiseBlock(template, def, wfiID, nodeID, stdVars)

	return template, suffix, systemPromptOverride, nil
}
