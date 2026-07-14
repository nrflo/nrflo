package orchestrator

import (
	"context"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
)

// consultBuildAPIProvider is a test seam so unit tests can inject a mock factory
// without needing real provider credentials.
var consultBuildAPIProvider func(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) = service.BuildAPIProvider

// Consult synchronously spawns a named consultant agent under the caller's
// workflow instance and returns its answer. It is the orchestrator-level
// entry point for the agent.consult socket method and the apirun consult
// builtin. Spawner.Consult handles all spawn mechanics; this method only
// resolves context, enforces the socket-boundary recursion guard, and builds
// an api-capable Config.
func (o *Orchestrator) Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return "", fmt.Errorf("consult: open database: %w", err)
	}
	defer database.Close()
	pool := db.WrapAsPool(database)

	// Resolve caller session.
	sessionRepo := repo.NewAgentSessionRepo(pool, o.clock)
	callerSession, err := sessionRepo.Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("consult: unknown caller session: %w", err)
	}

	projectID := callerSession.ProjectID

	// Resolve workflow instance to get the real workflow name.
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wfi, err := wfiRepo.Get(callerSession.WorkflowInstanceID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve workflow instance: %w", err)
	}

	// Recursion guard: consultants cannot initiate a consult. Keyed on AgentType
	// (template identity) — this resolves which agent_definitions row the caller
	// is, not which node it is, so it must NOT be switched to NodeID.
	defRepo := repo.NewAgentDefinitionRepo(pool, o.clock)
	callerDef, defErr := defRepo.Get(projectID, wfi.WorkflowID, callerSession.AgentType)
	if defErr == nil && callerDef.Consultant {
		return "", fmt.Errorf("consult: recursion guard — consultant agent %q cannot initiate a consult", callerSession.AgentType)
	}

	// Resolve project root; prefer worktree when one exists for ticket-scoped runs.
	projectRepo := repo.NewProjectRepo(database, o.clock)
	project, err := projectRepo.Get(projectID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve project: %w", err)
	}
	if !project.RootPath.Valid || project.RootPath.String == "" {
		return "", fmt.Errorf("consult: project %q has no root_path configured", projectID)
	}
	projectRoot := project.RootPath.String
	if wfi.WorktreePath.Valid && wfi.WorktreePath.String != "" {
		projectRoot = wfi.WorktreePath.String
	}

	// Build model configs and claude safety settings (read once; mid-consult changes have no effect).
	modelConfigs, _ := o.loadModelConfigs(pool)
	apiModelConfigs, _ := o.loadAPIModelConfigs(pool)
	claudeSettingsJSON := ""
	if raw, _ := pool.GetProjectConfig(projectID, "claude_safety_hook"); raw != "" {
		claudeSettingsJSON = spawner.BuildSafetySettingsJSON(raw)
	}

	projectEnv := loadProjectEnv(ctx, pool, projectID, o.clock)

	consultPool := pool
	cfg := spawner.Config{
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
			return consultBuildAPIProvider(ctx, consultPool, o.clock, providerName, projectID)
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
	}

	sp := spawner.New(cfg)
	defer sp.Close()
	return sp.Consult(ctx, callerSessionID, consultantID, question)
}
