package spawner

import (
	"context"
	"fmt"

	"be/internal/model"
	"be/internal/service"
)

// prepareAPIModeSpawn finishes API-mode prep once executionMode=="api": registry
// row lookup (or api-via-cli handoff), provider construction, effort/tool
// registry resolution, and prepResult.api* assembly. Split out of
// prepareSpawn to keep that file under its line cap. agentModel is the bare
// model id (parseModelID's second return); modelID is the full "cli:model"
// form.
func (s *Spawner) prepareAPIModeSpawn(ctx context.Context, req SpawnRequest, agentModel, modelID, phase, wfiID, sessionID, spawnToken string, effectiveThreshold int, extID, extCtx, prompt, suffix string, tmplVars map[string]string, agentDef *model.AgentDefinition, proc *processInfo, prep *prepResult) (*processInfo, *prepResult, error) {
	// API mode always requires a supported registry row.
	am, ok := s.config.ModelConfigs[agentModel]
	if !ok {
		return nil, nil, fmt.Errorf("api mode: model %q not found in models", agentModel)
	}
	if am.APIModel == "" {
		return nil, nil, wrapProviderBuildErr(fmt.Errorf("api mode: model %q does not support api mode", agentModel))
	}

	// api-via-cli hybrid: route Anthropic api-models through the Claude CLI.
	if s.config.APIViaCLI && am.Provider == "anthropic" {
		return s.prepareAPIViaCLISpawn(ctx, req, wfiID, sessionID, spawnToken, effectiveThreshold, extID, extCtx, prompt, am, agentDef, proc, prep)
	}

	// Build the provider for this spawn. Fail fast on missing credentials.
	apiProv, provErr := s.config.BuildAPIProvider(ctx, am.Provider, req.ProjectID)
	if provErr != nil {
		return nil, nil, wrapProviderBuildErr(fmt.Errorf("api mode: %w", provErr))
	}
	prep.apiProvider = apiProv
	apiEffort := s.resolveReasoningEffort(agentDef, req.AgentType, am.DefaultEffort)
	if req.ReasoningEffortOverride != "" {
		apiEffort = req.ReasoningEffortOverride
	}
	if err := service.ValidateEffortAllowed(apiEffort, am.APIEfforts); err != nil {
		return nil, nil, fmt.Errorf("api mode: %w", err)
	}
	prep.apiReasoningEffort, proc.resolvedEffort = apiEffort, apiEffort
	prep.apiCaptureThinking = s.projectOrGlobalBool(req.ProjectID, "capture_thinking_enabled")
	apiModelID := am.APIModel

	maxIter := defaultAPIMaxIterations
	if agentDef != nil && agentDef.APIMaxIterations != nil && *agentDef.APIMaxIterations > 0 {
		maxIter = *agentDef.APIMaxIterations
	} else if agentDef == nil {
		if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.APIMaxIterations != nil && *agentCfg.APIMaxIterations > 0 {
			maxIter = *agentCfg.APIMaxIterations
		}
	}

	maxTokens := defaultAPIMaxTokens
	if agentDef != nil && agentDef.APIMaxTokens != nil && *agentDef.APIMaxTokens > 0 {
		maxTokens = *agentDef.APIMaxTokens
	} else if agentDef == nil {
		if agentCfg, ok := s.config.Agents[req.AgentType]; ok && agentCfg.APIMaxTokens != nil && *agentCfg.APIMaxTokens > 0 {
			maxTokens = *agentCfg.APIMaxTokens
		}
	}
	maxCtx := am.APIContext // Registry context is authoritative; provider is fallback.
	if maxCtx <= 0 {
		maxCtx = apiProv.MaxContext(apiModelID)
	}
	proc.maxContext = maxCtx
	prep.apiContextBudget = resolveContextBudget(agentDef, deriveContextBudgetDefault(s.pool(), maxCtx))

	specs, handlers, toolEnv, regErr := s.buildAPIRegistry(req, wfiID, agentDef, proc, "", false, true, false)
	if regErr != nil {
		return nil, nil, regErr
	}

	prep.apiSystem = apiSystemPromptWithSuffix(ctx, s.pool(), stdTemplateVars(req.AgentType, phase, req.TicketID, req.ProjectID, req.WorkflowName, req.ParentSession, sessionID, modelID, tmplVars), suffix, defaultAPISystemPrompt, agentDefSystemTemplateID(agentDef), specs)
	proc.systemPrompt = prep.apiSystem
	prep.apiInitialPrompt = prompt
	prep.apiTools = specs
	prep.apiHandlers = handlers
	prep.apiToolEnv = toolEnv
	prep.apiMaxIterations = maxIter
	prep.apiMaxTokens = maxTokens
	prep.apiDeadline = proc.startTime.Add(proc.timeout)
	prep.apiModelID = apiModelID
	prep.apiMaxContext = maxCtx
	return proc, prep, nil
}
