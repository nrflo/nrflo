package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// refineryFoldDigestKey is the finding key the `_refinery-cli` def writes
// its folded digest to (migration 000231's prompt instructs exactly this).
const refineryFoldDigestKey = "_refinery_digest"

// refineryFoldTimeoutDefault backstops req.TimeoutSec when the caller (the
// refinery chain walk) leaves it unset.
const refineryFoldTimeoutDefault = 90 * time.Second

// RunRefineryFold implements refinery.CLIFolder: it spawns a one-off
// `_refinery-cli` headless child (context_save_agent.go/consult_run.go
// pattern), waits for it to finish, and reads its `_refinery_digest`
// finding. s is a long-lived, project-agnostic host Spawner built with just
// Pool/Clock/WSHub/DataPath/PTYManager/ProjectEnv (the observerSpawner shape
// in api/server.go), wired into refinery.Manager via SetCLIFolder — it never
// carries a fixed ProjectRoot/ModelConfigs snapshot, so both are resolved
// fresh per call. A missing `_refinery-cli` def or a build-time/credential
// spawn failure returns an error wrapped in
// types.ErrRefineryFoldProviderBuild so the refinery chain walk advances
// past it; any other failure (no session registered, missing digest
// finding) is a plain error, which stops the walk.
func (s *Spawner) RunRefineryFold(ctx context.Context, req types.RefineryFoldRequest) (types.RefineryFoldResult, error) {
	pool := s.pool()
	if pool == nil {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: no database pool")
	}
	if req.ProjectID == "" || req.ModelID == "" || req.UserText == "" {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: project_id, model_id and user_text are required")
	}

	clk := s.config.Clock
	modelSvc := service.NewModelService(pool, clk)
	sysDef, err := service.NewSystemAgentDefinitionService(pool, clk, modelSvc).GetForBackend("refinery", "cli_interactive")
	if err != nil {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: _refinery-cli def not found: %w", err)
	}

	project, err := repo.NewProjectRepo(pool, clk).Get(req.ProjectID)
	if err != nil {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: project lookup failed: %w", err)
	}
	projectRoot := ""
	if project.RootPath.Valid {
		projectRoot = project.RootPath.String
	}

	// Built fresh per call (never cached on the long-lived host spawner) so
	// a newly enabled/disabled model is reflected immediately.
	modelConfigs, err := buildRefineryFoldModelConfigs(modelSvc)
	if err != nil {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: load model configs: %w", err)
	}

	var foldMu sync.Mutex
	var foldSID string
	var foldSp *Spawner
	foldRegister, foldUnregister := s.childSessionHooks(func(sid string, child *Spawner) {
		foldMu.Lock()
		if child == foldSp {
			foldSID = sid
		}
		foldMu.Unlock()
	})

	var reasoningEffort *string
	if req.ReasoningEffort != "" {
		reasoningEffort = &req.ReasoningEffort
	}

	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			refineryFoldHiddenWorkflow: {
				Phases: []PhaseDef{{NodeID: "_refinery-cli", Agent: "_refinery-cli", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"_refinery-cli": {
				Model:            req.ModelID,
				ReasoningEffort:  reasoningEffort,
				Timeout:          sysDef.Timeout,
				ExecutionMode:    sysDef.ExecutionMode,
				Tools:            sysDef.Tools,
				APIMaxIterations: sysDef.APIMaxIterations,
				APIMaxTokens:     sysDef.APIMaxTokens,
			},
		},
		DataPath:           s.config.DataPath,
		ProjectRoot:        projectRoot,
		WSHub:              s.config.WSHub,
		Pool:               pool,
		Clock:              clk,
		ClaudeSettingsJSON: s.config.ClaudeSettingsJSON,
		ExternalMCPServers: s.config.ExternalMCPServers,
		ModelConfigs:       modelConfigs,
		AgentSvc:           s.config.AgentSvc,
		// DB-backed tool services are constructed fresh per call, like
		// ModelConfigs above: the long-lived host (api's
		// wireRefineryFoldSpawner) carries none, and a nil FindingsSvc makes
		// the child's findings_add fail — the digest never lands.
		FindingsSvc:         service.NewFindingsService(pool, clk),
		ProjectFindingsSvc:  service.NewProjectFindingsService(pool, clk),
		AgentSvcReal:        service.NewAgentService(pool, clk),
		WorkflowSvc:         service.NewWorkflowService(pool, clk),
		TicketSvc:           service.NewTicketService(pool, clk),
		PTYManager:          s.config.PTYManager,
		ProjectEnv:          s.config.ProjectEnv,
		APIMode:             true,
		OnSessionRegister:   foldRegister,
		OnSessionUnregister: foldUnregister,
	})
	foldMu.Lock()
	foldSp = sp
	foldMu.Unlock()

	timeout := refineryFoldTimeoutDefault
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}
	foldCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Console chats carry no workflow instance, but Spawn demands one; back
	// the fold with a reusable hidden host instance keyed to the session.
	wfiID := req.WorkflowInstanceID
	if wfiID == "" {
		wfiID, err = s.ensureRefineryFoldHostInstance(pool, req.ProjectID, req.SessionID)
		if err != nil {
			return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: host instance: %w", err)
		}
	}

	spawnErr := sp.Spawn(foldCtx, SpawnRequest{
		AgentType:          "_refinery-cli",
		NodeID:             "_refinery-cli",
		ProjectID:          req.ProjectID,
		WorkflowName:       refineryFoldHiddenWorkflow,
		WorkflowInstanceID: wfiID,
		ScopeType:          "project",
		ExtraVars: map[string]string{
			"FOLD_INPUT": req.UserText,
		},
	})
	sp.Close()

	if spawnErr != nil {
		if isProviderBuildError(spawnErr) {
			return types.RefineryFoldResult{}, fmt.Errorf("%w: %v", types.ErrRefineryFoldProviderBuild, spawnErr)
		}
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: spawn failed: %w", spawnErr)
	}

	foldMu.Lock()
	sid := foldSID
	foldMu.Unlock()
	if sid == "" {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: no session registered for _refinery-cli")
	}

	findings, err := repo.NewFindingRepo(pool, clk).GetOwn("session", sid)
	if err != nil {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: read findings: %w", err)
	}
	raw, ok := findings[refineryFoldDigestKey]
	if !ok {
		return types.RefineryFoldResult{}, fmt.Errorf("refinery fold: _refinery-cli did not write %s", refineryFoldDigestKey)
	}

	var content string
	if jsonErr := json.Unmarshal(raw, &content); jsonErr != nil {
		content = string(raw)
	}

	return types.RefineryFoldResult{Content: content, ChildSessionID: sid}, nil
}

// buildRefineryFoldModelConfigs mirrors Orchestrator.loadModelConfigs
// (be/internal/orchestrator/orchestrator_lifecycle.go) — spawner cannot
// import orchestrator, and ModelConfig is a spawner-owned type, so the
// registry-row-to-ModelConfig mapping is duplicated here rather than shared.
func buildRefineryFoldModelConfigs(modelSvc *service.ModelService) (map[string]ModelConfig, error) {
	models, err := modelSvc.ListEnabled()
	if err != nil {
		return nil, err
	}
	configs := make(map[string]ModelConfig, len(models))
	for _, m := range models {
		configs[m.ID] = ModelConfig{
			Provider:       m.Provider,
			CLIModel:       m.CLIModel,
			CLIContext:     m.CLIContext,
			CLIEfforts:     m.CLIEfforts,
			APIModel:       m.APIModel,
			APIContext:     m.APIContext,
			APIEfforts:     m.APIEfforts,
			FallbackModels: m.FallbackModels,
			DefaultEffort:  m.DefaultEffort,
		}
	}
	return configs, nil
}
