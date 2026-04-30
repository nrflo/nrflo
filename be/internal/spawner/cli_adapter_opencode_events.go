package spawner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// startOpencodeEventStream connects to opencode's embedded HTTP /event SSE
// endpoint and dispatches events to the provided Sink. Reconnects with
// exponential backoff (cap 5s) while ctx is alive. Returns a cancel func
// that stops the goroutine.
func startOpencodeEventStream(ctx context.Context, port int, sessionID, workDir string, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go opencodeSSELoop(cctx, port, sessionID, workDir, sink)
	return cancel
}

func opencodeSSELoop(ctx context.Context, port int, sessionID, workDir string, sink Sink) {
	url := fmt.Sprintf("http://127.0.0.1:%d/event?directory=%s", port, workDir)
	backoff := 250 * time.Millisecond
	state := newSSEState()

	for {
		if ctx.Err() != nil {
			return
		}
		err := consumeSSEStream(ctx, url, sessionID, sink, state)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// backoff before reconnect
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}
}

// sseState holds per-stream mutable state across SSE frames.
type sseState struct {
	mu sync.Mutex
	// textBuf accumulates streaming text deltas per part-id.
	textBuf map[string]string
	// partMsg maps part-id → message-id.
	partMsg map[string]string
	// msgParts maps message-id → ordered list of part-ids.
	msgParts map[string][]string
}

func newSSEState() *sseState {
	return &sseState{
		textBuf:  make(map[string]string),
		partMsg:  make(map[string]string),
		msgParts: make(map[string][]string),
	}
}

// consumeSSEStream opens the SSE stream and reads until EOF or ctx cancel.
func consumeSSEStream(ctx context.Context, url, sessionID string, sink Sink, state *sseState) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{} // no timeout — SSE is long-lived
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[len("data: "):]
		if payload == "" {
			continue
		}
		dispatchSSEEvent(ctx, payload, sessionID, sink, state)
	}
	return scanner.Err()
}

// dispatchSSEEvent parses one SSE data payload and routes it to the Sink.
func dispatchSSEEvent(ctx context.Context, payload, sessionID string, sink Sink, state *sseState) {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}
	eventType, _ := event["type"].(string)
	props, _ := event["properties"].(map[string]interface{})
	if props == nil {
		return
	}

	switch eventType {
	case "message.part.updated":
		handlePartUpdated(ctx, props, sessionID, sink, state)

	case "message.part.delta":
		handlePartDelta(props, state)

	case "message.updated":
		handleMessageUpdated(ctx, props, sessionID, sink, state)

	case "session.idle":
		sink.BumpLastMessage(sessionID)
		sink.OnTurnComplete(sessionID)

	case "session.error":
		handleSessionError(ctx, props, sessionID, sink)
	}
}

// handlePartUpdated processes message.part.updated events.
func handlePartUpdated(ctx context.Context, props map[string]interface{}, sessionID string, sink Sink, state *sseState) {
	part, _ := props["part"].(map[string]interface{})
	if part == nil {
		return
	}
	partType, _ := part["type"].(string)
	partID, _ := part["id"].(string)
	msgID, _ := part["messageID"].(string)

	state.mu.Lock()
	if partID != "" && msgID != "" {
		if _, exists := state.partMsg[partID]; !exists {
			state.partMsg[partID] = msgID
			state.msgParts[msgID] = append(state.msgParts[msgID], partID)
		}
	}
	state.mu.Unlock()

	switch partType {
	case "tool":
		handleToolPart(ctx, part, sessionID, sink)

	case "text":
		// Update text snapshot in buffer (overrides accumulated deltas with
		// the authoritative snapshot when available).
		if text, _ := part["text"].(string); text != "" && partID != "" {
			state.mu.Lock()
			state.textBuf[partID] = text
			state.mu.Unlock()
		}
	}
}

// handleToolPart dispatches tool-state transitions.
func handleToolPart(ctx context.Context, part map[string]interface{}, sessionID string, sink Sink) {
	partState, _ := part["state"].(map[string]interface{})
	if partState == nil {
		return
	}
	status, _ := partState["status"].(string)
	toolName, _ := part["tool"].(string)

	switch status {
	case "running":
		input, _ := partState["input"].(map[string]interface{})
		content := FormatToolDetail(toolName, input)
		category := ToolCategory(toolName)
		projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, content, category)
		if err == nil && projectID != "" {
			sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
		}
		sink.BumpLastMessage(sessionID)

	case "completed":
		output, _ := partState["output"].(string)
		content := "[" + capitalize(toolName) + " result]"
		if output != "" {
			content = "[" + capitalize(toolName) + " result] " + truncateStr(output, 200)
		}
		category := ToolCategory(toolName)
		projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, content, category)
		if err == nil && projectID != "" {
			sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
		}
		sink.BumpLastMessage(sessionID)
	}
}

// handlePartDelta accumulates streaming text deltas per part-id.
func handlePartDelta(props map[string]interface{}, state *sseState) {
	field, _ := props["field"].(string)
	if field != "text" {
		return
	}
	partID, _ := props["partID"].(string)
	delta, _ := props["delta"].(string)
	if partID == "" || delta == "" {
		return
	}
	state.mu.Lock()
	state.textBuf[partID] += delta
	state.mu.Unlock()
}

// handleMessageUpdated flushes accumulated text and updates context on message complete.
func handleMessageUpdated(ctx context.Context, props map[string]interface{}, sessionID string, sink Sink, state *sseState) {
	info, _ := props["info"].(map[string]interface{})

	// Flush accumulated text for parts in this message.
	msgID, _ := info["id"].(string)
	if msgID == "" {
		// Try direct props field
		msgID, _ = props["messageID"].(string)
	}
	if msgID != "" {
		state.mu.Lock()
		parts := state.msgParts[msgID]
		var combined strings.Builder
		for _, pid := range parts {
			if txt := state.textBuf[pid]; txt != "" {
				combined.WriteString(txt)
				delete(state.textBuf, pid)
			}
		}
		delete(state.msgParts, msgID)
		// Clean up partMsg entries
		for pid, mid := range state.partMsg {
			if mid == msgID {
				delete(state.partMsg, pid)
			}
		}
		state.mu.Unlock()

		if text := strings.TrimSpace(combined.String()); text != "" {
			projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, text, "text")
			if err == nil && projectID != "" {
				sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
			}
			sink.BumpLastMessage(sessionID)
		}
	}

	// Update context from token usage.
	if info != nil {
		tokens, _ := info["tokens"].(map[string]interface{})
		if tokens != nil {
			total := int(asFloat(tokens["total"]))
			if total > 0 {
				pct := ComputeContextLeftPct(total, 0)
				projectID, ticketID, workflow, _ := sink.UpdateContextLeft(sessionID, pct)
				if projectID != "" {
					_ = projectID
					_ = ticketID
					_ = workflow
				}
			}
		}
	}
}

// handleSessionError records a session.error event as an error message.
func handleSessionError(ctx context.Context, props map[string]interface{}, sessionID string, sink Sink) {
	errObj, _ := props["error"].(map[string]interface{})
	msg := "opencode session error"
	if errObj != nil {
		if name, _ := errObj["name"].(string); name != "" {
			msg = name
		}
		if data, _ := errObj["data"].(map[string]interface{}); data != nil {
			if errMsg, _ := data["message"].(string); errMsg != "" {
				msg = msg + ": " + truncateStr(errMsg, 300)
			}
		}
	}
	projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, msg, "text")
	if err == nil && projectID != "" {
		sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
		sink.RecordError(projectID, "agent", sessionID, msg)
	}
}

// capitalize returns s with the first character uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// truncateStr returns s truncated to at most max runes.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// asFloat safely converts an interface{} to float64 (returns 0 if not numeric).
func asFloat(v interface{}) float64 {
	f, _ := v.(float64)
	return f
}
