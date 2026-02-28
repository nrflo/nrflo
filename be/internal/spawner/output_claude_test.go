package spawner

import (
	"strings"
	"testing"

	"be/internal/repo"
)

// pendingEntries returns the full MessageEntry slice (content + category) under the lock.
func pendingEntries(proc *processInfo) []repo.MessageEntry {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	out := make([]repo.MessageEntry, len(proc.pendingMessages))
	copy(out, proc.pendingMessages)
	return out
}

// === toolCategory ===

func TestToolCategory(t *testing.T) {
	tests := []struct {
		toolName string
		want     string
	}{
		{"Task", "subagent"},
		{"Agent", "subagent"},
		{"Skill", "skill"},
		{"Bash", "tool"},
		{"Read", "tool"},
		{"Write", "tool"},
		{"Edit", "tool"},
		{"Glob", "tool"},
		{"Grep", "tool"},
		{"WebFetch", "tool"},
		{"WebSearch", "tool"},
		{"", "tool"},
		{"Unknown", "tool"},
		{"TodoWrite", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := toolCategory(tt.toolName)
			if got != tt.want {
				t.Errorf("toolCategory(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

// === Category assignment from Claude assistant events ===

func TestProcessOutput_Claude_CategoryAssignment(t *testing.T) {
	tests := []struct {
		name     string
		toolName string // empty = text message
		wantCat  string
	}{
		{"text-content", "", "text"},
		{"bash", "Bash", "tool"},
		{"read", "Read", "tool"},
		{"write", "Write", "tool"},
		{"task", "Task", "subagent"},
		{"agent", "Agent", "subagent"},
		{"skill", "Skill", "skill"},
		{"grep", "Grep", "tool"},
		{"glob", "Glob", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := noPoolSpawner()
			proc := minProc("sess-ca-" + tt.name)

			if tt.toolName == "" {
				processJSON(s, proc, map[string]interface{}{
					"type": "assistant",
					"message": map[string]interface{}{
						"content": []interface{}{
							map[string]interface{}{"type": "text", "text": "analysis complete"},
						},
					},
				})
			} else {
				input := buildToolInput(tt.toolName)
				processJSON(s, proc, map[string]interface{}{
					"type": "assistant",
					"message": map[string]interface{}{
						"content": []interface{}{
							map[string]interface{}{
								"type":  "tool_use",
								"id":    "toolu_ca_" + tt.toolName,
								"name":  tt.toolName,
								"input": input,
							},
						},
					},
				})
			}

			entries := pendingEntries(proc)
			if len(entries) == 0 {
				t.Fatalf("expected at least 1 entry, got 0")
			}
			if entries[0].Category != tt.wantCat {
				t.Errorf("category = %q, want %q", entries[0].Category, tt.wantCat)
			}
		})
	}
}

// buildToolInput returns a minimal input map for the given tool name.
func buildToolInput(toolName string) map[string]interface{} {
	switch toolName {
	case "Bash":
		return map[string]interface{}{"command": "ls"}
	case "Read", "Write", "Edit":
		return map[string]interface{}{"file_path": "file.go"}
	case "Task", "Agent":
		return map[string]interface{}{"description": "analyze the code"}
	case "Skill":
		return map[string]interface{}{"skill": "commit"}
	case "Grep":
		return map[string]interface{}{"pattern": "TODO"}
	case "Glob":
		return map[string]interface{}{"pattern": "*.go"}
	default:
		return map[string]interface{}{}
	}
}

// === pendingTasks tracking ===

func TestProcessOutput_Claude_TaskToolUse_TracksPendingTask(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-pending-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_abc123",
					"name": "Task",
					"input": map[string]interface{}{
						"description":       "do something useful",
						"subagent_type":     "general-purpose",
						"run_in_background": true,
					},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	info, found := proc.pendingTasks["toolu_abc123"]
	proc.messagesMutex.Unlock()

	if !found {
		t.Fatal("expected pendingTasks[toolu_abc123] to be set")
	}
	if info.description != "do something useful" {
		t.Errorf("description = %q, want %q", info.description, "do something useful")
	}
	if info.subagentType != "general-purpose" {
		t.Errorf("subagentType = %q, want %q", info.subagentType, "general-purpose")
	}
	if !info.background {
		t.Error("background should be true")
	}
}

func TestProcessOutput_Claude_NonTaskToolUse_NoPendingEntry(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-pending-2")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_bash1",
					"name":  "Bash",
					"input": map[string]interface{}{"command": "ls"},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()

	if taskCount != 0 {
		t.Errorf("pendingTasks should be empty for non-Task tool, got %d entries", taskCount)
	}
}

func TestProcessOutput_Claude_TaskToolUse_MissingID_NoPendingEntry(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-pending-3")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					// no "id" field
					"name":  "Task",
					"input": map[string]interface{}{"description": "some task"},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()

	if taskCount != 0 {
		t.Errorf("pendingTasks should be empty when Task has no id, got %d entries", taskCount)
	}
	// But a [Task] message should still be created
	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for Task without id, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "[Task]") {
		t.Errorf("expected [Task] message, got: %q", msgs[0])
	}
}

// === tool_result correlation ===

func TestProcessOutput_Claude_ToolResult_MatchingID_GeneratesTaskResult(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tr-1")

	// Register a pending Task via assistant event
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_tr001",
					"name": "Task",
					"input": map[string]interface{}{
						"description":   "write tests",
						"subagent_type": "test-runner",
					},
				},
			},
		},
	})

	// Tool result arrives with matching tool_use_id
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_tr001",
		"content":     "Tests passed",
	})

	entries := pendingEntries(proc)
	// Should have 2: [Task] and [TaskResult]
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Task + TaskResult), got %d: %v", len(entries), entries)
	}

	taskResult := entries[1]
	if !strings.HasPrefix(taskResult.Content, "[TaskResult]") {
		t.Errorf("expected [TaskResult] prefix, got: %q", taskResult.Content)
	}
	if taskResult.Category != "subagent" {
		t.Errorf("TaskResult category = %q, want %q", taskResult.Category, "subagent")
	}
	if !strings.Contains(taskResult.Content, "test-runner: write tests") {
		t.Errorf("TaskResult should contain 'test-runner: write tests', got: %q", taskResult.Content)
	}
}

func TestProcessOutput_Claude_ToolResult_MatchingID_RemovesPendingTask(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tr-2")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_tr002",
					"name":  "Task",
					"input": map[string]interface{}{"description": "do work"},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_tr002",
	})

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()

	if taskCount != 0 {
		t.Errorf("pendingTasks should be empty after tool_result, got %d entries", taskCount)
	}
}

func TestProcessOutput_Claude_ToolResult_UnknownID_Ignored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tr-3")

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_unknown999",
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for unknown tool_use_id, got %d: %v", len(msgs), msgs)
	}
}

func TestProcessOutput_Claude_ToolResult_EmptyID_Ignored(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-tr-4")

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "",
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for empty tool_use_id, got %d: %v", len(msgs), msgs)
	}
}
