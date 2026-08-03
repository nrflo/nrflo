package spawner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
		wfi, err = s.createDelegateHostInstance(pool, callerSession.ProjectID, callerSession.ID)
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

	// Persist the durable delegations row (migration 000216) synchronously,
	// before returning, so GetDelegation resolves the delegation as
	// {status:running} for the whole time the workers run. Each worker Spawn
	// blocks until its agent completes (consult pattern), so the final
	// session-id list only exists after wg.Wait; without this seed the
	// bounded-wait poll would hit "unknown delegation" on its first tick and
	// abort.
	run, err := s.createDelegationRecord(pool, wfi.ID, callerSession.ProjectID, callerSessionID, req.Tier, req.Brief, len(items))
	if err != nil {
		return "", fmt.Errorf("delegate: seed tracking: %w", err)
	}

	// Degrades to in-place ("" worktreePath) on any ineligibility or setup
	// failure — see prepareDelegateWorktree.
	worktreePath, branchName, baseCommit, err := s.prepareAndPersistDelegateWorktree(pool, sysDef, isHost, callerSession.ProjectID, run.delegationID)
	if err != nil {
		return "", fmt.Errorf("delegate: persist worktree: %w", err)
	}

	s.broadcast(ws.EventDelegateStarted, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
		"caller_session_id": callerSessionID,
		"delegation_id":     run.delegationID,
		"tier":              req.Tier,
		"fanout":            len(items),
	})

	// Detached: the caller's tool ctx must not cancel the workers (async
	// contract, matches startChildRun's context.Background() start).
	go s.runDelegateFanout(wfi, callerSession, tierAgentID, sysDef, chain, req, items, run, isHost, worktreePath, branchName, baseCommit)

	return delegateStatusJSON(run.delegationID, "running", nil, nil), nil
}

// runDelegateFanout spawns one worker per item concurrently (each Spawn blocks
// until its worker finishes), then records the final session-id list with
// done=true so GetDelegation flips out of "running", completes a hidden host
// instance, and broadcasts completion. chain is the worker's full resolved
// tier fallback chain (index 0 = primary, driving the initial spawn).
func (s *Spawner) runDelegateFanout(wfi *model.WorkflowInstance, callerSession *model.AgentSession, tierAgentID string, sysDef *model.SystemAgentDefinition, chain []service.AgentChainEntry, req apirun.DelegateRequest, items []string, run *delegateRun, isHost bool, worktreePath, branchName, baseCommit string) {
	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(i int, item string) {
			defer wg.Done()
			s.spawnDelegateWorker(wfi, callerSession, tierAgentID, sysDef, chain, req, item, run, i, worktreePath)
		}(i, item)
	}
	wg.Wait()

	// Commit is server-owned, not prompt-owned: workers are told never to
	// commit themselves, so a worker that failed or timed out still leaves
	// its partial work recoverable on the branch.
	s.finalizeDelegateWorktree(s.pool(), callerSession.ProjectID, run.delegationID, worktreePath, branchName, baseCommit, briefHead(req.Brief))

	// Hidden host instances (run-less console callers) never re-enter the
	// orchestrator, so mark this one terminal here instead of leaving it
	// "active" forever. The delegation row + worker findings stay readable
	// after completion — GetDelegation still resolves them.
	if isHost {
		repo.NewWorkflowInstanceRepo(s.pool(), s.config.Clock).UpdateStatus(wfi.ID, model.WorkflowInstanceCompleted) //nolint:errcheck
	}

	if err := s.markFanoutDone(s.pool(), run.delegationID); err != nil {
		s.broadcast(ws.EventDelegateFailed, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
			"caller_session_id": callerSession.ID,
			"delegation_id":     run.delegationID,
			"error":             err.Error(),
		})
		return
	}
	s.broadcast(ws.EventDelegateCompleted, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
		"caller_session_id": callerSession.ID,
		"delegation_id":     run.delegationID,
		"branch":            branchName,
	})
}

// spawnDelegateWorker spawns a single hidden `_delegate` worker under wfi in
// its own one-off child Spawner (mirrors consult.go's per-call sp). The
// worker's AgentConfig carries the full chain (index 0 = primary, driving
// Model/ExecutionMode/ReasoningEffort for the initial spawn) so a build-time
// or HARD failure can advance to a fallback entry. Returns the registered
// session id ("" on spawn failure) and the spawn error message ("" on
// success), which GetDelegation surfaces for session-less workers.
func (s *Spawner) spawnDelegateWorker(wfi *model.WorkflowInstance, callerSession *model.AgentSession, tierAgentID string, sysDef *model.SystemAgentDefinition, chain []service.AgentChainEntry, req apirun.DelegateRequest, item string, run *delegateRun, slot int, worktreePath string) (string, string) {
	var mu sync.Mutex
	var sid string
	var ownSp *Spawner
	delegateRegister, delegateUnregister := s.childSessionHooks(func(registeredSID string, child *Spawner) {
		mu.Lock()
		// Only this worker's own (re)spawns: grandchild registrations (the
		// worker's nested delegate fanout) bubble up here too and must not
		// overwrite the tracked session id.
		isOwn := child == ownSp
		if isOwn {
			sid = registeredSID
		}
		mu.Unlock()
		// Fire the DB write outside mu (lock-order discipline, mirrors
		// registerTerminalSignal) so the slot is linkable from
		// delegations.worker_session_ids while the worker is still running,
		// not only after Spawn returns.
		if isOwn {
			s.recordWorkerSlot(s.pool(), run.delegationID, slot, registeredSID, "") //nolint:errcheck
		}
	})

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
		ProjectRoot:        delegateWorkerProjectRoot(s.config.ProjectRoot, worktreePath),
		WSHub:              s.config.WSHub,
		Pool:               s.config.Pool,
		Clock:              s.config.Clock,
		ClaudeSettingsJSON: s.config.ClaudeSettingsJSON,
		ExternalMCPServers: s.config.ExternalMCPServers,
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
		// The worker's own spawner carries this delegation row's persisted
		// depth so its buildAPIRegistry (and any delegate it makes) sees the
		// correct per-chain depth — sourced from the DB, not threaded
		// in-memory, which is what makes the console path (a fresh Spawner
		// per call) resolve depth correctly too.
		DelegateDepth:       run.depth,
		OnSessionRegister:   delegateRegister,
		OnSessionUnregister: delegateUnregister,
	})
	defer sp.Close()
	mu.Lock()
	ownSp = sp
	mu.Unlock()

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
	errMsg := ""
	if spawnErr != nil {
		errMsg = spawnErr.Error()
		s.broadcast(ws.EventDelegateFailed, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
			"caller_session_id": callerSession.ID,
			"delegation_id":     run.delegationID,
			"tier":              req.Tier,
			"error":             errMsg,
		})
	}

	mu.Lock()
	resultSID := sid
	mu.Unlock()

	// Finalizes the spawn error field (registration already wrote the sid
	// above); re-writes the same resultSID so it cannot blank an sid already
	// recorded.
	if err := s.recordWorkerSlot(s.pool(), run.delegationID, slot, resultSID, errMsg); err != nil {
		s.broadcast(ws.EventDelegateFailed, callerSession.ProjectID, callerSession.TicketID, wfi.WorkflowID, map[string]interface{}{
			"caller_session_id": callerSession.ID,
			"delegation_id":     run.delegationID,
			"error":             err.Error(),
		})
	}
	return resultSID, errMsg
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
