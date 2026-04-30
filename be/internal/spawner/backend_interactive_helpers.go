package spawner

import (
	"encoding/json"
	"fmt"
	"time"

	ptyPkg "be/internal/pty"
)


// ptySessionIface abstracts *pty.Session so tests can inject a mock PTY session.
type ptySessionIface interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Kill() error
	Done() <-chan struct{}
	Pid() int
}

// ptyManagerIface abstracts *pty.Manager so tests can inject a mock PTY manager.
type ptyManagerIface interface {
	RegisterCommand(sessionID, cmd string, args []string)
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

func (w *ptyManagerWrapper) RegisterCommand(sessionID, cmd string, args []string) {
	w.m.RegisterCommand(sessionID, cmd, args)
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

// ferryPTYOutput reads PTY output and routes it to the spawner:
//   - Claude (isClaude=true): output is dropped but lastMessageTime is bumped on
//     each chunk to prevent false-positive stall detection during interactive runs.
//   - Codex/Opencode: output is recorded as category=text via TrackMessage so it
//     appears in the session message history.
//
// Returns when the session closes (Read returns error/EOF).
func ferryPTYOutput(s *Spawner, proc *processInfo, sess ptySessionIface, isClaude bool) {
	buf := make([]byte, 4096)
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			// First non-empty PTY read = TUI is alive and painting. Used by
			// deliverPrompt only as a fallback when SessionStart never fires
			// (older Claude builds, codex/opencode without hooks).
			if proc.firstByteCh != nil {
				proc.firstByteOnce.Do(func() {
					s.logAgent(proc, "ready signal: first PTY bytes received")
					close(proc.firstByteCh)
				})
			}
			if isClaude {
				// Claude TUI output is dropped — visibility comes from
				// PreToolUse/PostToolUse hook events forwarded via
				// `nrflo agent record-event`. We only bump the message
				// timestamp here so stall detection doesn't fire while the
				// agent is actively redrawing its UI.
				proc.messagesMutex.Lock()
				proc.lastMessageTime = s.config.Clock.Now()
				proc.hasReceivedMessage = true
				proc.messagesMutex.Unlock()
			} else {
				s.TrackMessage(proc, string(buf[:n]), "text")
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
//  2. Fallback: if SessionStart never arrives (older Claude builds, codex,
//     opencode), wait for firstByteCh (PTY first paint) then enforce a
//     bootstrap floor of bootstrapFloor relative to spawn — gives the TUI
//     enough time to set up raw mode after the first paint.
//
// Hard total deadline of totalDeadline ensures we never hang forever; if
// nothing fires we write anyway as a last resort.
//
// If sessionStartCh and firstByteCh are nil (legacy callers / tests), falls
// back to a fixed bootstrapFloor delay matching the original behavior.
func deliverPrompt(s *Spawner, proc *processInfo, sess ptySessionIface, body string, sessionStartCh, firstByteCh <-chan struct{}) {
	const sessionStartTimeout = 3 * time.Second
	const bootstrapFloor = 1500 * time.Millisecond
	const totalDeadline = 20 * time.Second

	start := time.Now()
	waitForReady(s, proc, start, sessionStartCh, firstByteCh, sessionStartTimeout, bootstrapFloor, totalDeadline)

	if n, err := sess.Write([]byte(body)); err != nil {
		s.errorAgent(proc, fmt.Sprintf("deliverPrompt: write body failed: %v", err))
		return
	} else {
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: wrote %d-byte body", n))
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := sess.Write([]byte("\r")); err != nil {
		s.errorAgent(proc, fmt.Sprintf("deliverPrompt: write CR failed: %v", err))
		return
	}
	s.logAgent(proc, fmt.Sprintf("deliverPrompt: submitted (total %s)", time.Since(start).Round(time.Millisecond)))
}

// waitForReady implements the two-stage SessionStart→firstByte+floor logic.
// Returns when ready (or when totalDeadline expires).
func waitForReady(s *Spawner, proc *processInfo, start time.Time, sessionStartCh, firstByteCh <-chan struct{}, sessionStartTimeout, bootstrapFloor, totalDeadline time.Duration) {
	if sessionStartCh == nil && firstByteCh == nil {
		// Legacy / test path: blind floor.
		time.Sleep(bootstrapFloor)
		return
	}
	// Stage 1: prefer SessionStart.
	select {
	case <-sessionStartCh:
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: ready via SessionStart after %s", time.Since(start).Round(time.Millisecond)))
		return
	case <-time.After(sessionStartTimeout):
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: SessionStart not received in %s — falling back to first-byte+floor", sessionStartTimeout))
	}
	// Stage 2: fall back to first-byte + bootstrap floor.
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
	// Bootstrap floor: ensure TUI has had bootstrapFloor since spawn to set
	// up raw mode after first paint.
	if rem := bootstrapFloor - time.Since(start); rem > 0 {
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: bootstrap floor — sleeping %s", rem.Round(time.Millisecond)))
		time.Sleep(rem)
	}
}
