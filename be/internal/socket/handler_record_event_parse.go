package socket

import (
	"encoding/json"
	"strings"
)

// extractToolResultBody pulls a human-readable string out of the PostToolUse
// payload. Claude ships `tool_response` (object on current builds, string on
// older builds) and sometimes `tool_result`. For object form, prefer an MCP
// content array, then stdout and other common content fields, then compact JSON.
func extractToolResultBody(event map[string]interface{}) string {
	for _, key := range []string{"tool_response", "tool_result"} {
		switch v := event[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]interface{}:
			// MCP tool results arrive as {content:[{type:"text",text:"..."}], isError}.
			if s := contentArrayText(v["content"]); s != "" {
				return s
			}
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

// contentArrayText extracts joined text from an MCP-style content array
// ([{type:"text", text:"..."}]). Returns "" when v is not such an array.
func contentArrayText(v interface{}) string {
	arr, ok := v.([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["text"].(string); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}
