package socket

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// handleAgentRecordEvent processes agent.record_event socket requests from
// Claude --settings hooks. Routes each event type to a specific recorder so
// the agent_messages table captures everything visible in the Claude TUI:
// tool calls, user prompts, notifications, turn boundaries, session events.
// Completion is still signaled by `agent finished/fail/continue` — Stop only
// records turn boundaries for visibility.
func (h *Handler) handleAgentRecordEvent(ctx context.Context, req Request) Response {
	var params struct {
		Event      json.RawMessage `json:"event"`
		SessionID  string          `json:"session_id"`
		InstanceID string          `json:"instance_id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError(err.Error()))
	}
	if params.SessionID == "" {
		return MakeErrorResponse(req.ID, NewValidationError("session_id is required"))
	}

	var event map[string]interface{}
	if err := json.Unmarshal(params.Event, &event); err != nil {
		return MakeErrorResponse(req.ID, NewInvalidParamsError("invalid event JSON: "+err.Error()))
	}

	hookEventName, _ := event["hook_event_name"].(string)

	// Codex interactive sessions don't include token usage in hook payloads,
	// but every hook payload carries `transcript_path` pointing at the rollout
	// JSONL where codex writes per-turn `token_count` events. Tail-scan it so
	// we can update context_left for codex agents the same way Claude does
	// from its assistant/result events. Best-effort: silently skipped when the
	// path is absent (Claude hooks don't carry it) or unreadable.
	if pct, ok := extractCodexContextLeft(event); ok {
		projectID, ticketID, workflow, err := h.agentSvc.UpdateContextLeft(params.SessionID, pct)
		if err != nil {
			logger.Info(ctx, "record_event: codex context_update error (best-effort)", "error", err)
		} else if projectID != "" {
			service.BroadcastFromCtx(h.wsHub, ws.EventAgentContextUpdated, service.BroadcastCtx{
				ProjectID: projectID,
				TicketID:  ticketID,
				Workflow:  workflow,
			}, map[string]interface{}{
				"session_id":   params.SessionID,
				"context_left": pct,
			})
		}
	}

	switch hookEventName {
	case "PreToolUse":
		return h.recordPreToolUse(ctx, req, params.SessionID, event)
	case "PostToolUse":
		return h.recordPostToolUse(ctx, req, params.SessionID, event)
	case "PostToolUseFailure":
		return h.recordPostToolFailure(ctx, req, params.SessionID, event)
	case "UserPromptSubmit":
		return h.recordSimpleEvent(ctx, req, params.SessionID, truncate(asString(event["prompt"]), 500), "user_input")
	case "UserPromptExpansion":
		cmd := asString(event["command_name"])
		args := asString(event["command_args"])
		msg := "/" + cmd
		if args != "" {
			msg += " " + args
		}
		return h.recordSimpleEvent(ctx, req, params.SessionID, msg, "user_input")
	case "Notification":
		return h.recordSimpleEvent(ctx, req, params.SessionID, asString(event["message"]), "text")
	case "SubagentStart":
		agentType := asString(event["agent_type"])
		prompt := truncate(asString(event["prompt"]), 200)
		msg := "subagent started"
		if agentType != "" {
			msg = "[" + agentType + "] subagent started"
		}
		if prompt != "" {
			msg += ": " + prompt
		}
		return h.recordSimpleEvent(ctx, req, params.SessionID, msg, "subagent")
	case "SubagentStop":
		return h.recordSimpleEvent(ctx, req, params.SessionID, "subagent complete", "subagent")
	case "StopFailure":
		msg := "turn failed (api error)"
		if errStr := extractErrorMessage(event); errStr != "" {
			msg = "turn failed: " + truncate(errStr, 300)
		}
		return h.recordSimpleEvent(ctx, req, params.SessionID, msg, "text")
	case "PreCompact":
		trigger := asString(event["trigger"])
		msg := "context compaction"
		if trigger != "" {
			msg += " (" + trigger + ")"
		}
		return h.recordSimpleEvent(ctx, req, params.SessionID, msg, "text")
	case "SessionStart":
		// Don't record — but use as a TUI-ready signal so the spawner can
		// release the prompt-delivery wait. Idempotent on the spawner side.
		source := asString(event["source"])
		logger.Info(ctx, "record_event: SessionStart received", "session_id", params.SessionID, "source", source)
		if h.signaler != nil {
			if err := h.signaler.SignalSessionReady(params.SessionID); err != nil {
				logger.Info(ctx, "record_event: SignalSessionReady error (best-effort)", "error", err)
			}
		}
		return MakeResponse(req.ID, map[string]string{"status": "ready"})
	case "Stop":
		// Per-turn boundary. For codex sessions, flush any new
		// `event_msg/agent_message` text from the rollout JSONL into
		// agent_messages so the model's spoken output is visible. Reasoning
		// blocks are NOT extracted (codex 0.125 emits only encrypted
		// reasoning). Best-effort: silently no-op when transcript_path is
		// absent (Claude Stop) or unreadable.
		h.flushCodexAgentMessages(ctx, req, params.SessionID, event)
		return MakeResponse(req.ID, map[string]string{"status": "recorded"})
	case "SessionEnd":
		// Predictable per-session noise — ignored.
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	default:
		logger.Info(ctx, "record_event: unknown hook event", "hook_event_name", hookEventName, "session_id", params.SessionID)
		return MakeResponse(req.ID, map[string]string{"status": "ignored"})
	}
}

// asString coerces a hook field to a string. Strings pass through; numbers,
// bools, and structured payloads (objects/arrays) get a sensible textual form
// — that way schema drift in Claude's hook payloads degrades gracefully
// instead of silently dropping data.
func asString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%v", x)
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	}
}

// extractToolResultBody pulls a human-readable string out of the PostToolUse
// payload. Claude ships `tool_response` (object on current builds, string on
// older builds) and sometimes `tool_result`. For object form, prefer stdout,
// then fall back to other common content fields, then to compact JSON.
func extractToolResultBody(event map[string]interface{}) string {
	for _, key := range []string{"tool_response", "tool_result"} {
		switch v := event[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]interface{}:
			for _, k := range []string{"stdout", "output", "content", "response", "text", "result"} {
				if s, ok := v[k].(string); ok && s != "" {
					return s
				}
			}
			if errStr, ok := v["stderr"].(string); ok && errStr != "" {
				return "stderr: " + errStr
			}
			if b, err := json.Marshal(v); err == nil {
				return string(b)
			}
		}
	}
	return ""
}

// truncate caps s at n bytes, appending "…" when over.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// recordPostToolFailure inserts a "[Tool failed] <error>" row when a tool
// invocation errored. PostToolUse only fires on success per Claude docs, so
// we'd otherwise miss tool failures entirely.
func (h *Handler) recordPostToolFailure(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	category := spawner.ToolCategory(toolName)
	content := "[" + toolName + " failed]"
	if msg := extractErrorMessage(event); msg != "" {
		content += " " + truncate(msg, 300)
	} else if body := extractToolResultBody(event); body != "" {
		content += " " + truncate(body, 300)
	}
	return h.recordSimpleEvent(ctx, req, sessionID, content, category)
}

// extractErrorMessage pulls a human-readable error string from an event.
// Claude error fields appear as strings on some hooks, structured objects
// (with `message` / `code` / `type` fields) on others.
func extractErrorMessage(event map[string]interface{}) string {
	switch v := event["error"].(type) {
	case string:
		return v
	case map[string]interface{}:
		for _, k := range []string{"message", "msg", "detail", "description"} {
			if s, ok := v[k].(string); ok && s != "" {
				return s
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return ""
}

// flushCodexAgentMessages reads new `event_msg/agent_message` records from
// the codex rollout JSONL referenced by event["transcript_path"] and inserts
// each as an agent_messages row (category=text). Tracks per-session byte
// offsets so subsequent Stop fires only emit new text. Best-effort: errors
// are logged at INFO and do not fail the response.
func (h *Handler) flushCodexAgentMessages(ctx context.Context, req Request, sessionID string, event map[string]interface{}) {
	path, _ := event["transcript_path"].(string)
	if path == "" {
		return
	}

	h.codexJSONLMu.Lock()
	startOffset := h.codexJSONLOffsets[sessionID]
	h.codexJSONLMu.Unlock()

	msgs, newOffset, err := extractCodexNewAgentMessages(path, startOffset)
	if err != nil {
		logger.Info(ctx, "record_event: codex jsonl scan error (best-effort)", "error", err, "session_id", sessionID)
		return
	}

	h.codexJSONLMu.Lock()
	h.codexJSONLOffsets[sessionID] = newOffset
	h.codexJSONLMu.Unlock()

	for _, body := range msgs {
		// recordSimpleEvent broadcasts messages.updated and bumps stall
		// detection per row, mirroring the Pre/PostToolUse path.
		h.recordSimpleEvent(ctx, req, sessionID, truncate(body, 2000), "text")
	}
}

// recordSimpleEvent inserts a single agent_messages row with the given content +
// category, broadcasts messages.updated, and bumps stall detection.
func (h *Handler) recordSimpleEvent(ctx context.Context, req Request, sessionID, content, category string) Response {
	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category)
	if err != nil {
		logger.Error(ctx, "record_event: failed to record hook message", "error", err, "content", content)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}
	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
	}
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
	}
	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

func (h *Handler) recordPreToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	toolInput, _ := event["tool_input"].(map[string]interface{})

	content := spawner.FormatToolDetail(toolName, toolInput)
	category := spawner.ToolCategory(toolName)

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category)
	if err != nil {
		logger.Error(ctx, "record_event: failed to record pre-tool message", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	// Bump stall detection (best-effort)
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
	}

	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

func (h *Handler) recordPostToolUse(ctx context.Context, req Request, sessionID string, event map[string]interface{}) Response {
	toolName, _ := event["tool_name"].(string)
	category := spawner.ToolCategory(toolName)

	content := "[" + toolName + " result]"
	if body := extractToolResultBody(event); body != "" {
		content = "[" + toolName + " result] " + truncate(body, 200)
	}

	projectID, ticketID, workflowName, err := h.agentSvc.RecordHookMessage(sessionID, content, category)
	if err != nil {
		logger.Error(ctx, "record_event: failed to record post-tool message", "error", err)
		return MakeErrorResponse(req.ID, NewInternalError(err.Error()))
	}

	if projectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
	}

	// Bump stall detection (best-effort)
	if h.signaler != nil {
		if sigErr := h.signaler.BumpLastMessage(projectID, ticketID, workflowName, sessionID); sigErr != nil {
			logger.Info(ctx, "record_event: BumpLastMessage error (best-effort)", "error", sigErr)
		}
	}

	return MakeResponse(req.ID, map[string]string{"status": "recorded"})
}

