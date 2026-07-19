package apirun

import (
	"context"
	"strings"
	"testing"
)

// Guard paths: without an artifact scope the result must pass through
// unchanged regardless of size — the quarantine never destroys data it
// cannot offload.
func TestMaybeOffload_NoArtifactScopePassesThrough(t *testing.T) {
	big := strings.Repeat("x", defaultOffloadThreshold*3)

	// nil ArtifactSvc
	out := MaybeOffloadToolResult(context.Background(), ToolEnv{WorkflowInstanceID: "wfi"}, "some_tool", big)
	if out != big {
		t.Error("nil ArtifactSvc: expected pass-through")
	}
	// empty workflow instance (console chats without artifact scope)
	out = MaybeOffloadToolResult(context.Background(), ToolEnv{}, "some_tool", big)
	if out != big {
		t.Error("empty wfi: expected pass-through")
	}
}

func TestMaybeOffload_UnderThresholdUnchanged(t *testing.T) {
	small := strings.Repeat("y", 100)
	out := MaybeOffloadToolResult(context.Background(), ToolEnv{}, "some_tool", small)
	if out != small {
		t.Error("small result must be unchanged")
	}
}

func TestOffloadExemptTools(t *testing.T) {
	for _, name := range []string{"artifact_get", "web_fetch", "web_search", "read_document", "agent_finished", "agent_fail"} {
		if !offloadExemptTool(name) {
			t.Errorf("%s should be exempt", name)
		}
	}
	for _, name := range []string{"bash", "read_file", "consult", "run_subworkflow"} {
		if offloadExemptTool(name) {
			t.Errorf("%s should NOT be exempt", name)
		}
	}
}

func TestOffloadExcerpt_HeadTailAndMarker(t *testing.T) {
	head := strings.Repeat("H", offloadHeadBytes)
	middle := strings.Repeat("M", 50_000)
	tail := strings.Repeat("T", offloadTailBytes)
	out := offloadExcerpt(head+middle+tail, "[MARKER]")

	if !strings.HasPrefix(out, head) {
		t.Error("excerpt must start with the head bytes")
	}
	if !strings.HasSuffix(out, tail) {
		t.Error("excerpt must end with the tail bytes")
	}
	if !strings.Contains(out, "[MARKER]") {
		t.Error("excerpt must contain the marker")
	}
	if len(out) > offloadHeadBytes+offloadTailBytes+200 {
		t.Errorf("excerpt too large: %d bytes", len(out))
	}
}

func TestOffloadExcerpt_RuneBoundaries(t *testing.T) {
	// Multi-byte runes straddling both cut points must not be split.
	s := strings.Repeat("é", (offloadHeadBytes+offloadTailBytes)*2) // 2-byte rune
	out := offloadExcerpt(s, "[M]")
	for _, part := range strings.Split(out, "[M]") {
		if strings.ContainsRune(part, '�') {
			t.Fatal("excerpt split a rune")
		}
	}
}

func TestClipRunesAndTailStart(t *testing.T) {
	if got := clipRunes("héllo", 2); got != "h" {
		t.Errorf("clipRunes mid-rune = %q, want %q", got, "h")
	}
	if got := clipRunes("abc", 10); got != "abc" {
		t.Errorf("clipRunes over-length = %q", got)
	}
	if n := tailStart("abc"); n != 3 {
		t.Errorf("tailStart short string = %d, want 3", n)
	}
}
