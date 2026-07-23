package spawner

import (
	"context"
	"time"
)

// stepCheckCommandTimeout is the per-command timeout for a stepwise step's
// `checks`. A var (not const) so tests can shorten it, mirroring
// validationCommandTimeout.
var stepCheckCommandTimeout = 60 * time.Second

// stepCheckTailSize is the captured-output tail size for step checks —
// smaller than validation's 64KB because this tail is embedded in the
// check_failed rejection message the agent reads back.
const stepCheckTailSize = 8 * 1024

// stepChecksTotalBudget bounds the wall time of an entire checks run. Not
// decorative: for cli_interactive/codex agents complete_step arrives over
// the agent mcp bridge, whose socket client has a 5-minute default read
// deadline (be/internal/client/client.go), so an unbounded run of many 60s
// commands could time the agent's tool call out while the server kept
// running.
var stepChecksTotalBudget = 4 * time.Minute

// RunStepChecks implements apirun.StepSession for per-step checks: runs cmds
// in the session's own proc.workDir/env via the shared shell executor. An
// unknown session or empty cmds is a no-op pass ((-1, 0, "", nil)) — checks
// never block an advance the spawner cannot run. If the total budget expires
// while the caller's ctx is still live, the budget timeout is converted into
// a normal check failure (rather than an opaque error) so the agent gets a
// check_failed rejection instead of a tool error.
func (s *Spawner) RunStepChecks(ctx context.Context, sessionID string, cmds []string) (failedIdx, exitCode int, outputTail string, err error) {
	if len(cmds) == 0 {
		return -1, 0, "", nil
	}
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return -1, 0, "", nil
	}

	budgetCtx, cancel := context.WithTimeout(ctx, stepChecksTotalBudget)
	defer cancel()

	idx, code, tail, runErr := s.runShellCommands(budgetCtx, proc, cmds, stepCheckCommandTimeout, stepCheckTailSize)
	if runErr != nil && ctx.Err() == nil {
		// Our own budget expired, not an orchestrator shutdown — surface it
		// as a check failure so the agent gets a normal rejection.
		return idx, -1, tail + "\n[step checks exceeded the total budget]", nil
	}
	return idx, code, tail, runErr
}
