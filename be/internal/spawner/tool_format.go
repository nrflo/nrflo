package spawner

import (
	"bytes"
	"encoding/json"
	"strings"

	"be/internal/spawner/apirun"
)

// maxInlineDetail caps inline tool input/output rendered into a single log row,
// matching the api-mode runner's 2 KB cap (apirun.formatToolResult and
// sink.OnToolUseStop) so CLI-hook, codex app-server, and api agents render
// tool activity at the same size.
const maxInlineDetail = 2048

// maxPayloadInput caps the raw tool input embedded in an invoke row's
// structured "input" payload field. Larger than maxInlineDetail because the
// structured input is the point of this field, not an afterthought.
const maxPayloadInput = 8192

// hiddenResultTools name the tools whose successful result row is dropped before
// storage: the invoke row already names the file/command and the output (file
// bodies, command stdout) is noise in the agent log. Errors are unaffected —
// they take a different path and stay visible.
var hiddenResultTools = map[string]bool{"Read": true, "Bash": true, "Edit": true}

// IsHiddenResultTool reports whether a tool's success result row should be
// suppressed. The name is title-cased so a lowercase CLI/MCP variant matches the
// same way FormatToolResult would render it.
func IsHiddenResultTool(toolName string) bool {
	return hiddenResultTools[titleToolName(toolName)]
}

// ToolCategory returns the message category for a tool invocation, delegating
// to the canonical apirun.ToolCategory so the CLI-hook/codex paths never
// drift from api mode's categorization (Rule 6).
func ToolCategory(toolName string) string {
	return apirun.ToolCategory(toolName)
}

// titleToolName upper-cases the first letter so a lowercase CLI/MCP tool name
// (e.g. "mcp__nrflo__emit_findings") renders identically in its invoke row and
// its result row.
func titleToolName(toolName string) string {
	if len(toolName) > 0 {
		return strings.ToUpper(toolName[:1]) + toolName[1:]
	}
	return toolName
}

// FormatToolDetail extracts relevant details from tool input based on tool type.
// It is a package-level function so socket handlers can reuse the same formatting
// for hook-sourced tool events without duplicating logic. Tools without a curated
// field (MCP tools such as mcp__nrflo__*, and anything not in the switch) fall
// back to a compact, capped dump of the raw input rather than a bare "[Name]".
func FormatToolDetail(toolName string, input map[string]interface{}) string {
	toolName = titleToolName(toolName)

	if input == nil {
		return "[" + toolName + "]"
	}

	var detail string

	switch toolName {
	case "Skill":
		skillName, _ := input["skill"].(string)
		if skillName == "" {
			skillName, _ = input["name"].(string)
		}
		skillArgs, _ := input["args"].(string)
		if skillName != "" {
			detail = "skill:" + skillName
			if skillArgs != "" {
				detail += " " + skillArgs
			}
		}

	case "Bash":
		cmd, _ := input["command"].(string)
		if cmd != "" {
			detail = cmd
		}

	case "Read":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Write":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Edit":
		path, _ := input["file_path"].(string)
		if path == "" {
			path, _ = input["filePath"].(string)
		}
		if path != "" {
			detail = path
		}

	case "Glob":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail = path + "/" + pattern
			}
		}

	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		if pattern != "" {
			detail = pattern
			if path != "" {
				detail += " in " + path
			}
		}

	case "Task", "Agent":
		desc, _ := input["description"].(string)
		agentType, _ := input["subagent_type"].(string)
		if desc != "" {
			detail = desc
			if agentType != "" {
				detail = agentType + ": " + desc
			}
		}

	case "WebFetch":
		url, _ := input["url"].(string)
		if url != "" {
			detail = url
		}

	case "WebSearch":
		query, _ := input["query"].(string)
		if query != "" {
			detail = query
		}

	case "TodoWrite", "TaskCreate", "TaskUpdate", "TaskList":
		// Just show tool name for task management tools
		return "[" + toolName + "]"
	}

	if detail == "" {
		if dump := compactInput(input); dump != "" {
			return "[" + toolName + "] " + dump
		}
		return "[" + toolName + "]"
	}

	return "[" + toolName + "] " + detail
}

// compactInput renders a tool's input map as compact JSON capped to
// maxInlineDetail bytes. Returns "" for empty input or a marshal failure.
func compactInput(input map[string]interface{}) string {
	if len(input) == 0 {
		return ""
	}
	b, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	if len(b) > maxInlineDetail {
		return string(b[:maxInlineDetail])
	}
	return string(b)
}

// BuildToolInvokePayload builds the JSON payload for a tool-invoke
// agent_messages row: {"tool_use_id":...,"input":...,"input_truncated":...}
// (an "ended_at" field is stamped in later, once the tool returns, by
// stampPendingToolEnd/SetToolEnded). rawInput is compacted; empty/"null"/"{}"
// inputs are omitted entirely. Inputs over maxPayloadInput set
// input_truncated:true instead of embedding a sliced (and therefore invalid)
// JSON object. Returns "" when the resulting payload would be empty.
func BuildToolInvokePayload(toolUseID string, rawInput []byte) string {
	p := map[string]any{}
	if toolUseID != "" {
		p["tool_use_id"] = toolUseID
	}
	if compact := compactRawInput(rawInput); compact != "" {
		if len(compact) <= maxPayloadInput {
			p["input"] = json.RawMessage(compact)
		} else {
			p["input_truncated"] = true
		}
	}
	if len(p) == 0 {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// compactRawInput compacts rawInput to single-line JSON, returning "" for
// empty/null/empty-object input or a marshal failure.
func compactRawInput(rawInput []byte) string {
	if len(rawInput) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, rawInput); err != nil {
		return ""
	}
	s := buf.String()
	if s == "" || s == "null" || s == "{}" {
		return ""
	}
	return s
}

// FormatToolResult renders a tool's output as a log row, mirroring the api-mode
// runner's formatToolResult (be/internal/spawner/apirun/runner.go): success as
// "[name] → out", error as "name: out" (no bracket, so the UI renders the Error
// badge without a duplicate tool badge). Output is capped to maxInlineDetail.
func FormatToolResult(toolName, out string, isErr bool) string {
	toolName = titleToolName(toolName)
	if len(out) > maxInlineDetail {
		out = out[:maxInlineDetail]
	}
	if isErr {
		return toolName + ": " + out
	}
	return "[" + toolName + "] → " + out
}
