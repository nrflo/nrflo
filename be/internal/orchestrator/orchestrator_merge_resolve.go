package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// conflictResolverConfig derives a spawner.Config for the one-off
// conflict-resolver spawn from the run's baseCfg, overriding only what the
// synthetic single-phase workflow needs. Copying baseCfg (rather than
// hand-listing fields) keeps every tool-backing service — AgentSvcReal,
// FindingsSvc, ProjectFindingsSvc, WorkflowSvc, TicketSvc, ArtifactSvc,
// BuildAPIProvider — wired so `agent_*`/`findings_*` tools work in the
// resolver session. RefinerySidecar is explicitly cleared: one-off system
// spawns get no fold sidecar by omission (see spawner/CLAUDE.md § Context Save).
func conflictResolverConfig(
	baseCfg spawner.Config,
	wt *worktreeInfo,
	primaryModel string,
	sysDef *model.SystemAgentDefinition,
	chain []service.AgentChainEntry,
	onRegister func(string, *spawner.Spawner),
	onUnregister func(string),
) spawner.Config {
	cfg := baseCfg
	cfg.Workflows = map[string]spawner.WorkflowDef{
		"_conflict_resolution": {
			Phases: []spawner.PhaseDef{{NodeID: "conflict-resolver", Agent: "conflict-resolver", Layer: 0}},
		},
	}
	cfg.Agents = map[string]spawner.AgentConfig{
		"conflict-resolver": {Model: primaryModel, Timeout: sysDef.Timeout, Chain: chain},
	}
	cfg.ProjectRoot = wt.projectRoot
	cfg.RefinerySidecar = nil
	cfg.OnSessionRegister = onRegister
	cfg.OnSessionUnregister = onUnregister
	return cfg
}

// attemptConflictResolution tries to resolve a merge conflict by spawning the
// conflict-resolver system agent. Returns nil on success (branch merged and
// deleted), or an error if resolution failed or no resolver is configured.
func (o *Orchestrator) attemptConflictResolution(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	wt *worktreeInfo,
	pool *db.Pool,
	mergeError string,
	baseCfg spawner.Config,
) error {
	// Load conflict-resolver system agent definition.
	// API-mode conflict resolution is not yet supported — the resolver needs
	// git/bash tools registered for apirun, so only the CLI variant is used here.
	svc := service.NewSystemAgentDefinitionService(pool, o.clock, service.NewModelService(pool, o.clock))
	sysDef, err := svc.Get("conflict-resolver")
	if err != nil {
		return fmt.Errorf("no conflict-resolver configured: %w", err)
	}
	chain, err := svc.ResolveAgentChain(sysDef)
	if err != nil {
		return fmt.Errorf("conflict-resolver: resolve agent chain: %w", err)
	}
	primaryModel := chain[0].ModelID

	// Broadcast resolving event
	o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictResolving, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"branch":      wt.branchName,
		"merge_error": mergeError,
	}))

	// Construct spawner with synthetic single-phase workflow.
	// Conflict resolution is CLI-only; manifest tools are not used here.
	cfg := conflictResolverConfig(baseCfg, wt, primaryModel, sysDef, chain,
		func(sid string, s *spawner.Spawner) {
			o.mu.Lock()
			if rs, ok := o.runs[wfiID]; ok {
				rs.spawners[sid] = s
			}
			o.mu.Unlock()
		},
		func(sid string) {
			o.mu.Lock()
			if rs, ok := o.runs[wfiID]; ok {
				delete(rs.spawners, sid)
			}
			o.mu.Unlock()
		},
	)
	sp := spawner.New(cfg)

	spawnErr := sp.Spawn(ctx, spawner.SpawnRequest{
		AgentType:          "conflict-resolver",
		NodeID:             "conflict-resolver",
		TicketID:           req.TicketID,
		ProjectID:          req.ProjectID,
		WorkflowName:       "_conflict_resolution",
		WorkflowInstanceID: wfiID,
		ScopeType:          req.ScopeType,
		ExtraVars: map[string]string{
			"BRANCH_NAME":    wt.branchName,
			"DEFAULT_BRANCH": wt.defaultBranch,
			"MERGE_ERROR":    mergeError,
		},
	})
	sp.Close()

	if spawnErr != nil {
		// The resolver session's reporting channel can fail (spawn error, stall,
		// crash) even though it already merged the branch before exiting. Check
		// git ancestry before declaring failure: an already-merged branch is a
		// success regardless of what the session's exit status says.
		if merged, mergedErr := (&service.WorktreeService{}).BranchMerged(wt.projectRoot, wt.defaultBranch, wt.branchName); mergedErr == nil && merged {
			logger.Warn(ctx, "conflict-resolver session reported failure but branch is already merged — treating as success",
				"branch", wt.branchName, "spawn_err", spawnErr)
			o.deleteResolvedBranch(ctx, wt, req, wfiID)
			return nil
		}

		if o.errorSvc != nil {
			o.errorSvc.RecordError(req.ProjectID, "system", wfiID, fmt.Sprintf("merge conflict resolution failed for branch %s: %s", wt.branchName, spawnErr.Error()))
		}
		o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"branch":      wt.branchName,
			"error":       spawnErr.Error(),
		}))
		return fmt.Errorf("conflict resolution failed: %w", spawnErr)
	}

	// Resolution succeeded — delete the feature branch
	o.deleteResolvedBranch(ctx, wt, req, wfiID)
	return nil
}

// deleteResolvedBranch removes the now-merged feature branch and broadcasts
// merge.conflict_resolved. Branch deletion failure is logged but non-fatal.
func (o *Orchestrator) deleteResolvedBranch(ctx context.Context, wt *worktreeInfo, req RunRequest, wfiID string) {
	cmd := exec.Command("git", "branch", "-d", wt.branchName)
	cmd.Dir = wt.projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(ctx, "failed to delete branch after conflict resolution", "branch", wt.branchName, "err", err, "output", strings.TrimSpace(string(out)))
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventMergeConflictResolved, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"branch":      wt.branchName,
	}))
}

// mergeWorktreeOnSuccess merges the run's worktree branch into the default
// branch after all layers complete, attempting automatic conflict resolution
// and falling back to preserving the branch for manual resolution.
func (o *Orchestrator) mergeWorktreeOnSuccess(ctx context.Context, wfiID string, req RunRequest, wt *worktreeInfo, pool *db.Pool, pushAfterMerge bool, baseCfg spawner.Config) {
	wtService := &service.WorktreeService{}
	if err := wtService.MergeAndCleanup(wt.projectRoot, wt.defaultBranch, wt.branchName, wt.worktreePath); err != nil {
		if resolveErr := o.attemptConflictResolution(ctx, wfiID, req, wt, pool, err.Error(), baseCfg); resolveErr != nil {
			// Resolution failed or no resolver configured — fall through to manual resolution
			logger.Error(ctx, "worktree merge failed — branch preserved for manual resolution",
				"branch", wt.branchName, "resolve_err", resolveErr, "merge_err", err)
			o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
				"instance_id":   wfiID,
				"merge_error":   err.Error(),
				"branch":        wt.branchName,
				"worktree_path": wt.worktreePath,
			}))
		} else {
			logger.Info(ctx, "merge conflict resolved automatically", "branch", wt.branchName)
			o.pushIfEnabled(ctx, pushAfterMerge, wt, wfiID, req)
		}
	} else {
		logger.Info(ctx, "worktree merged and cleaned up", "branch", wt.branchName)
		o.pushIfEnabled(ctx, pushAfterMerge, wt, wfiID, req)
	}
}

// pushIfEnabled pushes the default branch to origin after a successful merge,
// if the push_after_merge project setting is enabled. Push failure is logged
// and broadcast but does NOT fail the workflow.
func (o *Orchestrator) pushIfEnabled(ctx context.Context, pushAfterMerge bool, wt *worktreeInfo, wfiID string, req RunRequest) {
	if !pushAfterMerge {
		return
	}

	cmd := exec.Command("git", "push", "origin", wt.defaultBranch)
	cmd.Dir = wt.projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Error(ctx, "git push failed after merge", "branch", wt.defaultBranch, "err", err, "output", strings.TrimSpace(string(out)))
		o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowPushFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"branch":      wt.defaultBranch,
			"error":       strings.TrimSpace(string(out)),
		}))
		if o.errorSvc != nil {
			o.errorSvc.RecordError(req.ProjectID, "system", wfiID, fmt.Sprintf("git push failed for branch %s: %s", wt.defaultBranch, strings.TrimSpace(string(out))))
		}
	} else {
		logger.Info(ctx, "pushed default branch to origin", "branch", wt.defaultBranch)
		o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowPushed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"branch":      wt.defaultBranch,
		}))
	}
}
