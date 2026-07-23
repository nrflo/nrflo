package spawner

import (
	"encoding/json"
	"fmt"
	"strings"
)

// appServerSignal is the control hint dispatchAppServerEvent returns to the run
// loop. The mapper is pure (no proc/projectID): side effects that need proc
// context (RecordError, rate-limit dance, nudge) are driven by these flags.
type appServerSignal struct {
	turnStarted   bool
	turnCompleted bool
	rateLimited   bool   // typed limit reached or an error classified as a limit
	matchedReason string // pattern/text that triggered rateLimited
	fatalErr      string // non-limit error message (turn/error or `error` notification)
	compacted     bool   // codex-side contextCompaction item completed
}

// dispatchAppServerEvent maps ONE app-server notification to Sink calls and
// returns control hints. maxCtx is the fallback model context window when the
// event omits modelContextWindow. Every event bumps the stall heartbeat. emit
// is an optional normalized-event listener (console sessions); nil reproduces
// today's autonomous sink-only behavior byte-for-byte — see EventEmitter.emit.
func dispatchAppServerEvent(sessionID string, env rpcEnvelope, sink Sink, maxCtx int, emit EventEmitter) appServerSignal {
	var sig appServerSignal
	switch env.Method {
	case "item/completed":
		sig.compacted = dispatchCompletedItem(sessionID, env.Params, sink, emit)
	case "item/agentMessage/delta":
		// Streamed text; the canonical full text arrives via item/completed
		// (agentMessage). Deltas keep the heartbeat alive and, when a console
		// emitter is attached, stream live.
		sink.BumpLastMessage(sessionID)
		emitTextDeltaEvent(sessionID, env.Params, emit)
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		sink.BumpLastMessage(sessionID)
		emitThinkingDeltaEvent(sessionID, env.Params, emit)
	case "thread/tokenUsage/updated":
		dispatchTokenUsage(sessionID, env.Params, sink, maxCtx, emit)
	case "turn/started":
		sink.BumpLastMessage(sessionID)
		sig.turnStarted = true
		emitTurnStartedEvent(sessionID, emit)
	case "turn/completed":
		sink.OnTurnComplete(sessionID)
		sig.turnCompleted = true
		if msg := turnCompletedError(env.Params); msg != "" {
			classifyAppServerError(msg, &sig)
			emitErrorEvent(sessionID, msg, emit)
		}
		emitTurnCompletedEvent(sessionID, emit)
	case "error":
		var p struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(env.Params, &p)
		classifyAppServerError(p.Error.Message, &sig)
		emitErrorEvent(sessionID, p.Error.Message, emit)
	case "account/rateLimits/updated":
		if rateLimitReached(env.Params) {
			sig.rateLimited = true
			sig.matchedReason = "rate_limit_reached"
		}
		sink.BumpLastMessage(sessionID)
	default:
		sink.BumpLastMessage(sessionID)
	}
	return sig
}

// appServerItem is the union of item fields we consume across item types.
// Content/Summary stay raw because the same field NAME carries different
// shapes per item type (reasoning: string array; userMessage: input blocks) —
// a strictly-typed field would make json.Unmarshal fail for the other type and
// dispatchCompletedItem's error guard would then silently drop the whole item.
type appServerItem struct {
	Type             string             `json:"type"`
	Text             string             `json:"text"`             // agentMessage
	Content          json.RawMessage    `json:"content"`          // reasoning, userMessage
	Summary          json.RawMessage    `json:"summary"`          // reasoning
	Command          string             `json:"command"`          // commandExecution
	AggregatedOutput string             `json:"aggregatedOutput"` // commandExecution
	ExitCode         *int               `json:"exitCode"`         // commandExecution
	Server           string             `json:"server"`           // mcpToolCall
	Tool             string             `json:"tool"`             // mcpToolCall
	Arguments        json.RawMessage    `json:"arguments"`        // mcpToolCall
	Result           *mcpToolCallResult `json:"result"`           // mcpToolCall
	Error            *mcpToolCallError  `json:"error"`            // mcpToolCall
	Query            string             `json:"query"`            // webSearch
}

// mcpToolCallResult mirrors the codex app-server McpToolCallResult shape
// (`codex app-server generate-json-schema`): an array of content blocks plus
// optional structured content.
type mcpToolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

type mcpToolCallError struct {
	Message string `json:"message"`
}

// dispatchCompletedItem maps one item/completed notification to Sink/emit
// calls and reports whether the item was a contextCompaction — the run
// loop's single trigger for resetting proc's in-memory context_left watermark
// (item/started fires BEFORE the usage checkpoint, item/completed after).
func dispatchCompletedItem(sessionID string, params json.RawMessage, sink Sink, emit EventEmitter) bool {
	var p struct {
		Item appServerItem `json:"item"`
	}
	if json.Unmarshal(params, &p) != nil {
		sink.BumpLastMessage(sessionID)
		return false
	}
	switch p.Item.Type {
	case "agentMessage":
		emitAgentText(sessionID, p.Item.Text, sink)
		emitCompletedTextEvent(sessionID, p.Item.Text, emit)
	case "reasoning":
		sink.BumpLastMessage(sessionID)
		emitCompletedThinkingEvent(sessionID, p.Item, emit)
	case "commandExecution":
		cmdInput, _ := json.Marshal(map[string]string{"command": p.Item.Command})
		emitMessageWithPayload(sessionID, formatAppServerCommand(p.Item), "tool", BuildToolInvokePayload("", cmdInput), sink)
		emitToolInvokeEvent(sessionID, "Bash", map[string]any{"command": p.Item.Command}, emit)
		emitToolResultEvent(sessionID, "Bash", p.Item.AggregatedOutput, p.Item.ExitCode != nil && *p.Item.ExitCode != 0, emit)
	case "mcpToolCall":
		emitMcpToolCall(sessionID, p.Item, sink, emit)
	case "webSearch":
		queryInput, _ := json.Marshal(map[string]string{"query": p.Item.Query})
		emitMessageWithPayload(sessionID, FormatToolDetail("WebSearch", map[string]interface{}{"query": p.Item.Query}), "tool", BuildToolInvokePayload("", queryInput), sink)
		emitToolInvokeEvent(sessionID, "WebSearch", map[string]any{"query": p.Item.Query}, emit)
	case "contextCompaction":
		dispatchContextCompaction(sessionID, sink, emit)
		return true
	default:
		// userMessage, fileChange summaries, etc. — heartbeat only.
		sink.BumpLastMessage(sessionID)
	}
	return false
}

// emitMcpToolCall renders a codex mcpToolCall item (e.g. an nrflo MCP tool) as
// an invoke row plus a result row, matching the Claude-hook + api-mode shape.
// The name is normalized to "mcp__<server>__<tool>" so the same nrflo tool reads
// identically across providers. item/completed carries both the arguments and
// the result/error, so both rows come from this one event.
func emitMcpToolCall(sessionID string, it appServerItem, sink Sink, emit EventEmitter) {
	name := "mcp__" + it.Server + "__" + it.Tool
	var args map[string]interface{}
	if len(it.Arguments) > 0 {
		_ = json.Unmarshal(it.Arguments, &args)
	}
	emitMessageWithPayload(sessionID, FormatToolDetail(name, args), ToolCategory(name), BuildToolInvokePayload("", it.Arguments), sink)
	emitToolInvokeEvent(sessionID, name, args, emit)

	if it.Error != nil && it.Error.Message != "" {
		emitMessage(sessionID, FormatToolResult(name, it.Error.Message, true), "error", sink)
		emitToolResultEvent(sessionID, name, it.Error.Message, true, emit)
		return
	}
	if body := mcpResultText(it.Result); body != "" {
		emitMessage(sessionID, FormatToolResult(name, body, false), "tool", sink)
		emitToolResultEvent(sessionID, name, body, false, emit)
	}
}

// mcpResultText pulls human-readable text from an MCP tool result: joined text
// content blocks, else the structured-content JSON.
func mcpResultText(r *mcpToolCallResult) string {
	if r == nil {
		return ""
	}
	var parts []string
	for _, c := range r.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	if len(r.StructuredContent) > 0 && string(r.StructuredContent) != "null" {
		return string(r.StructuredContent)
	}
	return ""
}

// formatAppServerCommand renders a commandExecution item as a tool row:
// "[Bash] <command> (exit N)\n<output truncated to 4KB>".
func formatAppServerCommand(it appServerItem) string {
	out := it.Command
	if it.ExitCode != nil {
		out = fmt.Sprintf("%s (exit %d)", out, *it.ExitCode)
	}
	body := it.AggregatedOutput
	const maxOut = 4096
	if len(body) > maxOut {
		body = body[:maxOut] + "\n…(truncated)"
	}
	if body != "" {
		out = out + "\n" + body
	}
	return "[Bash] " + out
}

// turnCompletedError returns params.turn.error.message when a turn ended in
// error, else "".
func turnCompletedError(params json.RawMessage) string {
	var p struct {
		Turn struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal(params, &p) != nil || p.Turn.Error == nil {
		return ""
	}
	return p.Turn.Error.Message
}

// rateLimitReached reports whether an account/rateLimits/updated event signals
// an actually-reached limit (rateLimitReachedType != null). The event also
// fires for routine usage updates, which must NOT trigger a restart.
func rateLimitReached(params json.RawMessage) bool {
	var p struct {
		RateLimits struct {
			RateLimitReachedType *json.RawMessage `json:"rateLimitReachedType"`
		} `json:"rateLimits"`
	}
	if json.Unmarshal(params, &p) != nil {
		return false
	}
	return p.RateLimits.RateLimitReachedType != nil && string(*p.RateLimits.RateLimitReachedType) != "null"
}

// classifyAppServerError sets sig.rateLimited (reusing CodexAdapter's limit
// patterns) or sig.fatalErr from a free-text error message.
func classifyAppServerError(msg string, sig *appServerSignal) {
	if msg == "" {
		return
	}
	if class, matched := (&CodexAdapter{}).ClassifyExit(msg, "", 1, nil, nil); class == RetryClassRateLimit {
		sig.rateLimited = true
		sig.matchedReason = matched
		return
	}
	sig.fatalErr = msg
}
