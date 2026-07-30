package spawner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/repo"
)

const validationTailSize = 64 * 1024 // 64KB

// findingKeyValidationFailure is the findings key written on a validation
// command failure and read back by the restart-feedback prepend seam.
// validationFindingActorID is the actor.ID stamped on that write — the gate
// that distinguishes a genuine validation-failure finding from a copy
// forwarded by copyFindingsForContinuation (actor ID "continuation").
const (
	findingKeyValidationFailure = "validation_failure"
	validationFindingActorID    = "validation"
)

// validationCommandTimeout controls the per-command execution timeout.
// Tests may override this package-level var to use shorter durations.
var validationCommandTimeout = 5 * time.Minute

// runValidationCommands runs each command in proc.validationCommands sequentially.
// Returns (failedIdx, exitCode, outputTail, err):
//   - failedIdx=-1 → all commands passed
//   - failedIdx>=0 → that command exited non-zero; exitCode and outputTail are set
//   - err != nil → parent context was cancelled; caller should not override result
func (s *Spawner) runValidationCommands(ctx context.Context, proc *processInfo) (failedIdx, exitCode int, outputTail string, err error) {
	return s.runShellCommands(ctx, proc, proc.validationCommands, validationCommandTimeout, validationTailSize)
}

// runShellCommands runs each command in cmds sequentially in proc.workDir,
// tracking `$ <cmd>` / `exit=N (dur)` messages under the "validation"
// category (shared by whole-agent validation_commands and per-step checks).
// Returns (failedIdx, exitCode, outputTail, err):
//   - failedIdx=-1 → all commands passed
//   - failedIdx>=0 → that command exited non-zero; exitCode and outputTail are set
//   - err != nil → parent context was cancelled; caller should not override result
func (s *Spawner) runShellCommands(ctx context.Context, proc *processInfo, cmds []string, perCmdTimeout time.Duration, tailBytes int) (failedIdx, exitCode int, outputTail string, err error) {
	env := buildValidationEnv(proc)

	for i, cmd := range cmds {
		s.TrackMessage(proc, fmt.Sprintf("$ %s", cmd), "validation")
		s.saveMessages(proc)

		start := s.config.Clock.Now()
		cmdCtx, cancel := context.WithTimeout(ctx, perCmdTimeout)
		code, tail, runErr := runOneValidationCommand(cmdCtx, cmd, proc.workDir, env, tailBytes)
		cancel()

		dur := s.config.Clock.Now().Sub(start)
		s.TrackMessage(proc, fmt.Sprintf("exit=%d (%s)", code, dur.Round(time.Millisecond)), "validation")
		s.saveMessages(proc)

		if runErr != nil && ctx.Err() != nil {
			return i, 0, "", ctx.Err()
		}
		if code != 0 {
			return i, code, tail, nil
		}
	}
	return -1, 0, "", nil
}

// runOneValidationCommand executes a single shell command and returns its exit code,
// a tail of combined stdout+stderr (at most tailBytes), and any context error.
func runOneValidationCommand(ctx context.Context, cmd, workDir string, env []string, tailBytes int) (int, string, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	if workDir != "" {
		c.Dir = workDir
	}
	c.Env = env

	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	runErr := c.Run()

	code := 0
	if c.ProcessState != nil {
		code = c.ProcessState.ExitCode()
	} else if runErr != nil {
		code = 1
	}

	out := buf.Bytes()
	if len(out) > tailBytes {
		out = out[len(out)-tailBytes:]
	}

	if runErr != nil && ctx.Err() != nil {
		return code, string(out), ctx.Err()
	}
	return code, string(out), nil
}

// writeValidationFailureFinding persists a validation_failure finding on the session scope.
func (s *Spawner) writeValidationFailureFinding(proc *processInfo, idx, exitCode int, outputTail string) {
	pool := s.pool()
	if pool == nil {
		return
	}

	payload, marshalErr := json.Marshal(map[string]interface{}{
		"command":       proc.validationCommands[idx],
		"command_index": idx,
		"exit_code":     exitCode,
		"output_tail":   outputTail,
	})
	if marshalErr != nil {
		logger.Warn(context.Background(), "validation: failed to marshal finding", "session", proc.sessionID, "err", marshalErr)
		return
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	denorm := repo.Denorm{
		ProjectID:          proc.projectID,
		WorkflowInstanceID: proc.workflowInstanceID,
		AgentType:          proc.agentType,
		ModelID:            proc.modelID,
	}
	actor := repo.Actor{Source: "system", ID: validationFindingActorID}

	if upsertErr := findingRepo.Upsert("session", proc.sessionID, findingKeyValidationFailure, json.RawMessage(payload), denorm, actor); upsertErr != nil {
		logger.Warn(context.Background(), "validation: failed to write finding", "session", proc.sessionID, "err", upsertErr)
		return
	}

	logger.Warn(logger.WithTrx(context.Background(), proc.trx), "validation command failed",
		"session", proc.sessionID, "command", proc.validationCommands[idx], "exit_code", exitCode,
		"output_first_line", firstLine(outputTail, 200))
}

// firstLine returns the first line of s, capped to at most maxLen bytes on a
// UTF-8 rune boundary.
func firstLine(s string, maxLen int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return foldfmt.CapBytes(s, maxLen)
}

// buildValidationEnv returns proc.env with session-specific credentials stripped.
// NRFLO_AGENT_TOKEN and NRF_SESSION_ID must not be passed to validation commands
// since they are single-use, session-bound tokens.
func buildValidationEnv(proc *processInfo) []string {
	out := make([]string, 0, len(proc.env))
	for _, e := range proc.env {
		if strings.HasPrefix(e, "NRFLO_AGENT_TOKEN=") || strings.HasPrefix(e, "NRF_SESSION_ID=") {
			continue
		}
		out = append(out, e)
	}
	return out
}
