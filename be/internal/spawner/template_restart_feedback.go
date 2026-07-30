package spawner

import (
	"encoding/json"
	"strconv"

	"be/internal/repo"
)

// reasonFailRestart/reasonTimeoutRestart mirror the result_reason strings
// spawner_monitor.go's UpdateResult calls stamp on a continued session.
const (
	reasonFailRestart    = "fail_restart"
	reasonTimeoutRestart = "timeout_restart"
)

// restartFeedbackTailSize caps the validation output tail folded into the
// restart-feedback block. Mirrors stepCheckTailSize (step_checks.go) — the
// same agent-facing precedent for how much captured output is worth
// re-showing.
const restartFeedbackTailSize = stepCheckTailSize

// restartFeedbackBlock builds the truthful restart-feedback prepend block
// for a fail-restart or timeout-restart relaunch, or "" when there is
// nothing truthful to say. prevData is the already-resolved ${PREVIOUS_DATA}
// (same value the low-context injectable would have used) and is folded
// into whichever block renders.
//
// A timeout_restart always renders (nothing to gate — the process was
// killed for wall-clock timeout, not a claim requiring verification). A
// fail_restart only renders when the PREVIOUS session itself wrote a
// validation_failure finding (actor-gated to validationFindingActorID) —
// copyFindingsForContinuation carries that key forward under a different
// actor ID, so a stale copy on a later session can never fake this.
func (s *Spawner) restartFeedbackBlock(prev prevContinuedSession, prevData string) string {
	switch prev.reason {
	case reasonTimeoutRestart:
		return s.expandInjectable("timeout-restart", map[string]string{"PREVIOUS_DATA": prevData})
	case reasonFailRestart:
		return s.validationFailureBlock(prev.sessionID, prevData)
	default:
		return ""
	}
}

// validationFailureBlock reads the actor-gated validation_failure finding
// off the previous session and renders the validation-failure injectable,
// or "" when no genuine finding exists.
func (s *Spawner) validationFailureBlock(prevSessionID, prevData string) string {
	pool := s.pool()
	if pool == nil {
		return ""
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	raw, ok := findingRepo.GetOwnKeyByActor("session", prevSessionID, findingKeyValidationFailure, validationFindingActorID)
	if !ok {
		return ""
	}

	var payload struct {
		Command    string `json:"command"`
		ExitCode   int    `json:"exit_code"`
		OutputTail string `json:"output_tail"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}

	tail := capTail(payload.OutputTail, restartFeedbackTailSize)
	if tail == "" {
		tail = "(no output captured)"
	}

	return s.expandInjectable("validation-failure", map[string]string{
		"FAILED_COMMAND": payload.Command,
		"EXIT_CODE":      strconv.Itoa(payload.ExitCode),
		"OUTPUT_TAIL":    tail,
		"PREVIOUS_DATA":  prevData,
	})
}

// restartFeedbackForProc resolves and builds the restart-feedback block for
// a live proc, using the same agentType/modelID/nodeID/wfi key the template
// prepend seam uses. Used by the codex app-server thread-resume path, which
// bypasses prep.prompt entirely and so would otherwise never see this block.
func (s *Spawner) restartFeedbackForProc(proc *processInfo) string {
	prev, wfiID, ok := s.resolvePrevContinuedSession(proc.projectID, proc.ticketID, proc.workflowName, proc.agentType, proc.modelID, proc.nodeID, proc.workflowInstanceID)
	if !ok {
		return ""
	}
	prevData := s.previousDataFor(prev, wfiID, proc.agentType, proc.projectID, proc.workflowName, proc.nodeID)
	return s.restartFeedbackBlock(prev, prevData)
}

// capTail keeps at most the last n bytes of s, backing off to a UTF-8 rune
// boundary, and prefixes a truncation marker when it cut anything. Keeps
// the TAIL (unlike foldfmt.CapBytes, which keeps the head) — the failing
// command's most recent output is the truthful part to show.
func capTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut++
	}
	return "[... truncated]\n" + s[cut:]
}
