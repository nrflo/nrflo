package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

func suggestionTestModel(width int, skills []ConsoleSkill, skillIndex int) *model {
	input := textarea.New()
	input.SetValue("/")
	return &model{width: width, skills: skills, skillIndex: skillIndex, input: input}
}

func TestSuggestionView_ShowsMultipleOptionsWithinWidth(t *testing.T) {
	m := suggestionTestModel(40, []ConsoleSkill{
		{Name: "deploy", Description: strings.Repeat("long description ", 20)},
		{Name: "review"},
	}, 0)
	out := m.suggestionView()
	if strings.Contains(out, "\n") {
		t.Fatalf("suggestionView() = %q, want one line", out)
	}
	if got := lipgloss.Width(out); got > m.width {
		t.Errorf("suggestionView() width = %d, want <= %d", got, m.width)
	}
	if !strings.Contains(out, "/invoke") || !strings.Contains(out, "/deploy") {
		t.Errorf("suggestionView() = %q, want multiple visible options", out)
	}
	if !strings.Contains(out, "1/3") { // reserved /invoke row + two skills
		t.Errorf("suggestionView() = %q, want selection position", out)
	}
}

func TestSuggestionView_TracksSelection(t *testing.T) {
	m := suggestionTestModel(80, []ConsoleSkill{{Name: "alpha"}, {Name: "beta"}}, 2)
	out := m.suggestionView()
	if !strings.Contains(out, "/beta") || !strings.Contains(out, "3/3") {
		t.Errorf("suggestionView() = %q, want selected /beta at 3/3", out)
	}
}

func TestSuggestionView_NarrowWidth(t *testing.T) {
	m := suggestionTestModel(3, nil, 0)
	if got := lipgloss.Width(m.suggestionView()); got > m.width {
		t.Errorf("suggestionView() width = %d, want <= %d", got, m.width)
	}
}

func TestSuggestionStatus_DoesNotGrowFrameBand(t *testing.T) {
	m := anchorTestModel(t, 80, 24)
	baseHeight := lipgloss.Height(m.View().Content)
	m.skills = []ConsoleSkill{{Name: "address-comments", Description: "Address review comments"}}
	m.input.SetValue("/address-comments")
	openHeight := lipgloss.Height(m.View().Content)
	if openHeight != baseHeight {
		t.Fatalf("frame height with suggestions = %d, want base height %d", openHeight, baseHeight)
	}

	m.input.Reset()
	if gap := bandGapRows(m); gap != 0 {
		t.Errorf("gap after suggestions close = %d rows, want 0", gap)
	}
}

func TestWorkingLine_RendersAboveComposer(t *testing.T) {
	m := anchorTestModel(t, 80, 24)
	m.status = "running"
	lines := strings.Split(m.View().Content, "\n")
	working, composer, status := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "working…"):
			working = i
		case strings.Contains(line, "╭"):
			composer = i
		case strings.Contains(line, "claude/opus"):
			status = i
		}
	}
	if working < 0 || composer < 0 || status < 0 || working >= composer || composer >= status {
		t.Errorf("line order working=%d composer=%d status=%d, want working above composer and status below", working, composer, status)
	}
}
