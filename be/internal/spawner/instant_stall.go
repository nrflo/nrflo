package spawner

import (
	"context"
	"fmt"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// checkInstantStall detects agents that exit 0 suspiciously fast (< 1 minute)
// with minimal output (<=1 message). These are "instant stall" scenarios where
// the agent started, produced at most one message, and exited without doing real work.
// On detection, the old session is marked as continued/instant_stall and the agent
// is relaunched via the continuation mechanism, consuming the shared stall restart budget.
func (s *Spawner) checkInstantStall(ctx context.Context, proc *processInfo, req SpawnRequest) {
	// Guard: Claude CLI only (must support resume for continuation)
	cliName, _ := parseModelID(proc.modelID)
	adapter, err := GetCLIAdapter(cliName)
	if err != nil || adapter == nil || !adapter.SupportsResume() {
		return
	}

	// Guard: elapsed < 1 minute
	if proc.elapsed >= 1*time.Minute {
		return
	}

	// Guard: stall restart budget not exhausted
	if proc.stallRestartCount >= maxStallRestarts {
		return
	}

	// Guard: message count <= 1 (messages already flushed by handleCompletion)
	pool := s.pool()
	if pool == nil {
		return
	}
	msgRepo := repo.NewAgentMessagePoolRepo(pool, s.config.Clock)
	msgCount, err := msgRepo.CountBySession(proc.sessionID)
	if err != nil || msgCount > 1 {
		return
	}

	// Instant stall detected — override the already-registered pass result
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	sessionRepo.UpdateResult(proc.sessionID, "continue", "instant_stall")
	sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)

	proc.stallRestartCount++
	proc.finalStatus = "CONTINUE"

	s.broadcast(ws.EventAgentInstantStallRestart, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id":    proc.sessionID,
		"agent_type":    proc.agentType,
		"elapsed":       fmt.Sprintf("%.0fs", proc.elapsed.Seconds()),
		"message_count": msgCount,
		"stall_count":   proc.stallRestartCount,
	})

	logger.Warn(ctx, "instant stall detected: agent exited too fast with minimal output",
		"agent_type", proc.agentType, "session_id", proc.sessionID,
		"elapsed", proc.elapsed, "message_count", msgCount,
		"stall_restart_count", proc.stallRestartCount)
}
