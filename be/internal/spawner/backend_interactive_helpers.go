package spawner

import (
	"encoding/json"
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

// BuildInteractiveSettingsJSON returns a --settings JSON string for interactive
// CLI agents. Currently returns "" (T4 will add hook injection).
func BuildInteractiveSettingsJSON(_ *processInfo) string {
	return ""
}

// mergeInteractiveSettings merges two Claude --settings JSON strings by combining
// their hooks arrays. When one side is empty, the other is returned unchanged.
// Both inputs must be either empty or valid JSON objects with a "hooks" key.
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
			mergedHooks[k] = v
		}
		merged["hooks"] = mergedHooks
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
			if isClaude {
				// Bump timestamp to avoid stall detection — output is T4 hook territory.
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

// deliverPrompt writes the prompt body followed by a newline to the PTY session
// after a ~250ms readiness delay. Files listed in cleanupPaths are removed on
// error (normal cleanup happens in the wait goroutine).
//
// The 250ms delay is a v1 heuristic — a future version may sniff for a readiness
// indicator in the PTY output before writing.
func deliverPrompt(sess ptySessionIface, body string) {
	time.Sleep(250 * time.Millisecond)
	payload := []byte(body + "\n")
	_, _ = sess.Write(payload)
}
