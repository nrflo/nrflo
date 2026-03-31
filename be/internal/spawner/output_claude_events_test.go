package spawner

import (
	"strings"
	"testing"
)

// === assistant: thinking content ===

func TestProcessOutput_Claude_AssistantThinking_TracksMessage(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-think-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":     "thinking",
					"thinking": "The code needs refactoring",
				},
			},
		},
	})

	entries := pendingEntries(proc)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !strings.HasPrefix(entries[0].Content, "[thinking] ") {
		t.Errorf("message should start with '[thinking] ', got: %q", entries[0].Content)
	}
	if !strings.Contains(entries[0].Content, "The code needs refactoring") {
		t.Errorf("message should contain thinking text, got: %q", entries[0].Content)
	}
	if entries[0].Category != "text" {
		t.Errorf("thinking category = %q, want %q", entries[0].Category, "text")
	}
}

func TestProcessOutput_Claude_AssistantThinking_EmptyIgnored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-think-2")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"type": "thinking", "thinking": ""},
			},
		},
	})

	if len(pendingMessages(proc)) != 0 {
		t.Errorf("expected no messages for empty thinking text")
	}
}

// === system: init ===

func TestProcessOutput_Claude_SystemInit_TracksMessage(t *testing.T) {
	tests := []struct {
		name    string
		version string
		model   string
		wants   []string
	}{
		{"version-and-model", "1.2.3", "claude-3-5-sonnet", []string{"[init]", "v1.2.3", "model=claude-3-5-sonnet"}},
		{"version-only", "0.9.0", "", []string{"[init]", "v0.9.0"}},
		{"model-only", "", "claude-opus-4", []string{"[init]", "model=claude-opus-4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := noPoolSpawner()
			proc := minProc("sess-init-" + tt.name)
			processJSON(s, proc, map[string]interface{}{
				"type":                "system",
				"subtype":             "init",
				"claude_code_version": tt.version,
				"model":               tt.model,
			})
			msgs := pendingMessages(proc)
			if len(msgs) != 1 {
				t.Fatalf("expected 1 message, got %d", len(msgs))
			}
			for _, want := range tt.wants {
				if !strings.Contains(msgs[0], want) {
					t.Errorf("message %q missing %q", msgs[0], want)
				}
			}
		})
	}
}

func TestProcessOutput_Claude_SystemInit_BothEmpty_Silent(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-init-empty")
	processJSON(s, proc, map[string]interface{}{
		"type": "system", "subtype": "init",
		"claude_code_version": "", "model": "",
	})
	if len(pendingMessages(proc)) != 0 {
		t.Errorf("expected no messages when version and model are both empty")
	}
}

func TestProcessOutput_Claude_SystemNonInit_Silent(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-sysother")
	processJSON(s, proc, map[string]interface{}{
		"type": "system", "subtype": "heartbeat",
		"claude_code_version": "1.0.0",
	})
	if len(pendingMessages(proc)) != 0 {
		t.Errorf("expected no messages for non-init system event")
	}
}

// === assistant: context tracking via usage ===

func TestProcessOutput_Claude_AssistantUsage_UpdatesContextLeft(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ctx-1")
	proc.maxContext = 200000

	// input=30000, cache_read=10000, cache_create=10000, output=10000 → total=60000
	// 100 - (60000*100/200000) = 100-30 = 70
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{},
			"usage": map[string]interface{}{
				"input_tokens":                30000.0,
				"cache_read_input_tokens":     10000.0,
				"cache_creation_input_tokens": 10000.0,
				"output_tokens":               10000.0,
			},
		},
	})

	if proc.contextLeft != 70 {
		t.Errorf("contextLeft = %d, want 70", proc.contextLeft)
	}
}

func TestProcessOutput_Claude_AssistantUsage_ZeroTokens_NoUpdate(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-ctx-2")
	proc.maxContext = 200000
	proc.contextLeft = 90

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{},
			"usage":   map[string]interface{}{"input_tokens": 0.0, "output_tokens": 0.0},
		},
	})

	if proc.contextLeft != 90 {
		t.Errorf("contextLeft = %d, want 90 (unchanged for zero tokens)", proc.contextLeft)
	}
}

// === result: context tracking via usage ===

func TestProcessOutput_Claude_ResultUsage_Direct_UpdatesContextLeft(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-res-1")
	proc.maxContext = 100000

	// 50000 tokens of 100000 → 100 - (50000*100/100000) = 50
	processJSON(s, proc, map[string]interface{}{
		"type":  "result",
		"usage": map[string]interface{}{"input_tokens": 50000.0, "output_tokens": 0.0},
	})

	if proc.contextLeft != 50 {
		t.Errorf("contextLeft = %d, want 50", proc.contextLeft)
	}
}

func TestProcessOutput_Claude_ResultUsage_Nested_UpdatesContextLeft(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-res-2")
	proc.maxContext = 100000

	// usage nested under message.usage → 20000 tokens → 100 - (20000*100/100000) = 80
	processJSON(s, proc, map[string]interface{}{
		"type": "result",
		"message": map[string]interface{}{
			"usage": map[string]interface{}{"input_tokens": 20000.0, "output_tokens": 0.0},
		},
	})

	if proc.contextLeft != 80 {
		t.Errorf("contextLeft = %d, want 80", proc.contextLeft)
	}
}

func TestProcessOutput_Claude_ResultUsage_Missing_NoUpdate(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-res-3")
	proc.contextLeft = 55

	processJSON(s, proc, map[string]interface{}{"type": "result"})

	if proc.contextLeft != 55 {
		t.Errorf("contextLeft = %d, want 55 (unchanged)", proc.contextLeft)
	}
}

// === rate_limit_event ===

func TestProcessOutput_Claude_RateLimit_NonAllowed_Tracked(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		limitType string
	}{
		{"throttled", "throttled", "output_tokens"},
		{"rejected", "rejected", "requests"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := noPoolSpawner()
			proc := minProc("sess-rl-" + tt.name)
			processJSON(s, proc, map[string]interface{}{
				"type": "rate_limit_event",
				"rate_limit_info": map[string]interface{}{
					"status": tt.status, "rateLimitType": tt.limitType,
				},
			})
			entries := pendingEntries(proc)
			if len(entries) != 1 {
				t.Fatalf("expected 1 message, got %d", len(entries))
			}
			if !strings.HasPrefix(entries[0].Content, "[rate_limit]") {
				t.Errorf("message should start with '[rate_limit]', got: %q", entries[0].Content)
			}
			if !strings.Contains(entries[0].Content, tt.limitType) {
				t.Errorf("message should contain limitType %q, got: %q", tt.limitType, entries[0].Content)
			}
			if !strings.Contains(entries[0].Content, tt.status) {
				t.Errorf("message should contain status %q, got: %q", tt.status, entries[0].Content)
			}
			if entries[0].Category != "text" {
				t.Errorf("rate_limit category = %q, want %q", entries[0].Category, "text")
			}
		})
	}
}

func TestProcessOutput_Claude_RateLimit_Allowed_Silent(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-rl-ok")
	processJSON(s, proc, map[string]interface{}{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]interface{}{
			"status": "allowed", "rateLimitType": "requests",
		},
	})
	if len(pendingMessages(proc)) != 0 {
		t.Errorf("expected no messages for allowed rate limit")
	}
}

func TestProcessOutput_Claude_RateLimit_MissingInfo_NoCrash(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-rl-noinfo")
	// Must not panic even with no rate_limit_info key
	processJSON(s, proc, map[string]interface{}{"type": "rate_limit_event"})
	if len(pendingMessages(proc)) != 0 {
		t.Errorf("expected no messages for missing rate_limit_info")
	}
}

// === content_block_stop: dead code removed ===

func TestProcessOutput_Claude_ContentBlockStop_Ignored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-cbs-1")
	processJSON(s, proc, map[string]interface{}{
		"type": "content_block_stop", "index": 0,
	})
	if len(pendingMessages(proc)) != 0 {
		t.Errorf("content_block_stop should produce no messages (dead code removed), got %d", len(pendingMessages(proc)))
	}
}

// === codex turn.completed: proc.maxContext ===

func TestProcessOutput_TurnCompleted_UsesMaxContext_WhenSet(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tc-max")
	proc.maxContext = 100000

	// 10000 of 100000 → 100 - (10000*100/100000) = 90
	processJSON(s, proc, map[string]interface{}{
		"type":  "turn.completed",
		"usage": map[string]interface{}{"input_tokens": 10000.0, "output_tokens": 0.0},
	})

	if proc.contextLeft != 90 {
		t.Errorf("contextLeft = %d, want 90 (using maxContext=100000)", proc.contextLeft)
	}
}

func TestProcessOutput_TurnCompleted_FallsBackTo200k_WhenMaxContextZero(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tc-fallback")
	// proc.maxContext = 0 (minProc default) → fallback to 200000

	// 40000 of 200000 → 100 - (40000*100/200000) = 80
	processJSON(s, proc, map[string]interface{}{
		"type":  "turn.completed",
		"usage": map[string]interface{}{"input_tokens": 40000.0, "output_tokens": 0.0},
	})

	if proc.contextLeft != 80 {
		t.Errorf("contextLeft = %d, want 80 (fallback maxContext=200000)", proc.contextLeft)
	}
}
