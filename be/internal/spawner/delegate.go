package spawner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// delegateHiddenWorkflow is the lazily-seeded global workflow definition that
// hosts a fresh workflow instance for a Delegate caller with no bound
// workflow instance of its own (a console session).
const delegateHiddenWorkflow = "_delegate_host"

var delegateTierDefs = map[string]string{
	"extractor": "_t2_extractor",
	"executor":  "_t1_executor",
}

// Delegate implements apirun.Delegator: resolves the caller's context (real
// workflow instance for an in-run agent, a fresh hidden host instance for a
// console session with no bound run), spawns one detached hidden `_delegate`
// worker per fanout item under it, tracks the delegation, and returns
// immediately — GetDelegation polls the result. Mirrors Consult, but
// downward (tier resolves to a system_agent_definitions row) and async.
func (s *Spawner) Delegate(ctx context.Context, callerSessionID string, req apirun.DelegateRequest) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("delegate: no database pool")
	}
	tierAgentID, ok := delegateTierDefs[req.Tier]
	if !ok {
		return "", fmt.Errorf("delegate: unknown tier %q", req.Tier)
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	callerSession, err := sessionRepo.Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	var wfi *model.WorkflowInstance
	isHost := callerSession.WorkflowInstanceID == ""
	if isHost {
		// Console (or other run-less) caller: no bound workflow instance to
		// spawn workers under, so mint a hidden host instance for this call.
		wfi, err = s.createDelegateHostInstance(pool, callerSession.ProjectID)
	} else {
		wfi, err = wfiRepo.Get(callerSession.WorkflowInstanceID)
	}
	if err != nil {
		return "", fmt.Errorf("delegate: resolve workflow instance: %w", err)
	}

	sysAgentSvc := service.NewSystemAgentDefinitionService(pool, s.config.Clock, service.NewModelService(pool, s.config.Clock))
	sysDef, err := sysAgentSvc.Get(tierAgentID)
	if err != nil {
		return "", fmt.Errorf("delegate: load tier definition %q: %w", tierAgentID, err)
	}
	chain, err := sysAgentSvc.ResolveAgentChain(sysDef)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve agent chain for %q: %w", tierAgentID, err)
	}

	items := req.Fanout
	if len(items) == 0 {
		items = []string{""}
	}

	delegationID := wfi.ID + "." + uuid.New().String()[:8]

	// Persist an initial (not-yet-done) tracking record synchronously, before
	// returning, so GetDelegation resolves the delegation as {status:running}
	// for the whole time the workers run. Each worker Spawn blocks until its
	// agent completes (consult pattern), so the final session-id list only
	// exists after wg.Wait; without this seed the bounded-wait poll would hit
	// "unknown delegation" on its first tick and abort.
	if err := s.trackDelegation(wfi.ID, callerSession.ProjectID, delegationID, req.Tier, nil, false); err != nil {
		return "", fmt.Errorf("delegate: seed tracking: %w", err)
	}

	s.broadcast(ws.EventDelegateStarted, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
		"caller_session_id": callerSessionID,
		"delegation_id":     delegationID,
		"tier":              req.Tier,
		"fanout":            len(items),
	})

	// Detached: the caller's tool ctx must not cancel the workers (async
	// contract, matches startChildRun's context.Background() start).
	go s.runDelegateFanout(wfi, callerSession, tierAgentID, sysDef, chain, req, items, delegationID, isHost)

	return delegateStatusJSON(delegationID, "running", nil), nil
}

// runDelegateFanout spawns one worker per item concurrently (each Spawn blocks
// until its worker finishes), then records the final session-id list with
// done=true so GetDelegation flips out of "running", completes a hidden host
// instance, and broadcasts completion. chain is the worker's full resolved
// tier fallback chain (index 0 = primary, driving the initial spawn).
func (s *Spawner) runDelegateFanout(wfi *model.WorkflowInstance, callerSession *model.AgentSession, tierAgentID string, sysDef *model.SystemAgentDefinition, chain []service.AgentChainEntry, req apirun.DelegateRequest, items []string, delegationID string, isHost bool) {
	sessionIDs := make([]string, len(items))
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(i int, item string) {
			defer wg.Done()
			sessionIDs[i] = s.spawnDelegateWorker(wfi, callerSession, tierAgentID, sysDef, chain, req, item)
		}(i, item)
	}
	wg.Wait()

	// Hidden host instances (run-less console callers) never re-enter the
	// orchestrator, so mark this one terminal here instead of leaving it
	// "active" forever. The tracking + worker findings live on it and stay
	// readable — GetDelegation still resolves them post-completion.
	if isHost {
		repo.NewWorkflowInstanceRepo(s.pool(), s.config.Clock).UpdateStatus(wfi.ID, model.WorkflowInstanceCompleted) //nolint:errcheck
	}

	if err := s.trackDelegation(wfi.ID, callerSession.ProjectID, delegationID, req.Tier, sessionIDs, true); err != nil {
		s.broadcast(ws.EventDelegateFailed, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
			"caller_session_id": callerSession.ID,
			"delegation_id":     delegationID,
			"error":             err.Error(),
		})
		return
	}
	s.broadcast(ws.EventDelegateCompleted, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
		"caller_session_id": callerSession.ID,
		"delegation_id":     delegationID,
	})
}

// spawnDelegateWorker spawns a single hidden `_delegate` worker under wfi in
// its own one-off child Spawner (mirrors consult.go's per-call sp). The
// worker's AgentConfig carries the full chain (index 0 = primary, driving
// Model/ExecutionMode/ReasoningEffort for the initial spawn) so a build-time
// or HARD failure can advance to a fallback entry. Returns the registered
// session id, or "" on spawn failure.
func (s *Spawner) spawnDelegateWorker(wfi *model.WorkflowInstance, callerSession *model.AgentSession, tierAgentID string, sysDef *model.SystemAgentDefinition, chain []service.AgentChainEntry, req apirun.DelegateRequest, item string) string {
	var mu sync.Mutex
	var sid string

	primary := chain[0]
	effort := primary.ReasoningEffort
	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			wfi.WorkflowID: {
				Phases: []PhaseDef{{NodeID: "_delegate", Agent: tierAgentID, Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			tierAgentID: {
				Model:            primary.ModelID,
				Timeout:          sysDef.Timeout,
				ExecutionMode:    primary.ExecutionMode,
				Tools:            sysDef.Tools,
				APIMaxIterations: sysDef.APIMaxIterations,
				APIMaxTokens:     sysDef.APIMaxTokens,
				ReasoningEffort:  &effort,
				Chain:            chain,
			},
		},
		DataPath:           s.config.DataPath,
		ProjectRoot:        s.config.ProjectRoot,
		WSHub:              s.config.WSHub,
		Pool:               s.config.Pool,
		Clock:              s.config.Clock,
		ClaudeSettingsJSON: s.config.ClaudeSettingsJSON,
		ModelConfigs:       s.config.ModelConfigs,
		ErrorSvc:           s.config.ErrorSvc,
		BuildAPIProvider:   s.config.BuildAPIProvider,
		AgentSvc:           s.config.AgentSvc,
		FindingsSvc:        s.config.FindingsSvc,
		ProjectFindingsSvc: s.config.ProjectFindingsSvc,
		AgentSvcReal:       s.config.AgentSvcReal,
		WorkflowSvc:        s.config.WorkflowSvc,
		TicketSvc:          s.config.TicketSvc,
		DispatchRepo:       s.config.DispatchRepo,
		ArtifactSvc:        s.config.ArtifactSvc,
		PTYManager:         s.config.PTYManager,
		ProjectEnv:         s.config.ProjectEnv,
		Subworkflows:       s.config.Subworkflows,
		APIMode:            true,
		// One level down this delegate chain: the worker's own spawner carries
		// DelegateDepth+1 so its buildAPIRegistry (and any delegate it makes)
		// sees the correct per-chain depth. Never a shared instance counter.
		DelegateDepth: s.config.DelegateDepth + 1,
		OnSessionRegister: func(registeredSID string, _ *Spawner) {
			mu.Lock()
			sid = registeredSID
			mu.Unlock()
		},
	})
	defer sp.Close()

	spawnCtx, cancel := context.WithTimeout(context.Background(), SpawnDeadline(sysDef.Timeout, 30*time.Minute))
	defer cancel()

	spawnErr := sp.Spawn(spawnCtx, SpawnRequest{
		AgentType:          tierAgentID,
		NodeID:             "_delegate",
		TicketID:           callerSession.TicketID,
		ProjectID:          callerSession.ProjectID,
		WorkflowName:       wfi.WorkflowID,
		WorkflowInstanceID: wfi.ID,
		ScopeType:          wfi.ScopeType,
		ExtraVars: map[string]string{
			"DELEGATE_BRIEF":   req.Brief,
			"DELEGATE_CONTEXT": delegateContext(req),
			"DELEGATE_ITEM":    item,
		},
	})
	if spawnErr != nil {
		s.broadcast(ws.EventDelegateFailed, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
			"caller_session_id": callerSession.ID,
			"tier":              req.Tier,
			"error":             spawnErr.Error(),
		})
	}

	mu.Lock()
	defer mu.Unlock()
	return sid
}

// delegateContext appends a hint naming the caller-supplied artifacts to the
// inline context so the worker knows which #{ARTIFACT:name} to fetch out of
// the #{ARTIFACTS} listing (all materialized artifacts on the shared
// instance, not just the ones the caller named).
func delegateContext(req apirun.DelegateRequest) string {
	if len(req.Artifacts) == 0 {
		return req.Context
	}
	hint := "Relevant artifacts: " + strings.Join(req.Artifacts, ", ")
	if req.Context == "" {
		return hint
	}
	return req.Context + "\n\n" + hint
}

// createDelegateHostInstance lazily seeds the hidden `_delegate_host` global
// workflow definition (idempotent, INSERT OR IGNORE — see
// service.EnsureGlobalDynamicWorkflow for the identical shape) and mints a
// fresh instance under it, scoped to projectID.
func (s *Spawner) createDelegateHostInstance(pool *db.Pool, projectID string) (*model.WorkflowInstance, error) {
	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 0, 1, '{}', ?, ?)`,
		delegateHiddenWorkflow, service.GlobalProjectID,
		"Hidden host for delegate calls from a caller with no bound workflow instance (e.g. a console session)",
		now, now,
	); err != nil {
		return nil, fmt.Errorf("seed hidden workflow: %w", err)
	}

	wi := &model.WorkflowInstance{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		DefProjectID: service.GlobalProjectID,
		WorkflowID:   delegateHiddenWorkflow,
		ScopeType:    "project",
		Status:       model.WorkflowInstanceActive,
	}
	if err := repo.NewWorkflowInstanceRepo(pool, s.config.Clock).Create(wi); err != nil {
		return nil, fmt.Errorf("create host instance: %w", err)
	}
	return wi, nil
}
