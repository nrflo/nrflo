package spawner

// Shared helpers for file-tailing context trackers. They translate raw tailer
// output into Sink calls.

// emitAgentText records an agent_message body as a "text" agent_messages row
// and broadcasts the update. Empty bodies are dropped.
func emitAgentText(sessionID, body string, sink Sink) {
	if body == "" {
		return
	}
	emitMessage(sessionID, body, "text", sink)
}

// emitMessage is the common path: RecordHookMessage + BroadcastMessagesUpdated
// + BumpLastMessage + SetLastMessage.
func emitMessage(sessionID, body, category string, sink Sink) {
	emitMessageWithPayload(sessionID, body, category, "", sink)
}

// emitMessageWithPayload is emitMessage plus an explicit payload (e.g. the
// tool_use_id carried by a tool-invoke row) — used by the api console engine,
// which has no processInfo pending-message buffer to stamp in-memory the way
// TrackToolInvoke does for autonomous agents (output_tool_span.go).
func emitMessageWithPayload(sessionID, body, category, payload string, sink Sink) {
	projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, body, category, payload)
	if err != nil {
		return
	}
	sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
	sink.BumpLastMessage(sessionID)
	// Surface a short preview in the periodic "agent status" log line.
	preview := body
	if len(preview) > 120 {
		preview = preview[:120]
	}
	sink.SetLastMessage(sessionID, preview)
}
