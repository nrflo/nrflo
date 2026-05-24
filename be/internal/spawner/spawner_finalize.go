package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"be/internal/logger"
	"be/internal/repo"
)

// printStatus logs status for all running/completed agents
func (s *Spawner) printStatus(running, completed []*processInfo, phase string) {
	for _, proc := range running {
		elapsed := time.Since(proc.startTime).Round(time.Second)

		proc.messagesMutex.Lock()
		lastMsg := proc.lastMessage
		proc.messagesMutex.Unlock()
		if lastMsg != "" {
			if len(lastMsg) > 80 {
				lastMsg = lastMsg[:77] + "..."
			}
		}

		ctx := logger.WithTrx(context.Background(), proc.trx)
		logger.Info(ctx, "agent status", "phase", phase, "model", proc.modelID, "elapsed", elapsed, "last_msg", lastMsg)
	}

	for _, proc := range completed {
		ctx := logger.WithTrx(context.Background(), proc.trx)
		logger.Info(ctx, "agent status", "phase", phase, "model", proc.modelID, "status", proc.finalStatus, "duration", proc.elapsed.Round(time.Second))
	}
}

// finalizePhase completes the phase after all agents finish.
// Uses pass_count >= 1 semantics: at least one PASS is required for layer success.
// All-skipped counts as success (continue to next layer).
// Returns CallbackError if any agent completed with CALLBACK status.
func (s *Spawner) finalizePhase(ctx context.Context, completed []*processInfo, req SpawnRequest, phase string) error {
	// Clean up per-session tracking for completed sessions.
	cleanupBroadcastCoalescing(completed)
	s.unregisterSessionProcs(completed)

	for _, proc := range completed {
		logger.Info(ctx, "agent result", "phase", phase, "model", proc.modelID, "status", proc.finalStatus, "duration", proc.elapsed.Round(time.Second))
	}

	passCount := 0
	skippedCount := 0
	callbackCount := 0
	var callbackProc *processInfo
	for _, proc := range completed {
		switch proc.finalStatus {
		case "PASS":
			passCount++
		case "SKIPPED":
			skippedCount++
		case "CALLBACK":
			callbackCount++
			// Track the callback proc (if multiple, we'll pick lowest level in orchestrator)
			callbackProc = proc
		}
	}

	// Callback detected — read callback fields from session findings and signal orchestrator
	if callbackCount > 0 {
		cb := s.readCallbackFindings(callbackProc)
		cb.AgentType = req.AgentType
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "CALLBACK", "callback_level", cb.Level)
		return cb
	}

	// All skipped = success (continue to next layer)
	if skippedCount == len(completed) {
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "SKIPPED")
		return nil
	}

	// At least one pass = success
	if passCount >= 1 {
		logger.Info(ctx, "phase finalized", "phase", phase, "result", "PASS", "pass_count", passCount, "total", len(completed))
		return nil
	}

	// No passes = fail

	var failedModels []string
	for _, proc := range completed {
		if proc.finalStatus != "PASS" && proc.finalStatus != "SKIPPED" {
			failedModels = append(failedModels, proc.modelID)
		}
	}
	logger.Error(ctx, "phase finalized", "phase", phase, "result", "FAIL", "failed", strings.Join(failedModels, ", "))
	return fmt.Errorf("phase %s failed", phase)
}

// readCallbackFindings reads callback fields from agent session findings.
func (s *Spawner) readCallbackFindings(proc *processInfo) *CallbackError {
	pool := s.pool()
	if pool == nil {
		return &CallbackError{Mode: "layer"}
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	raw, err := findingRepo.GetOwn("session", proc.sessionID)
	if err != nil {
		return &CallbackError{Mode: "layer"}
	}

	level := 0
	if v, ok := raw["callback_level"]; ok {
		var n float64
		if json.Unmarshal(v, &n) == nil {
			level = int(n)
		}
	}

	instructions := ""
	if v, ok := raw["callback_instructions"]; ok {
		json.Unmarshal(v, &instructions) //nolint:errcheck
	}

	mode := "layer"
	if v, ok := raw["callback_mode"]; ok {
		var ms string
		if json.Unmarshal(v, &ms) == nil && ms != "" {
			mode = ms
		}
	}

	var targetAgent string
	if v, ok := raw["callback_target"]; ok {
		json.Unmarshal(v, &targetAgent) //nolint:errcheck
	}

	var chain []string
	if v, ok := raw["callback_chain"]; ok {
		var arr []string
		if json.Unmarshal(v, &arr) == nil {
			chain = arr
		} else {
			var str string
			if json.Unmarshal(v, &str) == nil {
				for _, part := range strings.Split(str, ",") {
					if p := strings.TrimSpace(part); p != "" {
						chain = append(chain, p)
					}
				}
			}
		}
	}

	return &CallbackError{
		Level:        level,
		Instructions: instructions,
		Mode:         mode,
		TargetAgent:  targetAgent,
		Chain:        chain,
	}
}
