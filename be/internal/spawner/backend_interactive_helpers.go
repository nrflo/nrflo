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

// ferryPTYOutput reads PTY output and drops it. Returns when the session closes.
//
// firstByteCh is closed unconditionally on the first chunk received —
// deliverPrompt depends on it regardless of adapter type.
//
// Heartbeat (lastMessageTime / hasReceivedMessage bump) is opt-in via
// bumpOnPTYBytes. Adapters whose hooks or SSE bus or JSONL tailer already
// drive BumpLastMessage (Claude via PreToolUse/PostToolUse/Stop, Opencode via
// message.part.updated / session.idle, Codex via the rollout JSONL tailer)
// pass false so the running-stall timer can accumulate while the TUI redraws.
//
// Terminal capability queries (DSR, DA, kitty keyboard, OSC color) are
// auto-answered when respondToQueries is true (codex's TUI bails during init
// otherwise). Adapters that don't probe (claude, opencode) pass false to skip
// the scan entirely. See respondToTerminalQueries.
func ferryPTYOutput(s *Spawner, proc *processInfo, sess ptySessionIface, respondToQueries bool, bumpOnPTYBytes bool) {
	buf := make([]byte, 4096)
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			if proc.firstByteCh != nil {
				proc.firstByteOnce.Do(func() {
					s.logAgent(proc, "ready signal: first PTY bytes received")
					close(proc.firstByteCh)
				})
			}
			if bumpOnPTYBytes {
				proc.messagesMutex.Lock()
				proc.lastMessageTime = s.config.Clock.Now()
				proc.hasReceivedMessage = true
				proc.messagesMutex.Unlock()
			}

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

// respondToTerminalQueries scans a chunk of PTY output for terminal capability
// queries that a real terminal emulator would answer. Returns the concatenated
// canned replies (or nil if no queries were seen). Without this, codex's TUI
// init blocks waiting on these probes and bails after a few seconds. The
// replies advertise a minimal but valid xterm-like terminal:
//
//	\x1b[6n        DSR cursor position    → \x1b[24;80R   (row 24, col 80)
//	\x1b[c         DA primary             → \x1b[?1;2c    (VT100 + advanced video)
//	\x1b[>c        DA secondary           → \x1b[>0;0;0c  (no version info)
//	\x1b[?u        kitty keyboard query   → \x1b[?0u      (no kitty flags)
//	\x1b]10;?\x1b\\  OSC 10 fg color      → \x1b]10;rgb:c0c0/c0c0/c0c0\x1b\\
//	\x1b]11;?\x1b\\  OSC 11 bg color      → \x1b]11;rgb:0000/0000/0000\x1b\\
func respondToTerminalQueries(chunk []byte) []byte {
	var reply []byte
	for i := 0; i < len(chunk); i++ {
		// CSI sequences: ESC [ ... final-byte
		if i+1 < len(chunk) && chunk[i] == 0x1b && chunk[i+1] == '[' {
			// Find final byte (0x40-0x7e) and capture intermediate bytes.
			j := i + 2
			for j < len(chunk) && (chunk[j] < 0x40 || chunk[j] > 0x7e) {
				j++
			}
			if j >= len(chunk) {
				break
			}
			seq := chunk[i : j+1]
			switch string(seq) {
			case "\x1b[6n":
				reply = append(reply, []byte("\x1b[24;80R")...)
			case "\x1b[c", "\x1b[0c":
				reply = append(reply, []byte("\x1b[?1;2c")...)
			case "\x1b[>c", "\x1b[>0c":
				reply = append(reply, []byte("\x1b[>0;0;0c")...)
			case "\x1b[?u":
				reply = append(reply, []byte("\x1b[?0u")...)
			}
			i = j
			continue
		}
		// OSC sequences: ESC ] ... BEL or ST (ESC \)
		if i+1 < len(chunk) && chunk[i] == 0x1b && chunk[i+1] == ']' {
			// Find terminator.
			j := i + 2
			term := 0
			for j < len(chunk) {
				if chunk[j] == 0x07 {
					term = 1
					break
				}
				if chunk[j] == 0x1b && j+1 < len(chunk) && chunk[j+1] == '\\' {
					term = 2
					break
				}
				j++
			}
			if term == 0 {
				break
			}
			payload := string(chunk[i+2 : j])
			switch payload {
			case "10;?":
				reply = append(reply, []byte("\x1b]10;rgb:c0c0/c0c0/c0c0\x1b\\")...)
			case "11;?":
				reply = append(reply, []byte("\x1b]11;rgb:0000/0000/0000\x1b\\")...)
			}
			if term == 1 {
				i = j
			} else {
				i = j + 1
			}
		}
	}
	return reply
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
func deliverPrompt(s *Spawner, proc *processInfo, sess ptySessionIface, body, adapterName string, sessionStartCh, firstByteCh <-chan struct{}) {
	const sessionStartTimeout = 3 * time.Second
	const bootstrapFloor = 1500 * time.Millisecond
	const totalDeadline = 20 * time.Second

	// Empty body = adapter delivered the prompt via argv (codex). Nothing to
	// type into the PTY; just return.
	if body == "" {
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: skipped (adapter=%s, prompt delivered via argv)", adapterName))
		return
	}

	start := time.Now()
	waitForReady(s, proc, start, sessionStartCh, firstByteCh, sessionStartTimeout, bootstrapFloor, totalDeadline)

	if n, err := sess.Write([]byte(body)); err != nil {
		s.errorAgent(proc, fmt.Sprintf("deliverPrompt: write body failed: %v", err))
		return
	} else {
		s.logAgent(proc, fmt.Sprintf("deliverPrompt: wrote %d-byte body (adapter=%s)", n, adapterName))
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
