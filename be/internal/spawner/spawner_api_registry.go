package spawner

import (
	"fmt"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
)

// buildAPIRegistry resolves the per-agent tool registry from the tools CSV,
// loads python tool definitions, resolves the registry, and assembles the
// ToolEnv. Called by the in-process api branch, the api-via-cli hybrid, and the
// cli_interactive Claude/codex path. When toolsCSVOverride is empty, the agent
// definition's tools field is used.
//
// forceBaseline merges in the baseline tools (agent_* lifecycle group plus
// findings_add) regardless of the CSV; socket-completion backends
// (cli_interactive/codex/api-via-cli) set it so a restrictive CSV can never
// strip an agent's ability to signal completion or record findings. Pure
// in-process api agents leave it false (they auto-PASS on end_turn and may be
// intentionally text-only).
func (s *Spawner) buildAPIRegistry(
	req SpawnRequest,
	wfiID string,
	agentDef *model.AgentDefinition,
	proc *processInfo,
	toolsCSVOverride string,
	forceBaseline bool,
) ([]provider.ToolSpec, apirun.Registry, apirun.ToolEnv, error) {
	toolsCSV := toolsCSVOverride
	if toolsCSV == "" {
		if agentDef != nil {
			toolsCSV = agentDef.Tools
		} else if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
			toolsCSV = agentCfg.Tools
		}
	}

	pythonHandlers, _ := s.loadProjectPythonTools(req.ProjectID, proc.sessionID)

	specs, handlers, regErr := apirun.ResolveRegistry(toolsCSV, tools_builtin.Builtins(), pythonHandlers)
	if regErr != nil {
		return nil, nil, apirun.ToolEnv{}, fmt.Errorf("api mode: %w", regErr)
	}

	if forceBaseline {
		specs, handlers = apirun.MergeBaseline(specs, handlers, tools_builtin.Builtins(), tools_builtin.BaselineToolNames())
	}

	// Recursion guard: consultant agents may not call consult themselves.
	if agentDef != nil && agentDef.Consultant {
		delete(handlers, "consult")
		filtered := specs[:0]
		for _, spec := range specs {
			if spec.Name != "consult" {
				filtered = append(filtered, spec)
			}
		}
		specs = filtered
	}

	extID, extCtx := s.fetchExternalRefs(req.ProjectID, req.TicketID, req.WorkflowName, wfiID)
	toolEnv := apirun.ToolEnv{
		Pool:               s.config.Pool,
		WSHub:              s.config.WSHub,
		Clock:              s.config.Clock,
		DispatchRepo:       s.config.DispatchRepo,
		SessionID:          proc.sessionID,
		AgentID:            proc.agentID,
		AgentType:          req.AgentType,
		ProjectID:          req.ProjectID,
		TicketID:           req.TicketID,
		WorkflowName:       req.WorkflowName,
		WorkflowInstanceID: wfiID,
		ExternalID:         extID,
		ExternalContext:    extCtx,
		Findings:           s.config.FindingsSvc,
		ProjectFindings:    s.config.ProjectFindingsSvc,
		Agent:              s.config.AgentSvcReal,
		Workflow:           s.config.WorkflowSvc,
		Ticket:             s.config.TicketSvc,
		ArtifactSvc:        s.config.ArtifactSvc,
		WorkflowControl:    s.config.WorkflowControl,
		Consultant:         s,
		ChainRun:           service.NewWorkflowChainRunService(s.config.Pool, s.config.Clock),
	}

	return specs, handlers, toolEnv, nil
}
