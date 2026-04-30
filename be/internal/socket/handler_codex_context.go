package socket

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"be/internal/spawner"
)

// extractCodexContextLeft scans the codex rollout JSONL referenced by the
// hook payload's `transcript_path` for the most recent `event_msg` /
// `token_count` record and returns the % of context remaining. Returns
// (0, false) when the path is absent / unreadable / no usable record found.
//
// codex emits a token_count event every turn with:
//
//	{"type":"event_msg","payload":{"type":"token_count","info":{
//	   "last_token_usage":{"input_tokens":...,"cached_input_tokens":...,
//	     "output_tokens":...,"reasoning_output_tokens":...,"total_tokens":...},
//	   "total_token_usage":{...},
//	   "model_context_window":258400}}}
//
// `last_token_usage.input_tokens` is the prompt size sent on the most recent
// turn — the best approximation of how full the context window is right now.
// `cached_input_tokens` is a subset of input_tokens (no double-count).
func extractCodexContextLeft(event map[string]interface{}) (int, bool) {
	path, _ := event["transcript_path"].(string)
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	// Scan from the end so we honor the latest token_count.
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				Type string `json:"type"`
				Info struct {
					Last struct {
						InputTokens int `json:"input_tokens"`
					} `json:"last_token_usage"`
					ModelContextWindow int `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Type != "event_msg" || rec.Payload.Type != "token_count" {
			continue
		}
		used := rec.Payload.Info.Last.InputTokens
		ctx := rec.Payload.Info.ModelContextWindow
		if used <= 0 || ctx <= 0 {
			return 0, false
		}
		return spawner.ComputeContextLeftPct(used, ctx), true
	}
	return 0, false
}

// extractCodexNewAgentMessages reads the codex rollout JSONL starting at
// startOffset and returns every `event_msg / agent_message` body found, plus
// the new file offset (size at end of read). Used by the `Stop` hook handler
// to flush per-turn agent text to agent_messages without re-emitting bodies
// from prior turns.
//
// Returns ([], startOffset, nil) when path is empty / unreadable / file is
// shorter than startOffset (indicates rotation; reset to 0 by caller).
// Reasoning blocks are NOT extracted — codex 0.125 only writes encrypted
// reasoning content, never plaintext.
func extractCodexNewAgentMessages(path string, startOffset int64) ([]string, int64, error) {
	if path == "" {
		return nil, startOffset, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, startOffset, nil
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, startOffset, nil
	}
	size := stat.Size()
	if size < startOffset {
		// File rotated/truncated — caller should reset offset.
		return nil, 0, nil
	}
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			return nil, startOffset, nil
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, startOffset, nil
	}

	var msgs []string
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Type != "event_msg" || rec.Payload.Type != "agent_message" {
			continue
		}
		if rec.Payload.Message == "" {
			continue
		}
		msgs = append(msgs, rec.Payload.Message)
	}
	return msgs, size, nil
}
