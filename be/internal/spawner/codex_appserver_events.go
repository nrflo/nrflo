package spawner

import (
	"encoding/json"
	"fmt"
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
}

// dispatchAppServerEvent maps ONE app-server notification to Sink calls and
// returns control hints. maxCtx is the fallback model context window when the
// event omits modelContextWindow. Every event bumps the stall heartbeat.
func dispatchAppServerEvent(sessionID string, env rpcEnvelope, sink Sink, maxCtx int) appServerSignal {
	var sig appServerSignal
	switch env.Method {
	case "item/completed":
		dispatchCompletedItem(sessionID, env.Params, sink)
	case "item/agentMessage/delta":
		// Streamed text; the canonical full text arrives via item/completed
		// (agentMessage). Deltas only keep the heartbeat alive.
		sink.BumpLastMessage(sessionID)
	case "thread/tokenUsage/updated":
		dispatchTokenUsage(sessionID, env.Params, sink, maxCtx)
	case "turn/started":
		sink.BumpLastMessage(sessionID)
		sig.turnStarted = true
	case "turn/completed":
		sink.OnTurnComplete(sessionID)
		sig.turnCompleted = true
		if msg := turnCompletedError(env.Params); msg != "" {
			classifyAppServerError(msg, &sig)
		}
	case "error":
		var p struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(env.Params, &p)
		classifyAppServerError(p.Error.Message, &sig)
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
type appServerItem struct {
	Type             string `json:"type"`
	Text             string `json:"text"`             // agentMessage
	Command          string `json:"command"`          // commandExecution
	AggregatedOutput string `json:"aggregatedOutput"` // commandExecution
	ExitCode         *int   `json:"exitCode"`         // commandExecution
}

func dispatchCompletedItem(sessionID string, params json.RawMessage, sink Sink) {
	var p struct {
		Item appServerItem `json:"item"`
	}
	if json.Unmarshal(params, &p) != nil {
		sink.BumpLastMessage(sessionID)
		return
	}
	switch p.Item.Type {
	case "agentMessage":
		emitAgentText(sessionID, p.Item.Text, sink)
	case "commandExecution":
		emitMessage(sessionID, formatAppServerCommand(p.Item), "tool", sink)
	default:
		// reasoning, userMessage, fileChange summaries, etc. — heartbeat only.
		sink.BumpLastMessage(sessionID)
	}
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

func dispatchTokenUsage(sessionID string, params json.RawMessage, sink Sink, maxCtx int) {
	var p struct {
		TokenUsage struct {
			Total struct {
				InputTokens int `json:"inputTokens"`
			} `json:"total"`
			ModelContextWindow int `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(params, &p) != nil {
		sink.BumpLastMessage(sessionID)
		return
	}
	ctxWindow := p.TokenUsage.ModelContextWindow
	if ctxWindow <= 0 {
		ctxWindow = maxCtx
	}
	used := p.TokenUsage.Total.InputTokens
	if ctxWindow > 0 && used > 0 {
		sink.UpdateContextLeft(sessionID, ComputeContextLeftPct(used, ctxWindow))
	}
	sink.BumpLastMessage(sessionID)
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
