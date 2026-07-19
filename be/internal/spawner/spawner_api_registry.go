package spawner

import (
	"fmt"

	"be/internal/clock"
	"be/internal/db"
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
//
// includeFS additionally offers the native filesystem/shell tools
// (tools_builtin.FSTools — read_file/edit_file/bash, jailed to the workdir).
// Only the pure in-process api branch passes true, and only when the
// `api_native_tools_enabled` global setting is on: CLI-backed agents have
// their CLI's own native tools, so granting a second bash over MCP would be
// redundant surface.
func (s *Spawner) buildAPIRegistry(
	req SpawnRequest,
	wfiID string,
	agentDef *model.AgentDefinition,
	proc *processInfo,
	toolsCSVOverride string,
	forceBaseline bool,
	includeFS bool,
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

	builtins := tools_builtin.Builtins()
	if includeFS && proc.workDir != "" && apiNativeToolsEnabled(s.config.Pool, s.config.Clock) {
		for name, handler := range tools_builtin.FSTools() {
			builtins[name] = handler
		}
	}

	specs, handlers, regErr := apirun.ResolveRegistry(toolsCSV, builtins, pythonHandlers)
	if regErr != nil {
		return nil, nil, apirun.ToolEnv{}, fmt.Errorf("api mode: %w", regErr)
	}

	if forceBaseline {
		specs, handlers = apirun.MergeBaseline(specs, handlers, tools_builtin.Builtins(), tools_builtin.BaselineToolNames())
	}

	extID, extCtx, subDepth := s.fetchExternalRefs(req.ProjectID, req.TicketID, req.WorkflowName, wfiID)

	// Recursion guard: consultant agents may not call consult themselves.
	if agentDef != nil && agentDef.Consultant {
		specs = stripTool(specs, handlers, "consult")
	}
	// Nesting guard: agents of a run at the sub-workflow depth cap may not start
	// further sub-workflows. Depth-based (not name-based) so it also bounds
	// mutual recursion A->B->A; StartSubworkflow/StartDynamicWorkflow re-check
	// server-side. Uses subworkflow_depth (run_subworkflow/dynamic_workflow
	// nesting only), so next-on-success chain hops never lose the tool.
	if subDepth+1 > service.SubworkflowCap(s.config.Pool, req.ProjectID, service.SubworkflowMaxDepthKey, service.DefaultSubworkflowMaxDepth) {
		specs = stripTool(specs, handlers, "run_subworkflow")
		specs = stripTool(specs, handlers, "dynamic_workflow")
	}
	// Nesting guard: delegate workers at the delegate-depth cap may not
	// delegate further. _t2_extractor never has "delegate" in its tools CSV
	// to begin with (native guard); this only bites _t1_executor once its
	// chain depth reaches the cap. Depth is this spawner's own in-memory
	// Config.DelegateDepth (0 for a top-level spawner, N for an N-levels-down
	// delegate worker's child spawner) — per-chain and race-free, unlike a
	// shared instance counter. Same shape as the subworkflow guard above.
	if s.config.DelegateDepth+1 > service.DelegateMaxDepth(s.config.Pool, req.ProjectID) {
		specs = stripTool(specs, handlers, "delegate")
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
		Delegator:          s,
		ChainRun:           service.NewWorkflowChainRunService(s.config.Pool, s.config.Clock),
		Subworkflows:       s.config.Subworkflows,
		Heartbeat:          func() { s.BumpLastMessage(proc.sessionID) },
		WorkDir:            proc.workDir,
	}

	return specs, handlers, toolEnv, nil
}

// apiNativeToolsEnabled reads the `api_native_tools_enabled` global setting
// (default off): whether in-process api agents/chats may use the native
// read_file/edit_file/bash tools.
func apiNativeToolsEnabled(pool *db.Pool, clk clock.Clock) bool {
	v, _ := service.NewGlobalSettingsService(pool, clk).Get("api_native_tools_enabled")
	return v == "true"
}

// stripTool removes a tool by name from both the handler registry and the spec
// list (recursion guards). Returns the filtered specs.
func stripTool(specs []provider.ToolSpec, handlers apirun.Registry, name string) []provider.ToolSpec {
	delete(handlers, name)
	filtered := specs[:0]
	for _, spec := range specs {
		if spec.Name != name {
			filtered = append(filtered, spec)
		}
	}
	return filtered
}
