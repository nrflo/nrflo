package socket

// normalizeGeminiHookEvent rewrites Gemini hook event names to their Claude
// equivalents so the existing switch in handleAgentRecordEvent handles them
// without modification. Mutates the event map in place; no-op for missing key,
// non-string value, or already-normalized names.
func normalizeGeminiHookEvent(event map[string]interface{}) {
	name, _ := event["hook_event_name"].(string)
	switch name {
	case "BeforeTool":
		event["hook_event_name"] = "PreToolUse"
	case "AfterTool":
		event["hook_event_name"] = "PostToolUse"
	case "AfterAgent":
		event["hook_event_name"] = "Stop"
	}
}
