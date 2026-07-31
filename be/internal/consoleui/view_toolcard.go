package consoleui

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// toolRowPrefix is the single prefix shared by every tool-family transcript
// row (tool, tool_use, tool_result, subagent) so they read as one kind of
// row instead of two accidental formats (ticket item 3).
const toolRowPrefix = "tool · "

// toolCardBodyLines caps a tool card's wrapped body so a large payload
// (e.g. a delegate brief) can't blow out scrollback; the cut is always
// marked via forceEllipsis.
const toolCardBodyLines = 6

// toolHeadValueMax clips a generic scalar param value in the head line.
const toolHeadValueMax = 40

// toolCard renders a compact capped card for a tool-family row: a head line
// (unified prefix + "[Name] key=params") plus a body wrapped to width and
// capped at toolCardBodyLines, the cut always marked with forceEllipsis.
// Pure text in, text out — no *model, no terminal — so the caller styles the
// whole result in one Render (lipgloss MaxWidth must never run over an
// already-styled string).
func toolCard(content string, width int) string {
	width = max(1, width)
	name, rest := splitToolRow(content)

	var head string
	if name != "" {
		head = toolRowPrefix + "[" + name + "]"
		if params := toolHeadParams(name, rest); params != "" {
			head += " " + params
		}
		head = truncate(head, width)
	} else {
		head = truncate(toolRowPrefix+firstLine(rest), width)
	}

	lines := strings.Split(fitWidth(prettyToolContent(rest), width), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > toolCardBodyLines {
		lines = lines[:toolCardBodyLines]
		lines[toolCardBodyLines-1] = forceEllipsis(lines[toolCardBodyLines-1], width)
	}
	if len(lines) == 0 {
		return head
	}
	return head + "\n" + strings.Join(lines, "\n")
}

// splitToolRow splits a "[Name] rest" row into its bracketed name and
// trimmed remainder. Rows without a leading "[" (the "Name: err" error-row
// shape from spawner/tool_format.go) have no name.
func splitToolRow(content string) (name, rest string) {
	if !strings.HasPrefix(content, "[") {
		return "", content
	}
	idx := strings.Index(content, "]")
	if idx < 0 {
		return "", content
	}
	return content[1:idx], content[idx+1:]
}

// normalizeToolName strips the MCP bridge prefixes and lower-cases the
// name, mirroring apirun.ToolCategory's matching (sink.go) so
// "[Mcp__nrflo__delegate]" (CLI/hook path, title-cased by tool_format.go)
// and "[delegate]" (api path) hit the same head-line case.
func normalizeToolName(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "mcp__nrflo__")
	name = strings.TrimPrefix(name, "nrflo/")
	return name
}

// toolHeadParams renders the key-params portion of a tool card's head line
// from a row's JSON payload, switching on the normalized tool name. Never
// errors or panics: invalid/absent JSON (result rows, non-JSON curated
// invokes like "[Bash] <cmd>") degrades to "" and the head line falls back
// to the bare tool name.
func toolHeadParams(name, rest string) string {
	if rest == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		return ""
	}
	if normalizeToolName(name) == "delegate" {
		return delegateHeadParams(payload)
	}
	return genericHeadParams(payload)
}

// delegateHeadParams renders "tier=<tier> fanout=<len(fanout)> · <first
// sentence of brief>" from delegate's tools_builtin/delegate.go arg shape.
func delegateHeadParams(payload map[string]any) string {
	tier, _ := payload["tier"].(string)
	fanout, _ := payload["fanout"].([]any)
	brief, _ := payload["brief"].(string)

	var b strings.Builder
	if tier != "" {
		b.WriteString("tier=" + tier + " ")
	}
	b.WriteString("fanout=" + strconv.Itoa(len(fanout)))
	if sentence := firstSentence(brief); sentence != "" {
		b.WriteString(" · " + sentence)
	}
	return b.String()
}

// firstSentence returns the text up to and including the first ".", "!", or
// "?", falling back to the trimmed whole string when no terminator is found.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, ".!?"); idx >= 0 {
		return strings.TrimSpace(s[:idx+1])
	}
	return s
}

// genericHeadParams renders sorted scalar "k=v" pairs from payload: strings
// are clipped, arrays render as "k=[N]", and nested objects/other types are
// skipped — keeps output deterministic and bounded for any tool.
func genericHeadParams(payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		switch v := payload[k].(type) {
		case string:
			parts = append(parts, k+"="+clipHeadValue(v))
		case bool:
			parts = append(parts, k+"="+strconv.FormatBool(v))
		case float64:
			parts = append(parts, k+"="+strconv.FormatFloat(v, 'g', -1, 64))
		case []any:
			parts = append(parts, k+"=["+strconv.Itoa(len(v))+"]")
		}
	}
	return strings.Join(parts, " ")
}

func clipHeadValue(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	if len(v) <= toolHeadValueMax {
		return v
	}
	return v[:toolHeadValueMax] + "…"
}
