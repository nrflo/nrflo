package spawner

import (
	"context"
	"encoding/json"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/ws"
)

const (
	contextSaveTimeout = 3 * time.Minute
	killGracePeriod    = 5 * time.Second
	maxMessageChars    = 120000
)

// initiateContextSave handles the low-context save flow:
// 1. Kill the running agent
// 2. Flush messages
// 3. Save context via the context-saver system agent
//
// processDoneCh is the original process's done channel (closed by the wait goroutine).
// completeCh is the replacement channel; closed when the full flow finishes, signaling monitorAll.
func (s *Spawner) initiateContextSave(ctx context.Context, proc *processInfo, req SpawnRequest, processDoneCh, completeCh chan struct{}) {
	defer close(completeCh)

	logger.Warn(ctx, "low context detected", "context_left", proc.contextLeft, "session_id", proc.sessionID)

	// 1. Kill the running agent: SIGTERM → wait → SIGKILL
	proc.backend.Kill(ctx, proc, syscall.SIGTERM)
	select {
	case <-processDoneCh:
		// Original process exited
	case <-time.After(killGracePeriod):
		proc.backend.Kill(ctx, proc, syscall.SIGKILL)
		<-processDoneCh
	}

	// 2. Flush messages from the killed process
	s.saveMessages(proc)

	// 3. Save context via the context-saver system agent.
	s.contextSaveViaAgent(ctx, proc, req)
}

// contextSaveViaAgent uses a system agent, running the dying agent's own
// inherited model (see contextSaverModel), to summarize the killed agent's
// message history and save to_resume findings. Works for all CLI types.
//
// When a fresh autonomous refinery slot digest already exists for this
// session's (workflow_instance_id, node_id) slot (see digest_freshness.go),
// the context-saver spawn and its context_saving broadcast are skipped
// entirely — fetchPreviousDataAndReason will read that same digest at
// relaunch-prompt-assembly time, so nothing is lost. Otherwise this falls
// back to the existing agent-save path unchanged.
func (s *Spawner) contextSaveViaAgent(ctx context.Context, proc *processInfo, req SpawnRequest) {
	if s.stepwiseDefFor(proc.agentType, req.ProjectID, req.WorkflowName) {
		logger.Info(ctx, "stepwise mode: cursor is the save, skipping context-saver spawn", "session_id", proc.sessionID)
	} else if s.freshDigestAfterForcedFold(ctx, proc) {
		logger.Info(ctx, "digest rotation: using slot digest, skipping context-saver spawn", "session_id", proc.sessionID)
	} else {
		// Broadcast context_saving event
		if s.config.WSHub != nil {
			s.config.WSHub.Broadcast(ws.NewEvent(ws.EventAgentContextSaving, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
				"session_id": proc.sessionID,
				"agent_type": proc.agentType,
			}))
		}

		// Spawn context-saver system agent
		saved := s.spawnContextSaver(ctx, proc, req)

		// Check if to_resume findings were actually saved
		findingsSaved := s.checkToResumeFindings(ctx, proc)
		if saved && !findingsSaved {
			logger.Warn(ctx, "context-saver completed but to_resume findings not saved, previous data will be empty on relaunch", "session_id", proc.sessionID)
		}
	}

	// Register stop
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	logger.Info(ctx, "context save flow complete, relaunching", "session_id", proc.sessionID)
}

// freshDigestAfterForcedFold reports whether a fresh slot digest now covers
// this session, forcing one bounded refinery fold first.
//
// Checking the digest alone loses a race the refinery cannot win on its own:
// autonomous folds are debounced (>=30s), so a session that burns from
// refinery_fold_start_context_pct down to the relaunch threshold inside one
// debounce window reaches this point with no digest, spawns a context-saver,
// and only then does the scheduled fold land — producing the handoff the
// relaunch actually reads while the saver's work is discarded. Forcing the
// fold makes the outcome deterministic: a bounded local-model call decides it
// instead of timing. Nil sidecar or a shut gate leaves the answer unchanged,
// so the context-saver fallback stays intact.
func (s *Spawner) freshDigestAfterForcedFold(ctx context.Context, proc *processInfo) bool {
	if s.config.RefinerySidecar != nil {
		s.config.RefinerySidecar.FoldNow(proc.sessionID)
	}
	_, ok := freshSlotDigest(s.pool(), s.config.Clock, proc.workflowInstanceID, proc.nodeID, proc.startTime)
	if ok {
		logger.Info(ctx, "refinery slot digest covers this session", "session_id", proc.sessionID, "node_id", proc.nodeID)
	}
	return ok
}

// checkToResumeFindings checks whether the session has to_resume findings after context save.
// Returns true if the to_resume key was found in the session's findings.
func (s *Spawner) checkToResumeFindings(ctx context.Context, proc *processInfo) bool {
	pool := s.pool()
	if pool == nil {
		logger.Error(ctx, "no database pool for findings check", "session_id", proc.sessionID)
		return false
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	findings, err := findingRepo.GetOwn("session", proc.sessionID)
	if err != nil {
		logger.Error(ctx, "failed to query findings", "err", err, "session_id", proc.sessionID)
		return false
	}

	if len(findings) == 0 {
		logger.Warn(ctx, "no findings saved by context-saver agent", "session_id", proc.sessionID)
		return false
	}

	rawVal, ok := findings["to_resume"]
	if !ok {
		logger.Warn(ctx, "findings saved but to_resume key missing", "keys_count", len(findings), "session_id", proc.sessionID)
		return false
	}

	var str string
	if json.Unmarshal(rawVal, &str) != nil || str == "" {
		logger.Warn(ctx, "to_resume key present but empty or non-string", "session_id", proc.sessionID)
		return false
	}

	logger.Info(ctx, "to_resume findings saved", "bytes", len(str), "session_id", proc.sessionID)
	return true
}
