package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
)

const plannerTimeout = 10 * time.Minute

// plannerNodeID is the reserved (`_`-prefixed) execution node id for the
// one-off planner child session — hidden from the v4 read model the same way
// _consult is (service/workflow_response.go transientAgentTypeExclusion).
const plannerNodeID = "_planner"

// plannerAgentConfig is the small set of fields RunPlanner needs regardless of
// whether the planner def is workflow-local (agent_definitions,
// node_role='planner') or the system default (system_agent_definitions,
// role='planner').
type plannerAgentConfig struct {
	ID               string
	Model            string
	Timeout          int
	ExecutionMode    string
	Tools            string
	APIMaxIterations *int
	APIMaxTokens     *int
	ReasoningEffort  *string
}

// resolvePlannerDef resolves the planner agent definition for a workflow:
// workflow-local agent_definitions with node_role='planner' first (looked up
// under defProjectID, the project the workflow definition itself resolved
// under — selected project or GlobalProjectID), else the
// system_agent_definitions role='planner' row for the cli_interactive
// backend, else the api backend — mirroring spawner/context_save.go's
// GetForBackend+fallback.
func (o *Orchestrator) resolvePlannerDef(pool *db.Pool, defProjectID, workflowID string) (plannerAgentConfig, error) {
	var plannerDefID string
	err := pool.QueryRow(
		`SELECT id FROM agent_definitions WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND node_role = 'planner' LIMIT 1`,
		defProjectID, workflowID,
	).Scan(&plannerDefID)
	if err == nil {
		def, gErr := repo.NewAgentDefinitionRepo(pool, o.clock).Get(defProjectID, workflowID, plannerDefID)
		if gErr != nil {
			return plannerAgentConfig{}, fmt.Errorf("planner: load workflow planner def: %w", gErr)
		}
		return plannerAgentConfig{
			ID: def.ID, Model: def.Model, Timeout: def.Timeout, ExecutionMode: def.ExecutionMode,
			Tools: def.Tools, APIMaxIterations: def.APIMaxIterations, APIMaxTokens: def.APIMaxTokens,
			ReasoningEffort: def.ReasoningEffort,
		}, nil
	}
	if err != sql.ErrNoRows {
		return plannerAgentConfig{}, fmt.Errorf("planner: query workflow planner def: %w", err)
	}

	sysSvc := service.NewSystemAgentDefinitionService(pool, o.clock, service.NewAPIModelService(pool, o.clock))
	sysDef, sErr := sysSvc.GetForBackend("planner", "cli_interactive")
	if sErr != nil {
		sysDef, sErr = sysSvc.GetForBackend("planner", "api")
	}
	if sErr != nil {
		return plannerAgentConfig{}, fmt.Errorf("planner: no planner agent definition configured: %w", sErr)
	}
	return plannerAgentConfig{
		ID: sysDef.ID, Model: sysDef.Model, Timeout: sysDef.Timeout, ExecutionMode: sysDef.ExecutionMode,
		Tools: sysDef.Tools, APIMaxIterations: sysDef.APIMaxIterations, APIMaxTokens: sysDef.APIMaxTokens,
		ReasoningEffort: sysDef.ReasoningEffort,
	}, nil
}

// renderTemplateLibrary formats the enabled fanout_template defs for the
// ${TEMPLATE_LIBRARY} prompt var, so the planner only ever sees usable
// templates (a disabled model is filtered out, not just flagged). The
// description (not the prompt body) is the selection surface: it is the
// load-bearing text an operator writes to tell the planner what a template
// does and which finding key it emits to.
func renderTemplateLibrary(templates []service.PlanTemplate) string {
	if len(templates) == 0 {
		return "_No templates configured for this workflow — the plan cannot include any nodes._"
	}
	var b strings.Builder
	for _, t := range templates {
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = "(no description provided)"
		}
		fmt.Fprintf(&b, "- %s (%s, %s, effort=%s)\n  %s\n", t.ID, t.Model, t.ExecutionMode, t.ReasoningEffort, strings.ReplaceAll(desc, "\n", " "))
	}
	return b.String()
}

// renderPlanAnswers formats caller-supplied answers for the ${PLAN_ANSWERS} prompt var.
func renderPlanAnswers(answers []types.PlanAnswer) string {
	if len(answers) == 0 {
		return "_No answers provided._"
	}
	var b strings.Builder
	for _, a := range answers {
		fmt.Fprintf(&b, "- Q %s: %s\n", a.QuestionID, a.Answer)
	}
	return b.String()
}

// RunPlanner implements service.PlannerRunner: it spawns a fresh one-off
// planner child session under the caller's workflow instance and returns its
// session id once the child settles. Running under the caller's
// WorkflowInstanceID (rather than a synthetic one) is what lets
// FindingsService.loadSessionContext resolve the REAL project+workflow when
// the planner calls emit_findings, and is also what makes a workflow-local
// agent_definitions (node_role='planner') override resolve "for free" via the
// existing project-then-system prompt fallback (spawner/template.go).
func (o *Orchestrator) RunPlanner(ctx context.Context, instanceID string, in service.PlannerInput) (string, error) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return "", fmt.Errorf("planner: open database: %w", err)
	}
	defer database.Close()
	pool := db.WrapAsPool(database)

	wfi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID)
	if err != nil {
		return "", fmt.Errorf("planner: resolve workflow instance: %w", err)
	}

	_, _, defProjectID, err := o.resolveWorkflowDef(pool, wfi.ProjectID, wfi.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("planner: resolve workflow def: %w", err)
	}

	plannerDef, err := o.resolvePlannerDef(pool, defProjectID, wfi.WorkflowID)
	if err != nil {
		return "", err
	}

	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(wfi.ProjectID)
	if err != nil {
		return "", fmt.Errorf("planner: resolve project: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return "", fmt.Errorf("planner: project %q has no root_path configured", wfi.ProjectID)
	}
	projectRoot := project.RootPath.String
	if wfi.WorktreePath.Valid && wfi.WorktreePath.String != "" {
		projectRoot = wfi.WorktreePath.String
	}

	templates, err := service.AllowedTemplates(pool, wfi.ProjectID, wfi.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("planner: load template library: %w", err)
	}
	templates = service.EnabledTemplates(pool, o.clock, templates)

	instructions := ""
	if raw, ferr := repo.NewFindingRepo(pool, o.clock).GetOwn("workflow_instance", instanceID); ferr == nil {
		if v, ok := raw["user_instructions"]; ok {
			json.Unmarshal(v, &instructions) //nolint:errcheck
		}
	}

	prevManifest := in.PreviousManifest
	if prevManifest == "" {
		prevManifest = "_(none — this is the first plan)_"
	}
	feedback := in.Feedback
	if feedback == "" {
		feedback = "_(none)_"
	}

	extraVars := map[string]string{
		"PLAN_GOAL":         in.Goal,
		"PLAN_INSTRUCTIONS": instructions,
		"TEMPLATE_LIBRARY":  renderTemplateLibrary(templates),
		"PLAN_FEEDBACK":     feedback,
		"PLAN_ANSWERS":      renderPlanAnswers(in.Answers),
		"PREVIOUS_MANIFEST": prevManifest,
	}

	modelConfigs, _ := o.loadModelConfigs(pool)
	apiModelConfigs, _ := o.loadAPIModelConfigs(pool)
	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(wfi.ProjectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}
	projectEnv := loadProjectEnv(ctx, pool, wfi.ProjectID, o.clock)

	var sidMu sync.Mutex
	var plannerSID string

	plannerPool := pool
	cfg := spawner.Config{
		Workflows: map[string]spawner.WorkflowDef{
			wfi.WorkflowID: {
				Phases: []spawner.PhaseDef{{NodeID: plannerNodeID, Agent: plannerDef.ID, Layer: 0}},
			},
		},
		Agents: map[string]spawner.AgentConfig{
			plannerDef.ID: {
				Model:            plannerDef.Model,
				Timeout:          plannerDef.Timeout,
				ExecutionMode:    plannerDef.ExecutionMode,
				Tools:            plannerDef.Tools,
				APIMaxIterations: plannerDef.APIMaxIterations,
				APIMaxTokens:     plannerDef.APIMaxTokens,
				ReasoningEffort:  plannerDef.ReasoningEffort,
			},
		},
		DataPath:           o.dataPath,
		ProjectRoot:        projectRoot,
		WSHub:              o.wsHub,
		Pool:               pool,
		Clock:              o.clock,
		APIMode:            true,
		ClaudeSettingsJSON: claudeSettingsJSON,
		ModelConfigs:       modelConfigs,
		APIModelConfigs:    apiModelConfigs,
		ErrorSvc:           o.errorSvc,
		BuildAPIProvider: func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
			return buildAPIProvider(ctx, plannerPool, o.clock, providerName, projectID)
		},
		AgentSvc:           newAPIAgentSvc(pool, o.clock, o.wsHub),
		FindingsSvc:        service.NewFindingsService(pool, o.clock),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, o.clock),
		AgentSvcReal:       service.NewAgentService(pool, o.clock),
		WorkflowSvc:        service.NewWorkflowService(pool, o.clock),
		DispatchRepo:       repo.NewDispatchRepo(pool, o.clock),
		ArtifactSvc:        service.NewArtifactService(pool, o.clock, o.wsHub, o.dataPath),
		PTYManager:         o.PTYManager,
		ProjectEnv:         projectEnv,
		SDKDir:             o.sdkDir,
		PythonScriptRepo:   repo.NewPythonScriptRepo(pool, o.clock),
		OnSessionRegister: func(sid string, _ *spawner.Spawner) {
			sidMu.Lock()
			plannerSID = sid
			sidMu.Unlock()
		},
	}

	sp := spawner.New(cfg)
	defer sp.Close()

	timeout := plannerTimeout
	if plannerDef.Timeout > 0 {
		timeout = time.Duration(plannerDef.Timeout) * time.Second
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spawnErr := sp.Spawn(ctxTimeout, spawner.SpawnRequest{
		AgentType:          plannerDef.ID,
		NodeID:             plannerNodeID,
		TicketID:           wfi.TicketID,
		ProjectID:          wfi.ProjectID,
		WorkflowName:       wfi.WorkflowID,
		WorkflowInstanceID: instanceID,
		ScopeType:          wfi.ScopeType,
		ExtraVars:          extraVars,
	})

	sidMu.Lock()
	sid := plannerSID
	sidMu.Unlock()

	if spawnErr != nil {
		return "", fmt.Errorf("planner: spawn failed: %w", spawnErr)
	}
	if sid == "" {
		return "", fmt.Errorf("planner: no session registered for planner agent %q", plannerDef.ID)
	}
	return sid, nil
}
