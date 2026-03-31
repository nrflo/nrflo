package spawner

import (
	"strings"
	"testing"
)

// === tool_result: nested content array correlation ===

func TestProcessOutput_Claude_ToolResult_NestedContent_MatchesTask(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-nc-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_nested",
					"name": "Task",
					"input": map[string]interface{}{
						"description": "nested id test",
					},
				},
			},
		},
	})

	// tool_use_id nested inside content array (alternative format)
	processJSON(s, proc, map[string]interface{}{
		"type": "tool_result",
		"content": []interface{}{
			map[string]interface{}{
				"tool_use_id": "toolu_nested",
				"type":        "tool_result",
			},
		},
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Task + TaskResult), got %d", len(entries))
	}
	if !strings.HasPrefix(entries[1].Content, "[TaskResult]") {
		t.Errorf("expected [TaskResult] message, got: %q", entries[1].Content)
	}
	if entries[1].Category != "subagent" {
		t.Errorf("category = %q, want subagent", entries[1].Category)
	}
}

// === user type tool_result triggers tool result correlation ===

func TestProcessOutput_Claude_UserToolResult_MatchesTask(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-utr-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_utr001",
					"name": "Task",
					"input": map[string]interface{}{
						"description":   "background task",
						"subagent_type": "general-purpose",
					},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "toolu_utr001",
				},
			},
		},
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Task + TaskResult), got %d", len(entries))
	}
	if !strings.HasPrefix(entries[1].Content, "[TaskResult]") {
		t.Errorf("user tool_result should generate [TaskResult], got: %q", entries[1].Content)
	}
	if entries[1].Category != "subagent" {
		t.Errorf("category = %q, want subagent", entries[1].Category)
	}
}

// TestProcessOutput_Claude_ToolResult_NestedContent_MatchesAgent mirrors the Task version for Agent.
func TestProcessOutput_Claude_ToolResult_NestedContent_MatchesAgent(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-nc-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_agent_nested",
					"name": "Agent",
					"input": map[string]interface{}{
						"description": "nested id agent test",
					},
				},
			},
		},
	})

	// tool_use_id nested inside content array (alternative format)
	processJSON(s, proc, map[string]interface{}{
		"type": "tool_result",
		"content": []interface{}{
			map[string]interface{}{
				"tool_use_id": "toolu_agent_nested",
				"type":        "tool_result",
			},
		},
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Agent + AgentResult), got %d", len(entries))
	}
	if !strings.HasPrefix(entries[1].Content, "[AgentResult]") {
		t.Errorf("expected [AgentResult] message, got: %q", entries[1].Content)
	}
	if entries[1].Category != "subagent" {
		t.Errorf("category = %q, want subagent", entries[1].Category)
	}
}

// TestProcessOutput_Claude_UserToolResult_MatchesAgent mirrors the Task version for Agent.
func TestProcessOutput_Claude_UserToolResult_MatchesAgent(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-utr-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_agent_utr001",
					"name": "Agent",
					"input": map[string]interface{}{
						"description":   "background agent task",
						"subagent_type": "general-purpose",
					},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "toolu_agent_utr001",
				},
			},
		},
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Agent + AgentResult), got %d", len(entries))
	}
	if !strings.HasPrefix(entries[1].Content, "[AgentResult]") {
		t.Errorf("user tool_result should generate [AgentResult] for Agent tool, got: %q", entries[1].Content)
	}
	if entries[1].Category != "subagent" {
		t.Errorf("category = %q, want subagent", entries[1].Category)
	}
}

func TestProcessOutput_Claude_UserToolResult_UnknownID_Ignored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-utr-2")

	processJSON(s, proc, map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "toolu_unknown",
				},
			},
		},
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for unknown user tool_result id, got %d", len(msgs))
	}
}

// === Multiple in-flight Task invocations ===

func TestProcessOutput_Claude_MultipleInFlightTasks(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-multi-1")

	// Dispatch 3 Task tool_uses with different IDs
	taskIDs := []string{"toolu_a", "toolu_b", "toolu_c"}
	descs := []string{"task alpha", "task beta", "task gamma"}
	types := []string{"type-a", "type-b", "type-c"}

	for i, id := range taskIDs {
		processJSON(s, proc, map[string]interface{}{
			"type": "assistant",
			"message": map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   id,
						"name": "Task",
						"input": map[string]interface{}{
							"description":   descs[i],
							"subagent_type": types[i],
						},
					},
				},
			},
		})
	}

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()

	if taskCount != 3 {
		t.Fatalf("expected 3 pending tasks, got %d", taskCount)
	}

	// Resolve the middle task only
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_b",
	})

	proc.messagesMutex.Lock()
	taskCountAfter := len(proc.pendingTasks)
	_, hasA := proc.pendingTasks["toolu_a"]
	_, hasB := proc.pendingTasks["toolu_b"]
	_, hasC := proc.pendingTasks["toolu_c"]
	proc.messagesMutex.Unlock()

	if taskCountAfter != 2 {
		t.Fatalf("expected 2 pending tasks after resolving toolu_b, got %d", taskCountAfter)
	}
	if !hasA {
		t.Error("toolu_a should still be pending")
	}
	if hasB {
		t.Error("toolu_b should have been removed after tool_result")
	}
	if !hasC {
		t.Error("toolu_c should still be pending")
	}

	// Should have 4 messages: [Task]*3 + [TaskResult]*1
	entries := pendingEntries(proc)
	if len(entries) != 4 {
		t.Fatalf("expected 4 messages (3 Task + 1 TaskResult), got %d", len(entries))
	}

	taskResultCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Content, "[TaskResult]") {
			taskResultCount++
		}
	}
	if taskResultCount != 1 {
		t.Errorf("expected 1 [TaskResult] message, got %d", taskResultCount)
	}
}

// === [TaskResult] message format variants ===

func TestProcessOutput_Claude_TaskResult_Formats(t *testing.T) {
	tests := []struct {
		name         string
		description  string
		subagentType string
		wantContains string
	}{
		{
			name:         "with-type-and-desc",
			description:  "explore the codebase",
			subagentType: "codebase-explorer",
			wantContains: "codebase-explorer: explore the codebase",
		},
		{
			name:         "desc-only",
			description:  "run the tests",
			subagentType: "",
			wantContains: "run the tests",
		},
		{
			name:         "empty-desc-falls-back-to-completed",
			description:  "",
			subagentType: "",
			wantContains: "completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := noPoolSpawner()
			proc := minProc("sess-fmt-" + tt.name)

			input := map[string]interface{}{}
			if tt.description != "" {
				input["description"] = tt.description
			}
			if tt.subagentType != "" {
				input["subagent_type"] = tt.subagentType
			}

			processJSON(s, proc, map[string]interface{}{
				"type": "assistant",
				"message": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"type":  "tool_use",
							"id":    "toolu_fmt_" + tt.name,
							"name":  "Task",
							"input": input,
						},
					},
				},
			})

			processJSON(s, proc, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": "toolu_fmt_" + tt.name,
			})

			entries := pendingEntries(proc)
			if len(entries) != 2 {
				t.Fatalf("expected 2 entries, got %d", len(entries))
			}

			taskResultMsg := entries[1].Content
			if !strings.HasPrefix(taskResultMsg, "[TaskResult] ") {
				t.Errorf("expected [TaskResult] prefix, got: %q", taskResultMsg)
			}
			if !strings.Contains(taskResultMsg, tt.wantContains) {
				t.Errorf("expected %q in message, got: %q", tt.wantContains, taskResultMsg)
			}
		})
	}
}

func TestProcessOutput_Claude_TaskResult_LongDetail_Truncated(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-trunc-1")

	// 210-char description (exceeds 200-char limit)
	longDesc := strings.Repeat("x", 210)

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_trunc",
					"name":  "Task",
					"input": map[string]interface{}{"description": longDesc},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_trunc",
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	msg := entries[1].Content
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("expected truncation '...', got: %q", msg)
	}
	// "[TaskResult] " (13) + 200 chars + "..." (3) = 216
	const wantLen = 216
	if len(msg) != wantLen {
		t.Errorf("expected len=%d (truncated), got %d: %q", wantLen, len(msg), msg)
	}
}
