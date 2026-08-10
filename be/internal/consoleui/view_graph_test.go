package consoleui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"be/internal/types"
)

func fixtureFlow() types.SessionFlowResponse {
	ctx71 := 71
	return types.SessionFlowResponse{
		RootSessionID: "root-session-id",
		Nodes: []types.SessionFlowNode{
			{SessionID: "root-session-id", Kind: "console_chat", Status: "user_interactive",
				ModelID: "opus-5", StartedAt: "2026-08-10T10:00:00Z"},
			{SessionID: "worker-1", Kind: "workflow_agent", AgentType: "_t2_extractor", Status: "completed",
				Result: "pass", ModelID: "haiku-4-5", ContextLeft: &ctx71,
				StartedAt: "2026-08-10T10:01:00Z", EndedAt: "2026-08-10T10:01:54Z"},
			{SessionID: "worker-2", Kind: "workflow_agent", AgentType: "_t1_executor", Status: "running",
				ModelID: "sonnet-5", StartedAt: "2026-08-10T10:02:00Z"},
		},
		Edges: []types.SessionFlowEdge{
			{FromSessionID: "root-session-id", ToSessionID: "worker-1", Kind: "delegate"},
			{FromSessionID: "root-session-id", ToSessionID: "worker-2", Kind: "delegate"},
		},
	}
}

func TestRenderFlowTree(t *testing.T) {
	t.Parallel()
	now, _ := time.Parse(time.RFC3339, "2026-08-10T10:03:00Z")
	lines := renderFlowTree(fixtureFlow(), 120, now)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansi.Strip(l)
	}
	if !strings.Contains(plain[0], "console_chat") || !strings.Contains(plain[0], "opus-5") {
		t.Errorf("root line = %q, want kind + model", plain[0])
	}
	if !strings.Contains(plain[1], "├─ ") || !strings.Contains(plain[1], "_t2_extractor") ||
		!strings.Contains(plain[1], "54s") || !strings.Contains(plain[1], "ctx 71%") ||
		!strings.Contains(plain[1], "[delegate]") {
		t.Errorf("worker-1 line = %q, want connector + agent + elapsed + ctx + edge kind", plain[1])
	}
	if !strings.Contains(plain[2], "└─ ") || !strings.Contains(plain[2], "_t1_executor") ||
		!strings.Contains(plain[2], "1m00s") || !strings.Contains(plain[2], "running") {
		t.Errorf("worker-2 line = %q, want last connector + running elapsed", plain[2])
	}
}

// A cyclic edge set must not recurse forever — the second visit renders the
// node line and stops.
func TestRenderFlowTree_CycleSafe(t *testing.T) {
	t.Parallel()
	flow := types.SessionFlowResponse{
		RootSessionID: "a",
		Nodes:         []types.SessionFlowNode{{SessionID: "a", Kind: "console_chat"}, {SessionID: "b", Kind: "workflow_agent"}},
		Edges: []types.SessionFlowEdge{
			{FromSessionID: "a", ToSessionID: "b", Kind: "delegate"},
			{FromSessionID: "b", ToSessionID: "a", Kind: "origin"},
		},
	}
	lines := renderFlowTree(flow, 80, time.Time{})
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (a, b, revisited a):\n%s", len(lines), strings.Join(lines, "\n"))
	}
}

// ctrl+t opens the alt-screen overlay (keys stop reaching the composer),
// esc closes it and reloads the tail to flush suppressed prints.
func TestGraphOverlay_ToggleAndKeySwallow(t *testing.T) {
	t.Parallel()
	m := &model{detail: ChatDetail{SessionID: "s1", ProjectID: "p1", Engine: "claude", Model: "opus-5"},
		deltas: map[string]string{}, input: textarea.New(), client: &Client{}}
	m.resize(100, 30)

	if cmd, handled := m.handleKey(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}); !handled || cmd == nil {
		t.Fatal("ctrl+t not handled or no fetch cmd")
	}
	if !m.graph.open {
		t.Fatal("graph.open = false after ctrl+t")
	}
	if view := m.View(); !view.AltScreen {
		t.Error("overlay view must use the alt screen")
	}

	m.applyGraph(graphMsg{flow: fixtureFlow(), stats: types.SessionStatsResponse{SelfCostUSD: 0.42}})
	content := m.View().Content
	if !strings.Contains(content, "Session flow") || !strings.Contains(content, "_t2_extractor") {
		t.Errorf("overlay content missing header/tree:\n%s", content)
	}

	// Typing while open never reaches the composer.
	if _, handled := m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"}); !handled {
		t.Error("overlay must swallow plain keys")
	}
	if m.input.Value() != "" {
		t.Errorf("composer value = %q, want empty", m.input.Value())
	}

	if cmd, handled := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}); !handled || cmd == nil {
		t.Fatal("esc not handled or no reload cmd")
	}
	if m.graph.open {
		t.Error("graph.open = true after esc")
	}
	if view := m.View(); view.AltScreen {
		t.Error("inline view must not use the alt screen after close")
	}
}
