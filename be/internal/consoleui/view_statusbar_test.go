package consoleui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestStatusBar_YoloBadge verifies the status bar shows a "YOLO" badge only
// when detail.Yolo is true.
func TestStatusBar_YoloBadge(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "codex", ProjectID: "proj-1"}}

	if got := m.statusBar(); strings.Contains(got, "YOLO") {
		t.Errorf("statusBar() with Yolo=false = %q, want no YOLO badge", got)
	}

	m.detail.Yolo = true
	if got := m.statusBar(); !strings.Contains(got, "YOLO") {
		t.Errorf("statusBar() with Yolo=true = %q, want a YOLO badge", got)
	}
}

// TestStatusBar_Profile verifies the profile segment renders the profile
// name (as used by --profile) when set, and is omitted entirely when empty.
func TestStatusBar_Profile(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", Model: "opus-5", ProjectID: "proj-1"}}
	if got := m.statusBar(); strings.Contains(got, "t0-bare") {
		t.Errorf("statusBar() with empty Profile = %q, want no profile segment", got)
	}

	m.detail.Profile = "t0-bare"
	if got := m.statusBar(); !strings.Contains(got, "t0-bare") {
		t.Errorf("statusBar() with Profile=t0-bare = %q, want the profile name rendered", got)
	}
}

// TestStatusBar_GitSegment verifies the git segment renders `[branch: +A -D]`
// when dirty, `[branch]` when clean, and is omitted entirely when no branch
// is known (not a repo / git unavailable).
func TestStatusBar_GitSegment(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", ProjectID: "proj-1"}}
	if got := m.statusBar(); strings.Contains(ansi.Strip(got), "[") {
		t.Errorf("statusBar() with no GitBranch = %q, want no bracket segment", got)
	}

	m.detail.GitBranch = "master"
	if got := m.statusBar(); !strings.Contains(got, "[master]") {
		t.Errorf("statusBar() clean repo = %q, want [master]", got)
	}

	m.detail.GitAdded = 20
	m.detail.GitDeleted = 3
	if got := m.statusBar(); !strings.Contains(got, "[master: +20 -3]") {
		t.Errorf("statusBar() dirty repo = %q, want [master: +20 -3]", got)
	}
}

// TestStatusBar_NoBanner verifies the hardcoded leading "nrflo" banner
// segment is gone.
func TestStatusBar_NoBanner(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", Model: "opus-5", ProjectID: "proj-1"}}
	if got := ansi.Strip(m.statusBar()); strings.Contains(got, "nrflo") {
		t.Errorf("statusBar() = %q, want no nrflo banner segment", got)
	}
}

// TestStatusBar_EngineModelFormat verifies the engine/model segment renders
// as "engine/model" with no spaces around the slash.
func TestStatusBar_EngineModelFormat(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", Model: "opus-5", ProjectID: "proj-1"}}
	if got := ansi.Strip(m.statusBar()); !strings.Contains(got, "claude/opus-5") {
		t.Errorf("statusBar() = %q, want to contain %q", got, "claude/opus-5")
	}
}

// TestStatusBar_CwdSegment verifies the working directory renders as the
// last segment, compacted, and is omitted when WorkDir is empty.
func TestStatusBar_CwdSegment(t *testing.T) {
	m := &model{detail: ChatDetail{Engine: "claude", ProjectID: "proj-1"}}
	if got := ansi.Strip(m.statusBar()); strings.Contains(got, "/tmp/nrflo") {
		t.Errorf("statusBar() with no WorkDir = %q, want no cwd segment", got)
	}

	m.detail.WorkDir = "/tmp/nrflo"
	got := ansi.Strip(m.statusBar())
	if !strings.HasSuffix(got, "/tmp/nrflo") {
		t.Errorf("statusBar() = %q, want to end with the cwd segment", got)
	}
}

// TestStatusBar_TruncatesToSingleRow verifies a status bar longer than the
// terminal width is truncated to one physical row: a wrapped bar breaks the
// chrome height math and can leak into scrollback via tea.Println inserts.
func TestStatusBar_TruncatesToSingleRow(t *testing.T) {
	m := &model{detail: ChatDetail{
		Engine: "claude", Model: "claude-fable-5", ProjectID: "a-rather-long-project-identifier",
		SessionApprovals: []string{"Bash", "Read", "Edit", "WebFetch", "WebSearch", "NotebookEdit"},
	}}
	m.width = 40
	if got := ansi.StringWidth(m.statusBar()); got > 40 {
		t.Errorf("statusBar() display width = %d, want <= terminal width 40", got)
	}
	if bar := m.statusBar(); strings.Contains(bar, "\n") {
		t.Errorf("statusBar() = %q, want a single line", bar)
	}
}
