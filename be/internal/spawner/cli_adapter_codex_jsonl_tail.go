package spawner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// startCodexJSONLTail launches a goroutine that watches the codex rollout
// JSONL file for this session's workdir and dispatches each new record to the
// Sink. Returns a cancel func that stops the goroutine.
//
// Why this exists: codex 0.130 has an upstream regression (openai/codex#21639)
// where PreToolUse/PostToolUse/Stop hooks never fire in TUI-PTY sessions. The
// rollout JSONL is the only complete, structured event channel codex still
// emits — it has agent_message text, token_count for context%, and
// function_call/function_call_output for tool I/O.
//
// The tailer is the source of truth for codex/cli_interactive visibility.
// Once running, codex's BumpsOnPTYBytes() returns false: the captureTUI
// raw-byte path is unnecessary and the stall heartbeat is driven by real
// agent activity instead of TUI redraws.
//
// Codex 0.130 ignores CODEX_HOME for rollout file placement — it always
// writes under $HOME/.codex/sessions/YYYY/MM/DD/. We identify OUR rollout
// by matching session_meta.payload.cwd in the file's first line against our
// resolved workdir.
func startCodexJSONLTail(ctx context.Context, sessionID, workDir string, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go codexJSONLTailLoop(cctx, sessionID, workDir, sink)
	return cancel
}

// codexJSONLTailLoop is the main tailer loop: first discovers the rollout
// file by globbing under ~/.codex/sessions/ and matching session_meta.cwd
// against our resolved workdir (deadline 30s — codex creates the file after
// model init), then tails byte-by-byte, parsing complete JSONL lines and
// dispatching them.
func codexJSONLTailLoop(ctx context.Context, sessionID, workDir string, sink Sink) {
	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		resolvedWorkDir = workDir
	}
	startedAt := time.Now()
	path, err := waitForRolloutFile(ctx, resolvedWorkDir, startedAt, 30*time.Second)
	if err != nil {
		return
	}
	tailRolloutFile(ctx, sessionID, path, sink)
}

// waitForRolloutFile polls $HOME/.codex/sessions/**/rollout-*.jsonl every
// 250ms looking for a file (a) created/modified after startedAt and (b)
// whose first line is a session_meta record with payload.cwd == resolvedWorkDir.
// Returns the path on success, ctx.Err() or deadline-exceeded on failure.
func waitForRolloutFile(ctx context.Context, resolvedWorkDir string, startedAt time.Time, deadline time.Duration) (string, error) {
	if resolvedWorkDir == "" {
		return "", fmt.Errorf("codex jsonl tail: empty workDir")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("codex jsonl tail: user home: %w", err)
	}
	pattern := filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*.jsonl")
	end := time.Now().Add(deadline)
	for {
		if p := findMatchingRollout(pattern, resolvedWorkDir, startedAt); p != "" {
			return p, nil
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("codex jsonl tail: matching rollout did not appear within %s for cwd=%s", deadline, resolvedWorkDir)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// findMatchingRollout scans glob matches (newest mtime first) and returns
// the first whose session_meta.payload.cwd equals resolvedWorkDir AND whose
// mtime is after startedAt. Empty string when no match.
func findMatchingRollout(pattern, resolvedWorkDir string, startedAt time.Time) string {
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return ""
	}
	// Sort by mtime desc — newest first. Stat once per candidate.
	type entry struct {
		path  string
		mtime time.Time
	}
	entries := make([]entry, 0, len(matches))
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().Before(startedAt.Add(-1 * time.Second)) {
			continue // skip files older than our spawn (1s slack for clock skew)
		}
		entries = append(entries, entry{p, info.ModTime()})
	}
	// Newest first — but most cases have ≤2 candidates so an O(n²) walk is fine.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].mtime.After(entries[i].mtime) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	for _, e := range entries {
		if rolloutCwdMatches(e.path, resolvedWorkDir) {
			return e.path
		}
	}
	return ""
}

// rolloutCwdMatches returns true if the first session_meta line in the file
// declares payload.cwd == resolvedWorkDir. Returns false on read/parse errors
// (treats them as non-match — the tailer keeps polling).
func rolloutCwdMatches(path, resolvedWorkDir string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				Cwd string `json:"cwd"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			return false // malformed first line — not a match
		}
		if rec.Type == "session_meta" {
			return rec.Payload.Cwd == resolvedWorkDir
		}
		// First non-empty record was not session_meta; not our rollout shape.
		return false
	}
	return false
}

// tailRolloutFile polls the rollout file every 250ms, reading new bytes from
// the last offset and dispatching each complete JSONL line. On file shrink
// (rotation, unlikely for codex) resets to 0.
func tailRolloutFile(ctx context.Context, sessionID, path string, sink Sink) {
	var offset int64
	for {
		offset = readNewLines(path, offset, func(line []byte) {
			dispatchCodexJSONL(sessionID, line, sink)
		})
		select {
		case <-ctx.Done():
			// One final drain after cancel — best-effort, catches the trailing
			// records that landed between the last poll and ctx.Done().
			_ = readNewLines(path, offset, func(line []byte) {
				dispatchCodexJSONL(sessionID, line, sink)
			})
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// readNewLines reads from path starting at startOffset, calls fn for each
// complete newline-delimited line found, and returns the new offset. Partial
// trailing lines (no terminating newline yet) are NOT consumed — their bytes
// stay above startOffset on the next call so we re-read them once the rest
// arrives. Best-effort: read errors return startOffset unchanged.
func readNewLines(path string, startOffset int64, fn func(line []byte)) int64 {
	f, err := os.Open(path)
	if err != nil {
		return startOffset
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return startOffset
	}
	size := stat.Size()
	if size < startOffset {
		// File rotated/truncated. Reset.
		startOffset = 0
	}
	if size == startOffset {
		return startOffset
	}
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			return startOffset
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return startOffset
	}

	// Walk full lines; keep any trailing partial as "unread" so the next
	// poll picks up the complete line.
	consumed := int64(0)
	for {
		idx := bytes.IndexByte(data[consumed:], '\n')
		if idx < 0 {
			break
		}
		line := data[consumed : consumed+int64(idx)]
		if len(line) > 0 {
			fn(line)
		}
		consumed += int64(idx) + 1
	}
	return startOffset + consumed
}

// codexJSONLRecord is the minimal shape we unmarshal per line. Unknown record
// types parse fine and are dropped by dispatchCodexJSONL when nothing matches.
type codexJSONLRecord struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type codexEventPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"` // agent_message body
	Info    *struct {
		Last struct {
			InputTokens int `json:"input_tokens"`
		} `json:"last_token_usage"`
		ModelContextWindow int `json:"model_context_window"`
	} `json:"info"` // token_count.info (often null on codex 0.130+)
}

type codexResponseItemPayload struct {
	Type      string `json:"type"`
	Name      string `json:"name"`      // function_call.name
	Arguments string `json:"arguments"` // function_call.arguments (JSON-encoded string)
	Output    string `json:"output"`    // function_call_output.output
	CallID    string `json:"call_id"`
}

// dispatchCodexJSONL parses one JSONL line and routes it to the appropriate
// Sink methods. Unknown / uninteresting records are silently ignored.
func dispatchCodexJSONL(sessionID string, line []byte, sink Sink) {
	var rec codexJSONLRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return
	}
	switch rec.Type {
	case "event_msg":
		var p codexEventPayload
		if err := json.Unmarshal(rec.Payload, &p); err != nil {
			return
		}
		switch p.Type {
		case "agent_message":
			emitAgentText(sessionID, p.Message, sink)
		case "token_count":
			if p.Info != nil && p.Info.Last.InputTokens > 0 && p.Info.ModelContextWindow > 0 {
				pct := ComputeContextLeftPct(p.Info.Last.InputTokens, p.Info.ModelContextWindow)
				_, _, _, _ = sink.UpdateContextLeft(sessionID, pct)
				sink.BumpLastMessage(sessionID)
			}
		}
	case "response_item":
		var p codexResponseItemPayload
		if err := json.Unmarshal(rec.Payload, &p); err != nil {
			return
		}
		switch p.Type {
		case "function_call":
			body := formatCodexToolUse(p.Name, p.Arguments)
			// "tool" matches the category Claude/opencode emit for tool
			// invocations (see ToolCategory in output.go — default branch).
			// Keeping a single category across backends lets UI filters and
			// scenario assertions work uniformly.
			emitMessage(sessionID, body, "tool", sink)
		case "function_call_output":
			if p.Output != "" {
				emitMessage(sessionID, p.Output, "tool", sink)
			}
		}
	}
}

// emitAgentText records an agent_message body as a "text" agent_messages row
// and broadcasts the update. Empty bodies are dropped.
func emitAgentText(sessionID, body string, sink Sink) {
	if body == "" {
		return
	}
	emitMessage(sessionID, body, "text", sink)
}

// emitMessage is the common path: RecordHookMessage + BroadcastMessagesUpdated
// + BumpLastMessage + SetLastMessage. Mirrors the opencode SSE consumer's
// "interesting event" handling.
func emitMessage(sessionID, body, category string, sink Sink) {
	projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, body, category, "")
	if err != nil {
		return
	}
	sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
	sink.BumpLastMessage(sessionID)
	// Surface a short preview in the periodic "agent status" log line.
	preview := body
	if len(preview) > 120 {
		preview = preview[:120]
	}
	sink.SetLastMessage(sessionID, preview)
}

// formatCodexToolUse renders a function_call record for an agent_messages row.
// codex.arguments is a JSON-encoded string of the tool args; we don't try to
// pretty-print here — the UI can render JSON. Format mirrors how
// `processOutput` formats Claude tool_use entries: `[<ToolName>] <args>`.
func formatCodexToolUse(name, arguments string) string {
	if name == "" {
		name = "tool"
	}
	if arguments == "" {
		return fmt.Sprintf("[%s]", name)
	}
	return fmt.Sprintf("[%s] %s", name, arguments)
}
