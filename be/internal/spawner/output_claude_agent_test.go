package spawner

import (
	"strings"
	"testing"
)

// TestProcessOutput_Claude_AgentToolUse_TracksPendingTask mirrors the Task version for "Agent" tool.
func TestProcessOutput_Claude_AgentToolUse_TracksPendingTask(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-pending-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_agent_abc123",
					"name": "Agent",
					"input": map[string]interface{}{
						"description":       "explore the repo",
						"subagent_type":     "codebase-explorer",
						"run_in_background": false,
					},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	info, found := proc.pendingTasks["toolu_agent_abc123"]
	proc.messagesMutex.Unlock()

	if !found {
		t.Fatal("expected pendingTasks[toolu_agent_abc123] to be set for Agent tool")
	}
	if info.description != "explore the repo" {
		t.Errorf("description = %q, want %q", info.description, "explore the repo")
	}
	if info.subagentType != "codebase-explorer" {
		t.Errorf("subagentType = %q, want %q", info.subagentType, "codebase-explorer")
	}
	if info.background {
		t.Error("background should be false")
	}
}

// TestProcessOutput_Claude_AgentToolUse_StoresToolNameInTaskInfo verifies toolName="Agent" is stored.
func TestProcessOutput_Claude_AgentToolUse_StoresToolNameInTaskInfo(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-toolname-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_agent_tn1",
					"name":  "Agent",
					"input": map[string]interface{}{"description": "test desc"},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	info, found := proc.pendingTasks["toolu_agent_tn1"]
	proc.messagesMutex.Unlock()

	if !found {
		t.Fatal("expected pendingTasks entry for Agent tool_use")
	}
	if info.toolName != "Agent" {
		t.Errorf("toolName = %q, want %q", info.toolName, "Agent")
	}
}

// TestProcessOutput_Claude_TaskToolUse_StoresToolNameInTaskInfo verifies toolName="Task" is stored.
func TestProcessOutput_Claude_TaskToolUse_StoresToolNameInTaskInfo(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-task-toolname-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_task_tn1",
					"name":  "Task",
					"input": map[string]interface{}{"description": "task desc"},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	info, found := proc.pendingTasks["toolu_task_tn1"]
	proc.messagesMutex.Unlock()

	if !found {
		t.Fatal("expected pendingTasks entry for Task tool_use")
	}
	if info.toolName != "Task" {
		t.Errorf("toolName = %q, want %q", info.toolName, "Task")
	}
}

// TestProcessOutput_Claude_AgentToolUse_MissingID_NoPendingEntry mirrors the Task version.
func TestProcessOutput_Claude_AgentToolUse_MissingID_NoPendingEntry(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-pending-2")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					// no "id" field
					"name":  "Agent",
					"input": map[string]interface{}{"description": "some agent task"},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()

	if taskCount != 0 {
		t.Errorf("pendingTasks should be empty when Agent has no id, got %d entries", taskCount)
	}
	// But an [Agent] message should still be created
	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for Agent without id, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "[Agent]") {
		t.Errorf("expected [Agent] message, got: %q", msgs[0])
	}
}

// TestProcessOutput_Claude_ToolResult_MatchingAgent_GeneratesAgentResult verifies [AgentResult] prefix.
func TestProcessOutput_Claude_ToolResult_MatchingAgent_GeneratesAgentResult(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-tr-1")

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_agent_tr001",
					"name": "Agent",
					"input": map[string]interface{}{
						"description":   "write tests",
						"subagent_type": "test-runner",
					},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_agent_tr001",
		"content":     "Tests passed",
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Agent + AgentResult), got %d: %v", len(entries), entries)
	}

	agentResult := entries[1]
	if !strings.HasPrefix(agentResult.Content, "[AgentResult]") {
		t.Errorf("expected [AgentResult] prefix, got: %q", agentResult.Content)
	}
	if agentResult.Category != "subagent" {
		t.Errorf("AgentResult category = %q, want %q", agentResult.Category, "subagent")
	}
	if !strings.Contains(agentResult.Content, "test-runner: write tests") {
		t.Errorf("AgentResult should contain 'test-runner: write tests', got: %q", agentResult.Content)
	}
}

// TestProcessOutput_Claude_AgentResult_Formats tests format variants for Agent tool results.
func TestProcessOutput_Claude_AgentResult_Formats(t *testing.T) {
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
			proc := minProc("sess-agentfmt-" + tt.name)

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
							"id":    "toolu_agentfmt_" + tt.name,
							"name":  "Agent",
							"input": input,
						},
					},
				},
			})

			processJSON(s, proc, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": "toolu_agentfmt_" + tt.name,
			})

			entries := pendingEntries(proc)
			if len(entries) != 2 {
				t.Fatalf("expected 2 entries, got %d", len(entries))
			}

			agentResultMsg := entries[1].Content
			if !strings.HasPrefix(agentResultMsg, "[AgentResult] ") {
				t.Errorf("expected [AgentResult] prefix, got: %q", agentResultMsg)
			}
			if !strings.Contains(agentResultMsg, tt.wantContains) {
				t.Errorf("expected %q in message, got: %q", tt.wantContains, agentResultMsg)
			}
		})
	}
}

// TestProcessOutput_Claude_AgentResult_LongDetail_Truncated verifies truncation at 200 chars.
func TestProcessOutput_Claude_AgentResult_LongDetail_Truncated(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-agent-trunc-1")

	longDesc := strings.Repeat("y", 210)

	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type":  "tool_use",
					"id":    "toolu_agent_trunc",
					"name":  "Agent",
					"input": map[string]interface{}{"description": longDesc},
				},
			},
		},
	})

	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_agent_trunc",
	})

	entries := pendingEntries(proc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	msg := entries[1].Content
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("expected truncation '...', got: %q", msg)
	}
	// "[AgentResult] " (14) + 200 chars + "..." (3) = 217
	const wantLen = 217
	if len(msg) != wantLen {
		t.Errorf("expected len=%d (truncated), got %d: %q", wantLen, len(msg), msg)
	}
}

// TestProcessOutput_Claude_TaskInfo_EmptyToolName_DefaultsToTaskResult verifies backward compat:
// a taskInfo with empty toolName (zero value) produces [TaskResult], not [AgentResult].
func TestProcessOutput_Claude_TaskInfo_EmptyToolName_DefaultsToTaskResult(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-compat-1")

	// Directly insert a taskInfo with empty toolName (simulates pre-existing entries before the toolName field)
	proc.messagesMutex.Lock()
	proc.pendingTasks["toolu_compat001"] = taskInfo{
		toolName:     "", // zero value — backward compat
		description:  "old style task",
		subagentType: "",
		background:   false,
	}
	proc.messagesMutex.Unlock()

	// Trigger result
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_compat001",
	})

	msgs := pendingMessages(proc)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message ([TaskResult]), got %d: %v", len(msgs), msgs)
	}
	if !strings.HasPrefix(msgs[0], "[TaskResult]") {
		t.Errorf("empty toolName should produce [TaskResult], got: %q", msgs[0])
	}
	if strings.HasPrefix(msgs[0], "[AgentResult]") {
		t.Errorf("empty toolName must NOT produce [AgentResult], got: %q", msgs[0])
	}
}

// TestProcessOutput_Claude_MixedTaskAndAgent_CorrectPrefixes verifies both tool types in-flight
// simultaneously produce the correct result prefix independently.
func TestProcessOutput_Claude_MixedTaskAndAgent_CorrectPrefixes(t *testing.T) {
	s := noPoolSpawner()
	proc := minProc("sess-mixed-1")

	// Dispatch a Task tool_use
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_task_mixed",
					"name": "Task",
					"input": map[string]interface{}{
						"description":   "old task style",
						"subagent_type": "general-purpose",
					},
				},
			},
		},
	})

	// Dispatch an Agent tool_use
	processJSON(s, proc, map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_agent_mixed",
					"name": "Agent",
					"input": map[string]interface{}{
						"description":   "new agent style",
						"subagent_type": "codebase-explorer",
					},
				},
			},
		},
	})

	proc.messagesMutex.Lock()
	taskCount := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()
	if taskCount != 2 {
		t.Fatalf("expected 2 pending tasks, got %d", taskCount)
	}

	// Resolve Task tool_use
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_task_mixed",
	})

	// Resolve Agent tool_use
	processJSON(s, proc, map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": "toolu_agent_mixed",
	})

	entries := pendingEntries(proc)
	// Should have 4: [Task], [Agent], [TaskResult], [AgentResult]
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %v", len(entries), entries)
	}

	// Verify categories
	for _, e := range entries {
		if e.Category != "subagent" {
			t.Errorf("entry %q category = %q, want %q", e.Content, e.Category, "subagent")
		}
	}

	// Find [TaskResult] and [AgentResult] among entries
	var taskResultFound, agentResultFound bool
	for _, e := range entries {
		if strings.HasPrefix(e.Content, "[TaskResult]") {
			taskResultFound = true
		}
		if strings.HasPrefix(e.Content, "[AgentResult]") {
			agentResultFound = true
		}
	}
	if !taskResultFound {
		t.Error("expected [TaskResult] message in entries")
	}
	if !agentResultFound {
		t.Error("expected [AgentResult] message in entries")
	}

	// Both task entries should be cleaned from pendingTasks
	proc.messagesMutex.Lock()
	remaining := len(proc.pendingTasks)
	proc.messagesMutex.Unlock()
	if remaining != 0 {
		t.Errorf("pendingTasks should be empty after both results, got %d", remaining)
	}
}
