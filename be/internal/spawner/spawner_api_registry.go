package spawner

import (
	"context"
	"fmt"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/spawner/apirun/tools_http"
)

// buildAPIRegistry resolves the per-agent tool registry from the tools CSV,
// loads HTTP and python tool definitions, resolves the registry, and assembles
// the ToolEnv. Called by both the in-process api branch and the api-via-cli hybrid.
func (s *Spawner) buildAPIRegistry(
	ctx context.Context,
	req SpawnRequest,
	wfiID string,
	agentDef *model.AgentDefinition,
	proc *processInfo,
) ([]provider.ToolSpec, apirun.Registry, apirun.ToolEnv, error) {
	toolsCSV := ""
	if agentDef != nil {
		toolsCSV = agentDef.Tools
	} else if agentCfg, ok := s.config.Agents[req.AgentType]; ok {
		toolsCSV = agentCfg.Tools
	}

	httpDefs, defsErr := s.loadAPIHTTPToolDefs(req.ProjectID, req.WorkflowName)
	if defsErr != nil {
		return nil, nil, apirun.ToolEnv{}, fmt.Errorf("api mode: load tool defs: %w", defsErr)
	}

	pythonHandlers, _ := s.loadProjectPythonTools(req.ProjectID, proc.sessionID)

	specs, handlers, regErr := apirun.ResolveRegistry(toolsCSV, tools_builtin.Builtins(), pythonHandlers, httpDefs, tools_http.New(nil))
	if regErr != nil {
		return nil, nil, apirun.ToolEnv{}, fmt.Errorf("api mode: %w", regErr)
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
		Findings:           s.config.FindingsSvc,
		ProjectFindings:    s.config.ProjectFindingsSvc,
		Agent:              s.config.AgentSvcReal,
		Workflow:           s.config.WorkflowSvc,
		ArtifactSvc:        s.config.ArtifactSvc,
		WorkflowControl:    s.config.WorkflowControl,
		Consultant:         s,
	}

	return specs, handlers, toolEnv, nil
}
