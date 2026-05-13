package spawner

// opencodeEventHandler is the transport-agnostic dispatch target for opencode
// agent events. Both the cli batch stdout-NDJSON parser (output.go) and the
// cli_interactive HTTP message poller (cli_adapter_opencode_poll.go) feed
// events into the same dispatcher via this interface, so the event-shape →
// (text|tool_use|tool_result) decoding lives in exactly one place.
type opencodeEventHandler interface {
	OnText(text string)
	OnToolUse(toolName string, input map[string]interface{})
	OnToolResult(data map[string]interface{})
}

// dispatchOpencodeEvent decodes one opencode-shaped event ({type, part:{...}} or
// {type:"tool_result", tool_use_id, ...}) and invokes the appropriate handler
// method. Unknown / empty payloads are ignored.
//
// Event shapes accepted (matches the NDJSON `opencode run --format json` emits;
// the poll module synthesizes equivalents from the HTTP message API):
//
//	{"type":"text",      "part":{"text":"..."}}
//	{"type":"tool_use",  "part":{"tool":"bash", "state":{"input":{...}}}}
//	{"type":"tool_result","tool_use_id":"…", "content":[...]}
func dispatchOpencodeEvent(eventType string, data map[string]interface{}, h opencodeEventHandler) {
	switch eventType {
	case "text":
		part, _ := data["part"].(map[string]interface{})
		if part == nil {
			return
		}
		text, _ := part["text"].(string)
		if text == "" {
			return
		}
		h.OnText(text)

	case "tool_use":
		part, _ := data["part"].(map[string]interface{})
		if part == nil {
			return
		}
		toolName, _ := part["tool"].(string)
		if toolName == "" {
			return
		}
		var input map[string]interface{}
		if state, _ := part["state"].(map[string]interface{}); state != nil {
			input, _ = state["input"].(map[string]interface{})
		}
		if input == nil {
			input, _ = part["input"].(map[string]interface{})
		}
		h.OnToolUse(toolName, input)

	case "tool_result":
		h.OnToolResult(data)
	}
}

// spawnerProcHandler adapts opencodeEventHandler to the cli batch path's
// existing spawner.handle* methods (which mutate proc.pendingMessages and
// proc.lastMessage*). Used from output.go's stdout-NDJSON parser.
type spawnerProcHandler struct {
	s    *Spawner
	proc *processInfo
}

func (h *spawnerProcHandler) OnText(text string) {
	h.s.handleTextMessage(h.proc, text)
}

func (h *spawnerProcHandler) OnToolUse(toolName string, input map[string]interface{}) {
	h.s.handleToolUse(h.proc, toolName, input)
}

func (h *spawnerProcHandler) OnToolResult(data map[string]interface{}) {
	h.s.handleClaudeToolResult(h.proc, data)
}
