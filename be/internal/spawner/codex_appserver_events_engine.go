package spawner

import (
	"encoding/json"
	"strings"
)

// This file holds the EngineEvent construction helpers used by
// dispatchAppServerEvent (codex_appserver_events.go) to emit normalized
// events alongside the existing Sink calls. Every helper is nil-safe via
// EventEmitter.emit, so a nil emitter reproduces today's autonomous-spawn
// behavior byte-for-byte — these helpers simply become no-ops.

// deltaParams is the shared shape of item/agentMessage/delta,
// item/reasoning/textDelta, and item/reasoning/summaryTextDelta.
type deltaParams struct {
	Delta  string `json:"delta"`
	ItemID string `json:"itemId"`
}

func emitTextDeltaEvent(sessionID string, params json.RawMessage, emit EventEmitter) {
	if emit == nil {
		return
	}
	var p deltaParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	emit(EngineEvent{Type: EventTextDelta, SessionID: sessionID, ItemID: p.ItemID, Text: p.Delta})
}

func emitThinkingDeltaEvent(sessionID string, params json.RawMessage, emit EventEmitter) {
	if emit == nil {
		return
	}
	var p deltaParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	emit(EngineEvent{Type: EventThinking, SessionID: sessionID, ItemID: p.ItemID, Text: p.Delta})
}

func emitCompletedTextEvent(sessionID, text string, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventText, SessionID: sessionID, Text: text})
}

// reasoningText renders a completed `reasoning` item's text. Both codex
// protocol generations type ReasoningThreadItem.content/.summary as arrays of
// plain strings (verified against `codex app-server generate-json-schema`,
// codex-cli 0.144.1) — the {type,text}-block shape belongs to the raw
// ReasoningItem, which item/completed never carries.
func reasoningText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var parts []string
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	return strings.Join(parts, "\n")
}

// emitCompletedThinkingEvent renders a completed `reasoning` item, which
// carries `content`/`summary` arrays rather than a `text` field. Codex leaves
// both empty when reasoning summaries are off; nothing is emitted then.
func emitCompletedThinkingEvent(sessionID string, it appServerItem, emit EventEmitter) {
	if emit == nil {
		return
	}
	text := reasoningText(it.Content)
	if text == "" {
		text = reasoningText(it.Summary)
	}
	if text == "" {
		return
	}
	emit(EngineEvent{Type: EventThinking, SessionID: sessionID, Text: text})
}

func emitToolInvokeEvent(sessionID, toolName string, input map[string]any, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventToolInvoke, SessionID: sessionID, ToolName: toolName, ToolInput: input})
}

func emitToolResultEvent(sessionID, toolName, text string, isErr bool, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventToolResult, SessionID: sessionID, ToolName: toolName, Text: text, IsError: isErr})
}

func emitTurnStartedEvent(sessionID string, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventTurnStarted, SessionID: sessionID})
}

func emitTurnCompletedEvent(sessionID string, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventTurnCompleted, SessionID: sessionID})
}

func emitTokenUsageEvent(sessionID string, pct int, usage *EngineUsage, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventTokenUsage, SessionID: sessionID, ContextLeftPct: pct, Usage: usage})
}

func emitErrorEvent(sessionID, msg string, emit EventEmitter) {
	emit.emit(EngineEvent{Type: EventError, SessionID: sessionID, Text: msg, IsError: true})
}
