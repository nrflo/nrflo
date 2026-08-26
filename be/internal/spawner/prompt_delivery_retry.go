package spawner

import (
	"fmt"
	"strings"
	"time"
)

const (
	// promptAckTimeout bounds the wait for evidence that the CLI actually took
	// the submitted turn. Claude's UserPromptSubmit hook fires on submission,
	// so in a healthy spawn the first message is recorded within ~1s of the CR;
	// this is deliberately far longer so a slow-but-working spawn is never
	// re-submitted.
	promptAckTimeout = 12 * time.Second
	promptAckPoll    = 250 * time.Millisecond
	// maxPromptSubmits caps total write attempts, including the first.
	maxPromptSubmits = 3
)

// submitPromptWithRetry writes body + CR and then confirms the CLI accepted
// the turn, re-submitting when it did not.
//
// Readiness before the first write is only ever *inferred* (SessionStart or
// first PTY byte, then a quiescence gate). Under a wide layer fan-out that
// inference is wrong often enough to matter: a quiet PTY during a starved
// bootstrap is indistinguishable from a quiet PTY parked on a ready input
// loop, so the paste + CR lands mid-bootstrap and the TUI redraws over it.
// The prompt is then silently gone — the process is healthy, the model never
// got a turn, and the only recovery is the start-stall detector two minutes
// later. Measured incidence on 7-wide fan-outs was ~21% of nodes.
//
// So delivery is confirmed rather than assumed. hasReceivedMessage is the
// acknowledgement: for adapters that do not bump on raw PTY bytes it flips
// only on a real recorded event, and the UserPromptSubmit echo (seq 0) is
// always the first one. That makes the re-submit safe by construction — it
// fires only when literally nothing was recorded, i.e. when the turn provably
// never started.
// ackTimeout/ackPoll are parameters rather than the consts directly so tests
// can drive the loop at test scale without mutating package state.
func submitPromptWithRetry(s *Spawner, proc *processInfo, sess ptySessionIface, body, adapterName string, start time.Time, ackViaHooks bool, ackTimeout, ackPoll time.Duration) {
	for attempt := 1; attempt <= maxPromptSubmits; attempt++ {
		if !writePromptOnce(s, proc, sess, body, adapterName) {
			return
		}
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: submitted (total %s, attempt %d/%d)",
			time.Since(start).Round(time.Millisecond), attempt, maxPromptSubmits))

		// Adapters that bump on PTY bytes (codex) have no usable ack: the
		// signal is set by paint traffic, not by the turn starting. They also
		// deliver inline via argv, so this path is not reached for them.
		if !ackViaHooks {
			return
		}
		if waitPromptAck(proc, ackTimeout, ackPoll) {
			return
		}
		if attempt < maxPromptSubmits {
			s.warnAgent(proc, fmt.Sprintf(
				"deliverPrompt: no recorded activity %s after submit — prompt was not accepted, re-submitting (attempt %d/%d)",
				ackTimeout, attempt+1, maxPromptSubmits))
		}
	}
	s.warnAgent(proc, fmt.Sprintf(
		"deliverPrompt: no recorded activity after %d submits — leaving recovery to stall detection", maxPromptSubmits))
}

// writePromptOnce writes the body then the submit CR. Returns false when a
// write failed, which is terminal for delivery — retrying a broken PTY only
// burns the attempt budget.
func writePromptOnce(s *Spawner, proc *processInfo, sess ptySessionIface, body, adapterName string) bool {
	proc.messagesMutex.Lock()
	paste := proc.bracketedPaste
	proc.messagesMutex.Unlock()

	payload := body
	if paste {
		payload = bracketedPasteSet2004 + stripPasteEnd(body) + bracketedPasteEnd
	}
	n, err := sess.Write([]byte(payload))
	if err != nil {
		s.errorAgent(proc, fmt.Sprintf("deliverPrompt: write body failed: %v", err))
		return false
	}
	s.logAgent(proc, fmt.Sprintf("deliverPrompt: wrote %d-byte body (adapter=%s, bracketed_paste=%t)", n, adapterName, paste))

	// In raw-mode TUIs Enter is \r; the short gap lets the paste settle before
	// the submit keystroke.
	time.Sleep(150 * time.Millisecond)
	if _, err := sess.Write([]byte("\r")); err != nil {
		s.errorAgent(proc, fmt.Sprintf("deliverPrompt: write CR failed: %v", err))
		return false
	}
	return true
}

// bracketedPasteSet2004 / bracketedPasteEnd delimit a bracketed paste. A TUI
// that enabled DECSET ?2004 (Claude does, for its whole input loop) treats the
// enclosed bytes as one atomic paste. Without the markers the same bytes go
// through the per-keystroke path, where a multi-KB body is silently reduced to
// its last ~100 bytes — the prompt looks delivered (write returns the full
// length, the submit CR is accepted, the UserPromptSubmit hook fires and acks)
// but the agent receives a fragment starting mid-word.
const (
	bracketedPasteSet2004 = "\x1b[200~"
	bracketedPasteEnd     = "\x1b[201~"
)

// stripPasteEnd removes any literal paste terminator from the body so embedded
// text cannot end the paste early and drop the remainder back onto the
// keystroke path.
func stripPasteEnd(body string) string {
	if !strings.Contains(body, bracketedPasteEnd) {
		return body
	}
	return strings.ReplaceAll(body, bracketedPasteEnd, "")
}

// waitPromptAck reports whether the proc recorded any activity within timeout,
// polling because the signal is a mutex-guarded field rather than a channel —
// it is set from several unrelated paths (hook events, tool spans, output).
func waitPromptAck(proc *processInfo, timeout, poll time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		proc.messagesMutex.Lock()
		acked := proc.hasReceivedMessage
		proc.messagesMutex.Unlock()
		if acked {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		if proc.doneCh != nil {
			select {
			case <-proc.doneCh: // process exited; nothing left to ack
				return true
			case <-time.After(poll):
			}
			continue
		}
		time.Sleep(poll)
	}
}
