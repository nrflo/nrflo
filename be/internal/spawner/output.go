package spawner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"be/internal/repo"
	"be/internal/ws"
)

// monitorOutput reads stdout and tracks messages/stats for a process
func (s *Spawner) monitorOutput(proc *processInfo, stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	// Increase buffer size to 10MB for large JSON outputs (file reads, diffs, etc.)
	const maxScannerBuffer = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxScannerBuffer)

	for scanner.Scan() {
		line := scanner.Text()
		s.processOutput(proc, line)
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("  [ERROR] Scanner error: %v\n", err)
	}
}

// processOutput processes a line of output from the agent and tracks stats
// Handles both Claude CLI and opencode JSON formats
func (s *Spawner) processOutput(proc *processInfo, line string) {
	// Try to parse as JSON (stream-json format)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		// Not JSON, skip
		return
	}

	// Extract message based on type
	eventType, _ := data["type"].(string)
	switch eventType {

	// === Claude CLI format ===
	case "assistant":
		message, _ := data["message"].(map[string]interface{})
		content, _ := message["content"].([]interface{})
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)
				if itemType == "text" {
					text, _ := itemMap["text"].(string)
					if text != "" {
						s.handleTextMessage(proc, text)
					}
				} else if itemType == "tool_use" {
					toolName, _ := itemMap["name"].(string)
					if toolName != "" {
						input, _ := itemMap["input"].(map[string]interface{})
						s.handleToolUse(proc, toolName, input)
					}
				}
			}
		}

	case "result":
		// Result subtype tracked via messages only

	// === Opencode format ===
	case "text":
		// Text content from opencode
		part, _ := data["part"].(map[string]interface{})
		if part != nil {
			text, _ := part["text"].(string)
			if text != "" {
				s.handleTextMessage(proc, text)
			}
		}

	case "tool_use":
		// Tool execution from opencode
		part, _ := data["part"].(map[string]interface{})
		if part != nil {
			toolName, _ := part["tool"].(string)
			if toolName != "" {
				// Opencode puts input under state.input, not part.input
				var input map[string]interface{}
				state, _ := part["state"].(map[string]interface{})
				if state != nil {
					input, _ = state["input"].(map[string]interface{})
				}
				// Fallback to part.input if state.input not found
				if input == nil {
					input, _ = part["input"].(map[string]interface{})
				}
				s.handleToolUse(proc, toolName, input)
			}
		}

	case "tool_result":
		// Tool result from opencode

	case "step_finish":
		// Step completion from opencode

	case "finish":
		// Session finish from opencode

	// === Codex CLI format ===
	case "thread.started":
		// Session start from codex

	case "turn.started":
		// Turn start from codex

	case "item.completed":
		// Item completion from codex - contains messages and tool calls
		item, _ := data["item"].(map[string]interface{})
		if item != nil {
			itemType, _ := item["type"].(string)
			switch itemType {
			case "agent_message":
				text, _ := item["text"].(string)
				if text != "" {
					s.handleTextMessage(proc, text)
				}
			case "tool_call":
				toolName, _ := item["name"].(string)
				if toolName != "" {
					args, _ := item["arguments"].(map[string]interface{})
					s.handleToolUse(proc, toolName, args)
				}
			case "tool_result":
				// Tool result from codex
			}
		}

	case "turn.completed":
		// Turn completion from codex
	}
}

// handleTextMessage processes text output from either Claude or opencode
func (s *Spawner) handleTextMessage(proc *processInfo, text string) {
	// Track full message content
	s.trackMessage(proc, text)

	// Print to console with truncation for long messages
	prefix := s.formatPrefix(proc)
	maxLen := 500
	if len(text) <= maxLen {
		fmt.Printf("  %s %s\n", prefix, text)
	} else {
		// Show start + ... + end for context
		startLen := 300
		endLen := 150
		fmt.Printf("  %s %s\n  ... [%d chars truncated] ...\n  %s\n", prefix, text[:startLen], len(text)-startLen-endLen, text[len(text)-endLen:])
	}
}

// handleToolUse processes tool usage from either Claude or opencode
func (s *Spawner) handleToolUse(proc *processInfo, toolName string, input map[string]interface{}) {
	toolDetail := s.formatToolDetail(toolName, input)

	// Track message
	s.trackMessage(proc, toolDetail)

	// Print to console with prefix
	prefix := s.formatPrefix(proc)
	fmt.Printf("  %s %s\n", prefix, toolDetail)
}

// trackMessage adds a message to the pending queue for DB insertion
func (s *Spawner) trackMessage(proc *processInfo, msg string) {
	proc.messagesMutex.Lock()
	defer proc.messagesMutex.Unlock()
	proc.pendingMessages = append(proc.pendingMessages, msg)
	proc.lastMessage = msg
	proc.messagesDirty = true
}

// formatPrefix returns a prefix string with agent type and model for console output
func (s *Spawner) formatPrefix(proc *processInfo) string {
	// Parse model from modelID (cli:model format)
	_, model := parseModelID(proc.modelID)
	if model == "" {
		model = "default"
	}
	return fmt.Sprintf("[%s:%s]", proc.agentType, model)
}

// formatToolDetail extracts relevant details from tool input based on tool type
func (s *Spawner) formatToolDetail(toolName string, input map[string]interface{}) string {
	// Normalize tool name to title case (opencode sends lowercase, Claude sends capitalized)
	if len(toolName) > 0 {
		toolName = strings.ToUpper(toolName[:1]) + toolName[1:]
	}

	if input == nil {
		return "[" + toolName + "]"
	}

	var detail string

	switch toolName {
	case "Skill":
		// Claude uses "skill", opencode uses "name"
		skillName, _ := input["skill"].(string)
		if skillName == "" {
			skillName, _ = input["name"].(string)
		}
		skillArgs, _ := input["args"].(string)
		if skillName != "" {
			detail = "skill:" + skillName
			if skillArgs != "" {
				detail += " " + skillArgs
			}
		}

	case "Bash":
		cmd, _ := input["command"].(string)
		if cmd != "" {
			detail = cmd
		}

	case "Read":
		// Try both snake_case (Claude) and camelCase (opencode)
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Write":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Edit":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Glob":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail = path + "/" + pattern
			}
		}

	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail += " in " + path
			}
		}

	case "Task":
		desc, _ := input["description"].(string)
		agentType, _ := input["subagent_type"].(string)
		if desc != "" {
			detail = desc
			if agentType != "" {
				detail = agentType + ": " + desc
			}
		}

	case "WebFetch":
		url, _ := input["url"].(string)
		if url != "" {
			detail = url
		}

	case "WebSearch":
		query, _ := input["query"].(string)
		if query != "" {
			detail = query
		}

	case "TodoWrite", "TaskCreate", "TaskUpdate", "TaskList":
		// Just show tool name for task management tools
		return "[" + toolName + "]"
	}

	// Format output: [ToolName] detail (truncated if needed)
	if detail == "" {
		return "[" + toolName + "]"
	}

	// Truncate long details
	maxLen := 200
	if len(detail) > maxLen {
		detail = detail[:maxLen] + "..."
	}

	return "[" + toolName + "] " + detail
}

// broadcastCoalesceWindow is the minimum interval between messages.updated broadcasts per session.
const broadcastCoalesceWindow = 2 * time.Second

// lastBroadcastMu protects lastBroadcastPerSession.
var lastBroadcastMu sync.Mutex

// lastBroadcastPerSession tracks the last broadcast time per session for coalescing.
var lastBroadcastPerSession = make(map[string]time.Time)

// cleanupBroadcastCoalescing removes entries for completed sessions.
func cleanupBroadcastCoalescing(completed []*processInfo) {
	lastBroadcastMu.Lock()
	for _, proc := range completed {
		delete(lastBroadcastPerSession, proc.sessionID)
	}
	lastBroadcastMu.Unlock()
}

// saveMessages flushes pending messages and raw output to the database
func (s *Spawner) saveMessages(proc *processInfo) {
	// Drain pending messages
	proc.messagesMutex.Lock()
	pending := proc.pendingMessages
	proc.pendingMessages = make([]string, 0)
	seqStart := proc.nextSeq
	proc.nextSeq += len(pending)
	proc.messagesMutex.Unlock()

	if len(pending) == 0 {
		return
	}

	pool := s.pool()
	if pool == nil {
		return
	}

	msgRepo := repo.NewAgentMessageRepo(pool)
	msgRepo.InsertBatch(proc.sessionID, seqStart, pending)

	// Broadcast messages update for real-time UI (coalesced per session)
	if proc.projectID != "" {
		now := time.Now()
		lastBroadcastMu.Lock()
		last := lastBroadcastPerSession[proc.sessionID]
		shouldBroadcast := now.Sub(last) >= broadcastCoalesceWindow
		if shouldBroadcast {
			lastBroadcastPerSession[proc.sessionID] = now
		}
		lastBroadcastMu.Unlock()

		if shouldBroadcast {
			s.broadcast(ws.EventMessagesUpdated, proc.projectID, proc.ticketID, proc.workflowName, map[string]interface{}{
				"session_id": proc.sessionID,
				"agent_type": proc.agentType,
				"model_id":   proc.modelID,
			})
		}
	}
}
