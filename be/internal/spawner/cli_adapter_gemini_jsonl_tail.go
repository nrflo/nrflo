package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// startGeminiJSONLTail launches a goroutine that watches the Gemini session
// JSONL transcript and dispatches each new line to the Sink. Returns a cancel
// func that stops the goroutine.
//
// Gemini writes transcripts to:
//
//	<userHome>/.gemini/tmp/<projectAlias>/chats/session-*-<sessionID>.jsonl
//
// Although the spawner overrides HOME to a per-session tempdir for hooks/auth,
// Gemini's transcript writer uses os.homedir() (getpwuid) and ignores HOME.
// We therefore search both locations and the unique session UUID picks the
// right file out of the glob.
func startGeminiJSONLTail(ctx context.Context, sessionID, geminiHome, workDir string, maxCtx int, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go geminiJSONLTailLoop(cctx, sessionID, geminiHome, maxCtx, sink)
	return cancel
}

func geminiJSONLTailLoop(ctx context.Context, sessionID, geminiHome string, maxCtx int, sink Sink) {
	path, err := waitForGeminiTranscript(ctx, sessionID, geminiHome, 30*time.Second)
	if err != nil {
		return
	}
	state := &geminiTailState{seenContent: make(map[string]int)}
	tailGeminiTranscript(ctx, sessionID, path, maxCtx, sink, state)
}

// geminiTranscriptSearchRoots returns the directory roots under which the
// per-session JSONL file may appear. The override is included for forward-
// compatibility; the user's real home is where current Gemini versions write.
func geminiTranscriptSearchRoots(geminiHome string) []string {
	roots := make([]string, 0, 2)
	if geminiHome != "" {
		roots = append(roots, geminiHome)
	}
	if real, err := os.UserHomeDir(); err == nil && real != "" && real != geminiHome {
		roots = append(roots, real)
	}
	return roots
}

// geminiSessionSuffix returns the suffix Gemini uses for the transcript
// filename: an 8-character prefix of the UUID. Filenames look like
// `session-<UTC-timestamp>-<first8>.jsonl`.
func geminiSessionSuffix(sessionID string) string {
	if len(sessionID) >= 8 {
		return sessionID[:8]
	}
	return sessionID
}

// waitForGeminiTranscript polls for the per-session JSONL file every 250ms.
// The filename suffix is the first 8 characters of the session UUID.
func waitForGeminiTranscript(ctx context.Context, sessionID, geminiHome string, deadline time.Duration) (string, error) {
	roots := geminiTranscriptSearchRoots(geminiHome)
	suffix := geminiSessionSuffix(sessionID)
	end := time.Now().Add(deadline)
	for {
		for _, root := range roots {
			pattern := filepath.Join(root, ".gemini", "tmp", "*", "chats", "session-*-"+suffix+".jsonl")
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return matches[0], nil
			}
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("gemini jsonl tail: transcript did not appear within %s for session=%s", deadline, sessionID)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// tailGeminiTranscript polls the transcript file every 250ms. Mirrors
// tailRolloutFile from cli_adapter_codex_jsonl_tail.go with a final-drain
// pass on ctx.Done().
func tailGeminiTranscript(ctx context.Context, sessionID, path string, maxCtx int, sink Sink, state *geminiTailState) {
	var offset int64
	for {
		offset = readNewLines(path, offset, func(line []byte) {
			dispatchGeminiJSONL(sessionID, line, sink, maxCtx, state)
		})
		select {
		case <-ctx.Done():
			_ = readNewLines(path, offset, func(line []byte) {
				dispatchGeminiJSONL(sessionID, line, sink, maxCtx, state)
			})
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// geminiTailState tracks per-turn-id seen-content length for cumulative delta
// deduplication: Gemini rewrites the full content on each token, so we only
// emit the newly-appended suffix.
type geminiTailState struct {
	seenContent map[string]int // turn-id -> byte length of last-emitted content
}

// geminiTranscriptLine is the minimal shape for one JSONL record. The $set
// sentinel field marks advisory records that must be skipped. Gemini emits
// `"type":"gemini"` for assistant turns (not `"role":"assistant"`) and uses
// camelCase `toolCalls`.
type geminiTranscriptLine struct {
	Set     *json.RawMessage `json:"$set"`
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Content string           `json:"content"`
	Tokens  struct {
		Total int `json:"total"`
	} `json:"tokens"`
	ToolCalls []geminiToolCall `json:"toolCalls"`
}

type geminiToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// dispatchGeminiJSONL parses one JSONL line and routes it to Sink methods.
// $set advisory records, non-assistant turns, and malformed JSON are silently
// dropped — mirroring dispatchCodexJSONL.
func dispatchGeminiJSONL(sessionID string, line []byte, sink Sink, maxCtx int, state *geminiTailState) {
	var rec geminiTranscriptLine
	if err := json.Unmarshal(line, &rec); err != nil {
		return
	}
	if rec.Set != nil {
		return
	}
	if rec.Type != "gemini" {
		return
	}

	// Emit only the newly-appended suffix; Gemini rewrites the full content.
	prev := state.seenContent[rec.ID]
	if len(rec.Content) > prev {
		emitAgentText(sessionID, rec.Content[prev:], sink)
		state.seenContent[rec.ID] = len(rec.Content)
	}

	if rec.Tokens.Total > 0 && maxCtx > 0 {
		pct := ComputeContextLeftPct(rec.Tokens.Total, maxCtx)
		_, _, _, _ = sink.UpdateContextLeft(sessionID, pct)
		sink.BumpLastMessage(sessionID)
	}

	for _, tc := range rec.ToolCalls {
		emitMessage(sessionID, formatGeminiToolCall(tc.Name, tc.Args), "tool", sink)
	}
}

// formatGeminiToolCall renders a tool_call for an agent_messages row.
// Mirrors formatCodexToolUse: `[<name>] <args>`.
func formatGeminiToolCall(name string, args json.RawMessage) string {
	if name == "" {
		name = "tool"
	}
	if len(args) == 0 {
		return fmt.Sprintf("[%s]", name)
	}
	return fmt.Sprintf("[%s] %s", name, string(args))
}
