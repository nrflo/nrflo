package spawner

import (
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
