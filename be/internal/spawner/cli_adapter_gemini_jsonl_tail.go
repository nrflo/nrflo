package spawner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// startGeminiJSONLTail launches a goroutine that watches the Gemini session
// JSONL transcript and dispatches each new line to the Sink. Returns a cancel
// func that stops the goroutine.
//
// Gemini writes transcripts to:
//
//	<GeminiHome>/.gemini/tmp/<sha256(resolvedWorkDir)>/chats/session-*-<sessionID>.jsonl
//
// The tailer discovers the file (up to 30s), then tails it every 250ms,
// emitting assistant text deltas, tool calls, and context usage updates.
func startGeminiJSONLTail(ctx context.Context, sessionID, geminiHome, workDir string, maxCtx int, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go geminiJSONLTailLoop(cctx, sessionID, geminiHome, workDir, maxCtx, sink)
	return cancel
}

func geminiJSONLTailLoop(ctx context.Context, sessionID, geminiHome, workDir string, maxCtx int, sink Sink) {
	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		resolvedWorkDir = workDir
	}
	path, err := waitForGeminiTranscript(ctx, sessionID, geminiHome, resolvedWorkDir, 30*time.Second)
	if err != nil {
		return
	}
	state := &geminiTailState{seenContent: make(map[string]int)}
	tailGeminiTranscript(ctx, sessionID, path, maxCtx, sink, state)
}

// waitForGeminiTranscript polls for the per-session JSONL file every 250ms.
// The file path encodes a sha256 of the resolved workdir and the session UUID
// as a suffix: session-*-<sessionID>.jsonl.
func waitForGeminiTranscript(ctx context.Context, sessionID, geminiHome, resolvedWorkDir string, deadline time.Duration) (string, error) {
	h := sha256.Sum256([]byte(resolvedWorkDir))
	projectHash := hex.EncodeToString(h[:])
	pattern := filepath.Join(geminiHome, ".gemini", "tmp", projectHash, "chats", "session-*-"+sessionID+".jsonl")
	end := time.Now().Add(deadline)
	for {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			return matches[0], nil
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
// sentinel field marks advisory records that must be skipped.
type geminiTranscriptLine struct {
	Set     *json.RawMessage `json:"$set"`
	ID      string           `json:"id"`
	Role    string           `json:"role"`
	Content string           `json:"content"`
	Tokens  struct {
		Total int `json:"total"`
	} `json:"tokens"`
	ToolCalls []geminiToolCall `json:"tool_calls"`
}

type geminiToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// dispatchGeminiJSONL parses one JSONL line and routes it to Sink methods.
// $set advisory records, non-assistant roles, and malformed JSON are silently
// dropped — mirroring dispatchCodexJSONL.
func dispatchGeminiJSONL(sessionID string, line []byte, sink Sink, maxCtx int, state *geminiTailState) {
	var rec geminiTranscriptLine
	if err := json.Unmarshal(line, &rec); err != nil {
		return
	}
	if rec.Set != nil {
		return
	}
	if rec.Role != "assistant" {
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
