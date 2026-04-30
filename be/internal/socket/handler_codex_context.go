package socket

import (
	"bytes"
	"encoding/json"
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
