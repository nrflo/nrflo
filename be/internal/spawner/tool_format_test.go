package spawner

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFormatToolDetail_MCPFallback verifies an unknown/MCP tool (no curated
// field) shows a compact JSON dump of its input instead of a bare "[Name]".
func TestFormatToolDetail_MCPFallback(t *testing.T) {
	t.Parallel()
	got := FormatToolDetail("mcp__nrflo__emit_findings", map[string]interface{}{"key": "summary"})
	want := `[Mcp__nrflo__emit_findings] {"key":"summary"}`
	if got != want {
		t.Errorf("FormatToolDetail MCP fallback = %q, want %q", got, want)
	}
}

// TestFormatToolDetail_EmptyInputBareName verifies nil/empty input still yields
// just the bracketed name (no dangling JSON).
func TestFormatToolDetail_EmptyInputBareName(t *testing.T) {
	t.Parallel()
	if got := FormatToolDetail("mcp__nrflo__ping", nil); got != "[Mcp__nrflo__ping]" {
		t.Errorf("nil input = %q, want %q", got, "[Mcp__nrflo__ping]")
	}
	if got := FormatToolDetail("mcp__nrflo__ping", map[string]interface{}{}); got != "[Mcp__nrflo__ping]" {
		t.Errorf("empty input = %q, want %q", got, "[Mcp__nrflo__ping]")
	}
}

// TestFormatToolDetail_KnownToolUnchanged guards that curated tools still render
// their single field (no JSON-dump regression for known tools).
func TestFormatToolDetail_KnownToolUnchanged(t *testing.T) {
	t.Parallel()
	if got := FormatToolDetail("Bash", map[string]interface{}{"command": "ls", "extra": "x"}); got != "[Bash] ls" {
		t.Errorf("Bash = %q, want %q", got, "[Bash] ls")
	}
}

// TestFormatToolDetail_FallbackCapped verifies the JSON fallback is capped at
// maxInlineDetail bytes.
func TestFormatToolDetail_FallbackCapped(t *testing.T) {
	t.Parallel()
	got := FormatToolDetail("mcp__x__y", map[string]interface{}{"v": strings.Repeat("z", 5000)})
	const prefix = "[Mcp__x__y] "
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("missing prefix: %q", got[:30])
	}
	if dumpLen := len(got) - len(prefix); dumpLen != maxInlineDetail {
		t.Errorf("dump length = %d, want %d (capped)", dumpLen, maxInlineDetail)
	}
}

// TestFormatToolResult verifies success/error renderings match the api-mode shape.
func TestFormatToolResult(t *testing.T) {
	t.Parallel()
	if got := FormatToolResult("mcp__nrflo__emit_findings", "ok", false); got != "[Mcp__nrflo__emit_findings] → ok" {
		t.Errorf("success = %q", got)
	}
	if got := FormatToolResult("Read", "boom", true); got != "Read: boom" {
		t.Errorf("error = %q, want %q", got, "Read: boom")
	}
}

// TestIsHiddenResultTool verifies Read/Bash/Edit are flagged (case-normalized via
// titleToolName) while other tools render their result rows.
func TestIsHiddenResultTool(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Read", "Bash", "Edit", "read", "bash", "edit"} {
		if !IsHiddenResultTool(name) {
			t.Errorf("IsHiddenResultTool(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"WebFetch", "Write", "Grep", "mcp__nrflo__emit_findings", ""} {
		if IsHiddenResultTool(name) {
			t.Errorf("IsHiddenResultTool(%q) = true, want false", name)
		}
	}
}

// TestBuildToolInvokePayload_OmitsEmptyInput verifies empty/"null"/"{}" raw
// inputs never surface an "input" field, and a toolUseID with no usable input
// still produces a payload carrying only tool_use_id.
func TestBuildToolInvokePayload_OmitsEmptyInput(t *testing.T) {
	t.Parallel()
	for _, raw := range [][]byte{nil, []byte(""), []byte("null"), []byte("{}")} {
		got := BuildToolInvokePayload("tu_1", raw)
		var p map[string]any
		if err := json.Unmarshal([]byte(got), &p); err != nil {
			t.Fatalf("payload not JSON for input=%q: %v", raw, err)
		}
		if _, ok := p["input"]; ok {
			t.Errorf("input=%q: payload = %q, want no input field", raw, got)
		}
		if _, ok := p["input_truncated"]; ok {
			t.Errorf("input=%q: payload = %q, want no input_truncated field", raw, got)
		}
		if p["tool_use_id"] != "tu_1" {
			t.Errorf("input=%q: tool_use_id = %v, want tu_1", raw, p["tool_use_id"])
		}
	}
}

// TestBuildToolInvokePayload_SmallInputEmbedded verifies a small raw input is
// embedded verbatim (compacted) as a nested JSON object under "input".
func TestBuildToolInvokePayload_SmallInputEmbedded(t *testing.T) {
	t.Parallel()
	got := BuildToolInvokePayload("tu_2", []byte(`{"command": "ls -la"}`))
	var p map[string]any
	if err := json.Unmarshal([]byte(got), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	input, ok := p["input"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %q, want an object at $.input", got)
	}
	if input["command"] != "ls -la" {
		t.Errorf("input.command = %v, want %q", input["command"], "ls -la")
	}
	if _, ok := p["input_truncated"]; ok {
		t.Errorf("payload = %q, want no input_truncated for small input", got)
	}
}

// TestBuildToolInvokePayload_OverCapTruncates verifies an over-cap raw input
// yields input_truncated:true, still-valid JSON, and never a sliced/partial
// object under "input".
func TestBuildToolInvokePayload_OverCapTruncates(t *testing.T) {
	t.Parallel()
	huge := `{"blob":"` + strings.Repeat("z", maxPayloadInput+1000) + `"}`
	got := BuildToolInvokePayload("tu_3", []byte(huge))

	var p map[string]any
	if err := json.Unmarshal([]byte(got), &p); err != nil {
		t.Fatalf("payload not valid JSON: %v; got %q", err, got)
	}
	if p["input_truncated"] != true {
		t.Errorf("input_truncated = %v, want true", p["input_truncated"])
	}
	if _, ok := p["input"]; ok {
		t.Errorf("payload = %q, want no partial/sliced input object", got)
	}
	if p["tool_use_id"] != "tu_3" {
		t.Errorf("tool_use_id = %v, want tu_3", p["tool_use_id"])
	}
}

// TestBuildToolInvokePayload_NoToolUseIDOmitsField verifies a codex-style
// invoke (no tool_use_id) yields a payload with only "input", no
// "tool_use_id" key at all.
func TestBuildToolInvokePayload_NoToolUseIDOmitsField(t *testing.T) {
	t.Parallel()
	got := BuildToolInvokePayload("", []byte(`{"query":"foo"}`))
	var p map[string]any
	if err := json.Unmarshal([]byte(got), &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if _, ok := p["tool_use_id"]; ok {
		t.Errorf("payload = %q, want no tool_use_id key", got)
	}
	input, ok := p["input"].(map[string]any)
	if !ok || input["query"] != "foo" {
		t.Errorf("payload = %q, want input.query=foo", got)
	}
}

// TestBuildToolInvokePayload_AllEmptyReturnsEmptyString verifies no
// toolUseID + no usable input produces an empty payload string (falls back to
// TrackMessage at the call site).
func TestBuildToolInvokePayload_AllEmptyReturnsEmptyString(t *testing.T) {
	t.Parallel()
	if got := BuildToolInvokePayload("", nil); got != "" {
		t.Errorf("BuildToolInvokePayload(\"\", nil) = %q, want \"\"", got)
	}
	if got := BuildToolInvokePayload("", []byte("{}")); got != "" {
		t.Errorf("BuildToolInvokePayload(\"\", {}) = %q, want \"\"", got)
	}
}

// TestFormatToolResult_Capped verifies output is capped at maxInlineDetail.
func TestFormatToolResult_Capped(t *testing.T) {
	t.Parallel()
	got := FormatToolResult("Bash", strings.Repeat("a", 5000), false)
	const prefix = "[Bash] → "
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("missing prefix: %q", got[:20])
	}
	if outLen := len(got) - len(prefix); outLen != maxInlineDetail {
		t.Errorf("out length = %d, want %d", outLen, maxInlineDetail)
	}
}
