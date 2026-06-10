package spawner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
		s.errorAgent(proc, fmt.Sprintf("scanner error: %v", err))
	}
}

// processOutput processes a line of output from the agent and tracks stats.
func (s *Spawner) processOutput(proc *processInfo, line string) {
	// For backends that don't emit structured JSON, track each line as plain text.
	if proc.backend != nil && !proc.backend.ParsesStructuredOutput() {
		s.TrackMessage(proc, line, "text")
		return
	}

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
				switch itemType {
				case "text":
					text, _ := itemMap["text"].(string)
					if text != "" {
						s.handleTextMessage(proc, text)
					}
				case "thinking":
					thinking, _ := itemMap["thinking"].(string)
					if thinking != "" {
						s.handleTextMessage(proc, "[thinking] "+thinking)
					}
				case "tool_use":
					toolName, _ := itemMap["name"].(string)
					if toolName != "" {
						input, _ := itemMap["input"].(map[string]interface{})
						s.handleToolUse(proc, toolName, input)
						// Track in-flight Task/Agent invocations for tool_result correlation
						if toolName == "Task" || toolName == "Agent" {
							if id, ok := itemMap["id"].(string); ok && id != "" {
								desc, _ := input["description"].(string)
								subagentType, _ := input["subagent_type"].(string)
								bg, _ := input["run_in_background"].(bool)
								proc.messagesMutex.Lock()
								proc.pendingTasks[id] = taskInfo{
									toolName:     toolName,
									description:  desc,
									subagentType: subagentType,
									background:   bg,
								}
								proc.messagesMutex.Unlock()
							}
						}
					}
				}
			}
		}
		if message != nil {
			s.updateClaudeContext(proc, message)
		}

	case "result":
		// Skip context update — result usage is cumulative across all turns,
		// which would overestimate context window usage. Per-turn assistant
		// events already track context correctly.

	case "user":
		// Claude tool_result items arrive inside "user" events
		message, _ := data["message"].(map[string]interface{})
		if message != nil {
			content, _ := message["content"].([]interface{})
			for _, item := range content {
				if itemMap, ok := item.(map[string]interface{}); ok {
					itemType, _ := itemMap["type"].(string)
					if itemType == "tool_result" {
						s.handleClaudeToolResult(proc, itemMap)
					}
				}
			}
		}

	case "system":
		subtype, _ := data["subtype"].(string)
		if subtype == "init" {
			version, _ := data["claude_code_version"].(string)
			model, _ := data["model"].(string)
			if version != "" || model != "" {
				msg := fmt.Sprintf("[init] v%s model=%s", version, model)
				s.TrackMessage(proc, msg, "text")
				s.logAgent(proc, msg)
			}
		}

	case "rate_limit_event":
		info, _ := data["rate_limit_info"].(map[string]interface{})
		if info != nil {
			status, _ := info["status"].(string)
			limitType, _ := info["rateLimitType"].(string)
			if status != "" && status != "allowed" {
				msg := fmt.Sprintf("[rate_limit] %s %s", limitType, status)
				s.TrackMessage(proc, msg, "text")
				s.warnAgent(proc, msg)
			}
		}

	// === Codex CLI format ===
	case "thread.started":
		if threadID, ok := data["thread_id"].(string); ok && threadID != "" {
			proc.externalSessionID = threadID
			s.logAgent(proc, "Codex thread: "+threadID)
		}

	case "turn.started":
		// Turn start from codex

	case "item.started":
		// In-progress item from codex (logging only, no DB tracking)
		item, _ := data["item"].(map[string]interface{})
		if item != nil {
			itemType, _ := item["type"].(string)
			if itemType == "command_execution" {
				command, _ := item["command"].(string)
				if command != "" {
					s.logAgent(proc, "[executing] "+command)
				}
			}
		}

	case "item.completed":
		// Item completion from codex - contains messages and tool calls
		item, _ := data["item"].(map[string]interface{})
		if item != nil {
			itemType, _ := item["type"].(string)
			switch itemType {
			case "reasoning":
				text, _ := item["text"].(string)
				if text != "" {
					s.handleTextMessage(proc, "[thinking] "+text)
				}
			case "agent_message":
				text, _ := item["text"].(string)
				if text != "" {
					s.handleTextMessage(proc, text)
				}
			case "command_execution":
				command, _ := item["command"].(string)
				if command != "" {
					s.handleToolUse(proc, "Bash", map[string]interface{}{"command": command})
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

	}
}

// handleTextMessage processes plain text output.
func (s *Spawner) handleTextMessage(proc *processInfo, text string) {
	// Track full message content
	s.TrackMessage(proc, text, "text")

	s.logAgent(proc, text)
}

// handleToolUse processes tool usage from structured output.
func (s *Spawner) handleToolUse(proc *processInfo, toolName string, input map[string]interface{}) {
	toolDetail := FormatToolDetail(toolName, input)
	category := ToolCategory(toolName)

	// Track message
	s.TrackMessage(proc, toolDetail, category)

	// Log with prefix
	s.logAgent(proc, toolDetail)
}

// handleClaudeToolResult processes a Claude tool_result event and generates
// a [TaskResult] message if the tool_use_id matches a pending Task invocation.
func (s *Spawner) handleClaudeToolResult(proc *processInfo, data map[string]interface{}) {
	// Try top-level tool_use_id
	toolUseID, _ := data["tool_use_id"].(string)
	if toolUseID == "" {
		// Try nested content[0].tool_use_id
		if content, ok := data["content"].([]interface{}); ok {
			for _, item := range content {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if id, ok := itemMap["tool_use_id"].(string); ok && id != "" {
						toolUseID = id
						break
					}
				}
			}
		}
	}
	if toolUseID == "" {
		return
	}

	proc.messagesMutex.Lock()
	info, found := proc.pendingTasks[toolUseID]
	if found {
		delete(proc.pendingTasks, toolUseID)
	}
	proc.messagesMutex.Unlock()

	if !found {
		return
	}

	// Build [TaskResult] or [AgentResult] message based on original tool name
	detail := info.description
	if info.subagentType != "" {
		detail = info.subagentType + ": " + info.description
	}
	if detail == "" {
		detail = "completed"
	}
	// Truncate long details
	if len(detail) > 200 {
		detail = detail[:200] + "..."
	}
	resultTag := "TaskResult"
	if info.toolName == "Agent" {
		resultTag = "AgentResult"
	}
	msg := "[" + resultTag + "] " + detail

	s.TrackMessage(proc, msg, "subagent")

	s.logAgent(proc, msg)
}

// TrackMessage adds a message to the pending queue for DB insertion. Exported so
// that future API-mode runners (T2-T5) can feed messages from a different source
// without depending on the rest of the spawner.
func (s *Spawner) TrackMessage(proc *processInfo, msg string, category string) {
	proc.messagesMutex.Lock()
	proc.pendingMessages = append(proc.pendingMessages, repo.MessageEntry{Content: msg, Category: category})
	proc.lastMessage = msg
	proc.messagesDirty = true
	proc.lastMessageTime = s.config.Clock.Now()
	proc.hasReceivedMessage = true
	proc.messagesMutex.Unlock()
	// Feed rate-limit ring buffer outside the messages lock to avoid nested locking.
	proc.appendRecent(msg)
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

// RecordUserInput records a user-typed line for the given session through the
// normal message pipeline. Returns true if this spawner owns the session's live
// proc (TrackMessage+immediate saveMessages path), false otherwise (caller falls back).
func (s *Spawner) RecordUserInput(sessionID, text string) bool {
	s.sessionProcsMu.Lock()
	proc, found := s.sessionProcs[sessionID]
	s.sessionProcsMu.Unlock()
	if !found {
		return false
	}
	s.TrackMessage(proc, text, "user_input")
	s.saveMessages(proc)
	return true
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

// saveMessages flushes pending messages and raw output to the database.
// saveMu serializes drain+insert so concurrent flushes keep seq order.
func (s *Spawner) saveMessages(proc *processInfo) {
	proc.saveMu.Lock()
	defer proc.saveMu.Unlock()
	proc.messagesMutex.Lock()
	pending := proc.pendingMessages
	proc.pendingMessages = make([]repo.MessageEntry, 0)
	proc.messagesMutex.Unlock()

	if len(pending) == 0 {
		return
	}

	pool := s.pool()
	if pool == nil {
		return
	}

	msgRepo := repo.NewAgentMessageRepo(pool, s.config.Clock)
	_ = msgRepo.InsertBatch(proc.sessionID, pending) // best-effort message persistence

	// Broadcast messages update for real-time UI (coalesced per session)
	if proc.projectID != "" {
		now := s.config.Clock.Now()
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
