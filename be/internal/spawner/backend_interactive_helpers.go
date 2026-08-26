package spawner

import (
	"encoding/json"
	"fmt"
	"time"

	ptyPkg "be/internal/pty"
)

// ptyManagerIface abstracts *pty.Manager so tests can inject a mock PTY manager.
type ptyManagerIface interface {
	RegisterLaunch(sessionID string, l ptyPkg.Launch)
	Create(sessionID, workDir string, env []string) (ptySessionIface, error)
	Get(sessionID string) ptySessionIface
}

// ptyManagerWrapper wraps *pty.Manager to satisfy ptyManagerIface.
// *pty.Session satisfies ptySessionIface because it has Read/Write/Close/Kill/Done.
type ptyManagerWrapper struct {
	m *ptyPkg.Manager
}

func wrapPtyManager(m *ptyPkg.Manager) ptyManagerIface {
	if m == nil {
		return nil
	}
	return &ptyManagerWrapper{m: m}
}

func (w *ptyManagerWrapper) RegisterLaunch(sessionID string, l ptyPkg.Launch) {
	w.m.RegisterLaunch(sessionID, l)
}

func (w *ptyManagerWrapper) Create(sessionID, workDir string, env []string) (ptySessionIface, error) {
	sess, err := w.m.Create(sessionID, workDir, env)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (w *ptyManagerWrapper) Get(sessionID string) ptySessionIface {
	sess := w.m.Get(sessionID)
	if sess == nil {
		return nil
	}
	return sess
}

// mergeInteractiveSettings merges two Claude --settings JSON strings by combining
// their hooks sub-maps. When one side is empty, the other is returned unchanged.
// Hook event keys present in both sides (e.g. PreToolUse) have their arrays
// concatenated so no entry is lost.
func mergeInteractiveSettings(safetyJSON, hooksJSON string) string {
	if safetyJSON == "" {
		return hooksJSON
	}
	if hooksJSON == "" {
		return safetyJSON
	}

	var safety, hooks map[string]interface{}
	if err := json.Unmarshal([]byte(safetyJSON), &safety); err != nil {
		return hooksJSON
	}
	if err := json.Unmarshal([]byte(hooksJSON), &hooks); err != nil {
		return safetyJSON
	}

	// Merge hooks sub-map
	merged := make(map[string]interface{})
	for k, v := range safety {
		merged[k] = v
	}
	safetyHooks, _ := safety["hooks"].(map[string]interface{})
	hooksHooks, _ := hooks["hooks"].(map[string]interface{})
	if safetyHooks != nil && hooksHooks != nil {
		mergedHooks := make(map[string]interface{})
		for k, v := range safetyHooks {
			mergedHooks[k] = v
		}
		for k, v := range hooksHooks {
			// Concatenate arrays when both sides define the same hook event key
			if existing, ok := mergedHooks[k]; ok {
				if existingArr, ok1 := existing.([]interface{}); ok1 {
					if newArr, ok2 := v.([]interface{}); ok2 {
						mergedHooks[k] = append(existingArr, newArr...)
						continue
					}
				}
			}
			mergedHooks[k] = v
		}
		merged["hooks"] = mergedHooks
	}

	// hooks side wins on conflict for non-hooks top-level keys
	for k, v := range hooks {
		if k == "hooks" {
			continue
		}
		merged[k] = v
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return safetyJSON
	}
	return string(out)
}

// ferryPTYOutput reads PTY output and drops it. Returns when the session closes.
//
// firstByteCh is closed unconditionally on the first chunk received —
// deliverPrompt depends on it regardless of adapter type.
//
// Heartbeat (lastMessageTime / hasReceivedMessage bump) is opt-in via
// bumpOnPTYBytes. Adapters whose hooks already drive BumpLastMessage
// (Claude via PreToolUse/PostToolUse/Stop, Codex via the rollout JSONL tailer)
// pass false so the running-stall timer can accumulate while the TUI redraws.
//
// Terminal capability queries (DSR, DA, kitty keyboard, OSC color) are
// auto-answered when respondToQueries is true (codex's TUI bails during init
// otherwise). Adapters that don't probe (claude) pass false to skip
// the scan entirely. See respondToTerminalQueries.
func ferryPTYOutput(s *Spawner, proc *processInfo, sess ptySessionIface, respondToQueries bool, bumpOnPTYBytes bool) {
	buf := make([]byte, 4096)
	// carry holds the trailing bytes of the previous chunk so a mode sequence
	// straddling a read boundary is still matched.
	var carry []byte
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			if mode, ok := scanBracketedPasteMode(carry, buf[:n]); ok {
				proc.messagesMutex.Lock()
				proc.bracketedPaste = mode
				proc.messagesMutex.Unlock()
			}
			carry = carryTail(buf[:n])
			if proc.firstByteCh != nil {
				proc.firstByteOnce.Do(func() {
					s.logAgent(proc, "ready signal: first PTY bytes received")
					close(proc.firstByteCh)
				})
			}
			// Always record PTY activity time for the quiescence gate in
			// deliverPrompt — this is decoupled from stall-detection bumps
			// (those are opt-in via bumpOnPTYBytes).
			now := s.config.Clock.Now()
			proc.messagesMutex.Lock()
			proc.lastPTYByteAt = now
			proc.messagesMutex.Unlock()
			if bumpOnPTYBytes {
				proc.messagesMutex.Lock()
				proc.lastMessageTime = now
				proc.hasReceivedMessage = true
				proc.messagesMutex.Unlock()
			}

			// Feed PTY bytes into the rate-limit ring buffer for adapters that
			// lack structured event channels. Matching is best-effort (raw bytes
			// may include ANSI escapes); the ring caps at 10 chunks so only
			// recent output is inspected. Adapters with structured channels
			// (Claude via hooks, Codex via JSONL tailer) populate the buffer
			// via TrackMessage; PTY bytes are a looser fallback for those too.
			proc.appendRecent(string(buf[:n]))

			if respondToQueries {
				if reply := respondToTerminalQueries(buf[:n]); len(reply) > 0 {
					_, _ = sess.Write(reply)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// deliverPrompt waits for the TUI to signal ready, then writes the prompt
// body + CR. In raw-mode TUIs Enter is \r, not \n — \n inside the body is
// preserved as a newline in the input box and the trailing \r submits.
//
// Readiness uses a two-stage strategy:
//
//  1. Primary: wait up to sessionStartTimeout for SessionStart hook (the
//     canonical "TUI fully bootstrapped" signal from Claude). On arrival,
//     write immediately — no extra wait needed.
//  2. Fallback: if SessionStart never arrives (older Claude builds, codex),
//     wait for firstByteCh (PTY first paint) then enforce a
//     bootstrap floor of bootstrapFloor relative to spawn — gives the TUI
//     enough time to set up raw mode after the first paint.
//
// Hard total deadline of totalDeadline ensures we never hang forever; if
// nothing fires we write anyway as a last resort.
//
// Both stages only ever *infer* readiness, and under a wide fan-out that
// inference is sometimes wrong, so the write itself is confirmed and retried
// by submitPromptWithRetry (prompt_delivery_retry.go).
//
// If sessionStartCh and firstByteCh are nil (legacy callers / tests), falls
// back to a fixed bootstrapFloor delay matching the original behavior.
func deliverPrompt(s *Spawner, proc *processInfo, sess ptySessionIface, body, adapterName string, sessionStartCh, firstByteCh <-chan struct{}, ackViaHooks bool) {
	// SessionStart is the authoritative readiness signal; the first-byte
	// fallback below exists only for builds that never send it. Under a wide
	// layer fan-out SessionStart has been observed at ~4s, so a tight timeout
	// here drops an otherwise-healthy spawn onto the weaker fallback and the
	// prompt gets written into a TUI that has not painted yet. Waiting longer
	// is free on the happy path — the select returns the instant it arrives.
	const sessionStartTimeout = 10 * time.Second
	const bootstrapFloor = 1500 * time.Millisecond
	const totalDeadline = 30 * time.Second

	// Empty body = adapter delivered the prompt via argv (codex). Nothing to
	// type into the PTY; just return.
	if body == "" {
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: skipped (adapter=%s, prompt delivered via argv)", adapterName))
		return
	}

	start := time.Now()
	waitForReady(s, proc, start, sessionStartCh, firstByteCh, sessionStartTimeout, bootstrapFloor, totalDeadline)
	submitPromptWithRetry(s, proc, sess, body, adapterName, start, ackViaHooks, promptAckTimeout, promptAckPoll)
}

// waitForReady blocks until the CLI's TUI is ready to accept a submitted
// prompt. SessionStart / the first PTY byte only prove the TUI has *begun*
// bootstrapping; its input loop must finish painting before it honors a paste
// + submit CR. So after the start signal we ALWAYS gate on PTY quiescence +
// the bootstrap floor — writing during bootstrap drops the submit CR, the
// prompt is never sent, and the start-stall detector loops on the idle agent.
func waitForReady(s *Spawner, proc *processInfo, start time.Time, sessionStartCh, firstByteCh <-chan struct{}, sessionStartTimeout, bootstrapFloor, totalDeadline time.Duration) {
	if sessionStartCh == nil && firstByteCh == nil {
		time.Sleep(bootstrapFloor) // Legacy / test path: blind floor.
		return
	}
	// Stage 1: wait for a "TUI began bootstrapping" signal — SessionStart, else first PTY byte.
	select {
	case <-sessionStartCh:
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: SessionStart after %s", time.Since(start).Round(time.Millisecond)))
	case <-time.After(sessionStartTimeout):
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: SessionStart not received in %s — falling back to first-byte", sessionStartTimeout))
		remaining := totalDeadline - time.Since(start)
		if remaining <= 0 {
			s.warnAgent(proc, fmt.Sprintf("deliverPrompt: total deadline %s reached — writing anyway", totalDeadline))
			return
		}
		select {
		case <-firstByteCh:
			s.logAgent(proc, fmt.Sprintf("deliverPrompt: first-byte fallback after %s", time.Since(start).Round(time.Millisecond)))
		case <-time.After(remaining):
			s.warnAgent(proc, fmt.Sprintf("deliverPrompt: total deadline %s reached — writing anyway", totalDeadline))
			return
		}
	}
	// Stage 2: gate on PTY quiescence + bootstrap floor (both paths above only mean the TUI *started* painting).
	waitForPTYQuiescence(s, proc, start, bootstrapFloor, totalDeadline)
}

// waitForPTYQuiescence blocks until the PTY byte stream has been continuously
// idle for quietWindow (TUI finished its initial render, parked on its input
// loop) AND the bootstrap floor has elapsed. Bounded by totalDeadline so a
// chatty TUI (spinner that never settles) can't hang delivery.
func waitForPTYQuiescence(s *Spawner, proc *processInfo, start time.Time, bootstrapFloor, totalDeadline time.Duration) {
	const quietWindow = 750 * time.Millisecond
	const quietPoll = 100 * time.Millisecond
	deadlineLeft := totalDeadline - time.Since(start)
	if deadlineLeft <= 0 {
		return
	}
	quietDeadline := time.Now().Add(deadlineLeft)
	floorEnd := start.Add(bootstrapFloor)
	for {
		proc.messagesMutex.Lock()
		last := proc.lastPTYByteAt
		proc.messagesMutex.Unlock()
		idleFor := time.Since(last)
		if !last.IsZero() && idleFor >= quietWindow && time.Now().After(floorEnd) {
			s.logAgent(proc, fmt.Sprintf("deliverPrompt: ready via PTY quiescence after %s (idle for %s)",
				time.Since(start).Round(time.Millisecond), idleFor.Round(time.Millisecond)))
			return
		}
		if time.Now().After(quietDeadline) {
			s.warnAgent(proc, fmt.Sprintf("deliverPrompt: total deadline %s reached during quiescence wait — writing anyway", totalDeadline))
			return
		}
		time.Sleep(quietPoll)
	}
}
