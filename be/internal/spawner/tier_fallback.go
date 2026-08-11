package spawner

import (
	"context"
	"errors"
	"fmt"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// errProviderBuild sentinel-wraps a build-time provider-construct failure
// (missing credentials, api_mode_disabled, unsupported mode/model for the
// selected registry row) so isProviderBuildError can classify it distinctly
// from a structural spawn error (template load, node not found, unknown CLI
// adapter, ...) — the latter must NEVER advance the chain.
var errProviderBuild = errors.New("provider build failure")

// wrapProviderBuildErr wraps a non-nil err as a build-time provider failure.
func wrapProviderBuildErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", errProviderBuild, err)
}

// isProviderBuildError reports whether err is a build-time provider-construct
// failure eligible for a clean-restart chain advance.
func isProviderBuildError(err error) bool {
	return errors.Is(err, errProviderBuild)
}

// chainEntryModelID derives the "cli:model" form spawnSingle expects from a
// resolved chain entry.
func chainEntryModelID(entry service.AgentChainEntry) string {
	return cliForProvider(entry.Provider) + ":" + entry.ModelID
}

// spawnEntryWithBuildFallback spawns starting at chain[0], advancing over
// build-time provider-construct failures (isProviderBuildError) to the next
// chain entry, capped at len(chain)-1. A structural error is returned
// immediately without advancing. modelID is used verbatim when chain is
// empty (main workflow-phase agents, which never carry a resolved chain) so
// behavior there is byte-identical to a bare spawnSingle call. Returns the
// winning processInfo and the chain position it landed on (0 for the
// no-chain case).
func (s *Spawner) spawnEntryWithBuildFallback(ctx context.Context, req SpawnRequest, modelID, phase, wfiID string, chain []service.AgentChainEntry) (*processInfo, int, error) {
	if len(chain) == 0 {
		proc, err := s.spawnSingle(ctx, req, modelID, phase, wfiID)
		if err == nil {
			s.recordResolvedSpawn(proc, chain, 0)
		}
		return proc, 0, err
	}

	var lastErr error
	for pos := 0; pos < len(chain); pos++ {
		entry := chain[pos]
		if pos < len(chain)-1 && s.skipAPIEntryNoCredentials(ctx, entry, req.ProjectID) {
			logger.Info(ctx, "tier fallback: skipping api entry, credentials unavailable",
				"agent_type", req.AgentType, "chain_pos", pos, "provider", entry.Provider)
			continue
		}
		entryReq := req
		entryReq.ExecutionModeOverride = entry.ExecutionMode
		entryReq.ReasoningEffortOverride = entry.ReasoningEffort

		proc, err := s.spawnSingle(ctx, entryReq, chainEntryModelID(entry), phase, wfiID)
		if err == nil {
			s.recordResolvedSpawn(proc, chain, pos)
			return proc, pos, nil
		}
		lastErr = err
		if !isProviderBuildError(err) {
			return nil, pos, err
		}
		logger.Warn(ctx, "tier fallback: build-time provider failure, advancing chain",
			"agent_type", req.AgentType, "chain_pos", pos, "provider", entry.Provider, "err", err)
	}
	return nil, len(chain) - 1, fmt.Errorf("tier fallback: chain exhausted: %w", lastErr)
}

// skipAPIEntryNoCredentials reports whether an api-mode chain entry is
// statically doomed: its provider's credentials do not resolve, so the spawn
// could only fail at build time. api-via-cli anthropic entries route through
// the Claude CLI and need no API key, so they are never skipped.
func (s *Spawner) skipAPIEntryNoCredentials(ctx context.Context, entry service.AgentChainEntry, projectID string) bool {
	if entry.ExecutionMode != "api" {
		return false
	}
	if s.config.APIViaCLI && entry.Provider == "anthropic" {
		return false
	}
	if s.config.HasAPICredentials != nil {
		return !s.config.HasAPICredentials(ctx, entry.Provider, projectID)
	}
	pool := s.pool()
	if pool == nil {
		return false
	}
	return !service.HasAPICredentials(ctx, pool, s.config.Clock, entry.Provider, projectID)
}

// shouldAdvanceChain is the monotonic advance guard consulted by monitorAll's
// FAIL branch: true only when the dying proc actually hit a HARD provider
// failure (never rate-limit — that stays in-band) and the chain has at least
// one more entry beyond its current position. Every relaunch path (fallback
// or plain continuation) MUST reset hardProviderFail on the new proc, or a
// same-model retry could re-trigger an advance from a stale flag.
func shouldAdvanceChain(proc *processInfo) (nextPos int, entry service.AgentChainEntry, ok bool) {
	if !proc.hardProviderFail {
		return 0, service.AgentChainEntry{}, false
	}
	if len(proc.chain) == 0 || proc.chainPos >= len(proc.chain)-1 {
		return 0, service.AgentChainEntry{}, false
	}
	nextPos = proc.chainPos + 1
	return nextPos, proc.chain[nextPos], true
}

// chainExhausted reports whether proc's chain actually advanced (chainPos>0)
// and then ran out (chainPos at the last entry). A proc that never advanced
// (nil chain or a length-1 chain, chainPos still 0) is NOT exhausted — its
// completion path's classified reason must survive untouched.
func chainExhausted(proc *processInfo) bool {
	return proc.chainPos > 0 && proc.chainPos >= len(proc.chain)-1
}

// tryChainFallback is monitorAll's FAIL-branch entry point: it consults
// shouldAdvanceChain and, when eligible, mid-work-saves and relaunches under
// the next entry via relaunch (relaunchFallbackAndRegister, which shares the
// same registry/ledger/sidecar/cost bookkeeping relaunchAndRegister uses).
// advanced=false means the caller should fall through to the ordinary
// FAIL/maxFailRestarts handling unchanged; advanced=true with a nil proc
// means chain exhaustion (or the relaunch itself failed) — the caller must
// treat proc as terminally completed.
func (s *Spawner) tryChainFallback(ctx context.Context, proc *processInfo, req SpawnRequest, relaunch func(*processInfo, service.AgentChainEntry, int) (*processInfo, error)) (newProc *processInfo, advanced bool) {
	nextPos, entry, ok := shouldAdvanceChain(proc)
	if !ok {
		if chainExhausted(proc) {
			s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
				proc.sessionID, proc.agentID, "fail", "chain_exhausted", proc.modelID)
		}
		return nil, false
	}

	s.saveForFallback(ctx, proc, req)
	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		sessionRepo.UpdateResult(proc.sessionID, "continue", "provider_fallback")
		sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
	}
	proc.finalStatus = "CONTINUE"

	newProc, err := relaunch(proc, entry, nextPos)
	if err != nil {
		logger.Error(ctx, "failed to relaunch after tier fallback", "model", proc.modelID, "err", err)
		return nil, true
	}
	return newProc, true
}

// saveForFallback best-effort context-saves a mid-work agent before it
// advances to the next chain entry. The process is already dead by the time
// monitorAll's FAIL branch observes it, so unlike initiateContextSave there is
// no kill step — this mirrors only its step 3 (save via context-saver agent).
func (s *Spawner) saveForFallback(ctx context.Context, proc *processInfo, req SpawnRequest) {
	s.contextSaveViaAgent(ctx, proc, req)
}

// relaunchForFallback spawns a new agent under the next chain entry
// (cross-mode allowed), preserving workflowInstanceID/nodeID and the same
// continuation bookkeeping relaunchForContinuation applies, then broadcasts
// the provider-fallback degrade event.
func (s *Spawner) relaunchForFallback(ctx context.Context, oldProc *processInfo, req SpawnRequest, phase string, entry service.AgentChainEntry, nextPos int) (*processInfo, error) {
	ancestorID := oldProc.ancestorSessionID
	if ancestorID == "" {
		ancestorID = oldProc.sessionID
	}

	fallbackReq := req
	fallbackReq.ExecutionModeOverride = entry.ExecutionMode
	fallbackReq.ReasoningEffortOverride = entry.ReasoningEffort

	newProc, err := s.spawnSingle(ctx, fallbackReq, chainEntryModelID(entry), phase, oldProc.workflowInstanceID)
	if err != nil {
		return nil, err
	}

	applyProactiveRotationCarry(ctx, oldProc, newProc, ancestorID)
	newProc.restartThreshold = oldProc.restartThreshold
	newProc.maxFailRestarts = oldProc.maxFailRestarts
	newProc.failRestartCount = oldProc.failRestartCount
	newProc.stallRestartCount = oldProc.stallRestartCount
	newProc.stallStartTimeout = oldProc.stallStartTimeout
	newProc.stallRunningTimeout = oldProc.stallRunningTimeout
	newProc.validationCommands = oldProc.validationCommands
	newProc.workDir = oldProc.workDir
	newProc.rateLimitRetryCount = oldProc.rateLimitRetryCount
	newProc.rateLimitTotalWait = oldProc.rateLimitTotalWait
	newProc.rateLimitConfig = oldProc.rateLimitConfig
	newProc.chain = oldProc.chain
	newProc.chainPos = nextPos
	newProc.hardProviderFail = false
	s.recordResolvedSpawn(newProc, oldProc.chain, nextPos)

	if pool := s.pool(); pool != nil {
		sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
		sessionRepo.UpdateAncestorSession(newProc.sessionID, ancestorID)
		sessionRepo.UpdateRestartCount(newProc.sessionID, newProc.restartCount)
	}

	s.copyFindingsForContinuation(ctx, oldProc.sessionID, newProc.sessionID)

	fromProvider, fromMode := "", ""
	if oldProc.chainPos >= 0 && oldProc.chainPos < len(oldProc.chain) {
		fromProvider = oldProc.chain[oldProc.chainPos].Provider
		fromMode = oldProc.chain[oldProc.chainPos].ExecutionMode
	}
	s.broadcast(ws.EventAgentProviderFallback, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id":     oldProc.sessionID,
		"new_session_id": newProc.sessionID,
		"agent_type":     req.AgentType,
		"from_provider":  fromProvider,
		"to_provider":    entry.Provider,
		"from_mode":      fromMode,
		"to_mode":        entry.ExecutionMode,
		"chain_pos":      nextPos,
	})

	logger.Info(ctx, "tier fallback relaunch", "old_session", oldProc.sessionID, "new_session", newProc.sessionID,
		"chain_pos", nextPos, "model", newProc.modelID, "to_provider", entry.Provider, "to_mode", entry.ExecutionMode)

	return newProc, nil
}
