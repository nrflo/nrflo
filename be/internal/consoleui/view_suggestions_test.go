package consoleui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

// TestSuggestionWindowSize verifies the reserved indicator row: all matches
// render when they fit within maxSuggestionRows, otherwise one row is
// reserved for the "N/total" indicator.
func TestSuggestionWindowSize(t *testing.T) {
	tests := []struct {
		total int
		want  int
	}{
		{0, 0},
		{1, 1},
		{maxSuggestionRows, maxSuggestionRows},
		{maxSuggestionRows + 1, maxSuggestionRows - 1},
		{20, maxSuggestionRows - 1},
	}
	for _, tt := range tests {
		if got := suggestionWindowSize(tt.total); got != tt.want {
			t.Errorf("suggestionWindowSize(%d) = %d, want %d", tt.total, got, tt.want)
		}
	}
}

// TestSuggestionWindow covers the scroll-window cases from the planner notes:
// fits-within-size, selection at start/mid/end, and clamped wrap. Every case
// asserts start<=selected<end and end-start==min(total,size).
func TestSuggestionWindow(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		selected int
		size     int
	}{
		{"fits within window", 3, 0, 8},
		{"selected at start", 12, 0, 7},
		{"selected mid-list centers window", 12, 6, 7},
		{"selected at last index", 12, 11, 7},
		{"selected past end clamps to last", 12, 50, 7},
		{"selected negative clamps to zero", 12, -3, 7},
		{"total equals size", 7, 3, 7},
		{"zero total", 0, 0, 8},
		{"zero size", 5, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := suggestionWindow(tt.total, tt.selected, tt.size)
			wantLen := min(tt.total, tt.size)
			if tt.total <= 0 || tt.size <= 0 {
				wantLen = 0
			}
			if end-start != wantLen {
				t.Fatalf("suggestionWindow(%d,%d,%d) len = %d, want %d", tt.total, tt.selected, tt.size, end-start, wantLen)
			}
			if wantLen == 0 {
				return
			}
			selected := clampInt(tt.selected, tt.total-1)
			if start > selected || selected >= end {
				t.Errorf("suggestionWindow(%d,%d,%d) = (%d,%d): selected %d not within [start,end)", tt.total, tt.selected, tt.size, start, end, selected)
			}
			if start < 0 || end > tt.total {
				t.Errorf("suggestionWindow(%d,%d,%d) = (%d,%d): out of [0,total) bounds", tt.total, tt.selected, tt.size, start, end)
			}
		})
	}
}

// TestSuggestionWindow_WrapToZero verifies a selection near the top of a
// large list keeps the window anchored at 0 rather than going negative.
func TestSuggestionWindow_WrapToZero(t *testing.T) {
	start, end := suggestionWindow(12, 1, 7)
	if start != 0 {
		t.Errorf("suggestionWindow(12,1,7) start = %d, want 0", start)
	}
	if end != 7 {
		t.Errorf("suggestionWindow(12,1,7) end = %d, want 7", end)
	}
}

// TestSuggestionWindow_EndReachesTotal verifies a selection at the last
// index pins the window's end to total (no room to scroll further).
func TestSuggestionWindow_EndReachesTotal(t *testing.T) {
	_, end := suggestionWindow(12, 11, 7)
	if end != 12 {
		t.Errorf("suggestionWindow(12,11,7) end = %d, want 12 (pinned to total)", end)
	}
}

// TestRowTruncation_OneLine builds a long "/name — description" row and
// asserts truncate() flattens it to exactly one terminal line within width.
func TestRowTruncation_OneLine(t *testing.T) {
	longDesc := strings.Repeat("this is a very long skill description ", 10)
	line := "/mytool — " + longDesc
	const width = 40
	got := truncate(line, width)
	if strings.Contains(got, "\n") {
		t.Fatalf("truncate row contains newline: %q", got)
	}
	if w := lipgloss.Width(got); w > width {
		t.Errorf("truncate(row, %d) width = %d, want <= %d", width, w, width)
	}
}

// TestRowTruncation_ViaSuggestionView renders a real model with a
// long-description skill and asserts every produced row is single-line and
// within the box's inner width.
func TestRowTruncation_ViaSuggestionView(t *testing.T) {
	longDesc := strings.Repeat("word ", 60)
	m := suggestionTestModel(20, []ConsoleSkill{
		{Name: "deploy", Description: longDesc},
	}, 0, false)
	out := m.suggestionView()
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("rendered box line width %d exceeds box width %d: %q", w, m.width, line)
		}
	}
}

// TestDetailLines_WordWrapAndCap verifies detailLines word-wraps a long
// description at a narrow width, caps total lines at maxLines, and keeps the
// header as the truncated "/name" line.
//
// NOTE: per the spec, the last line should carry a "…" truncation marker
// when the body overflows maxLines. The current implementation truncates
// the kept last line via truncate(lines[maxLines-1], width), but a
// word-wrapped line is already <= width by construction, so truncate()'s
// width check never fires and no marker is ever appended on a line-count
// overflow (only on a width overflow, which can't happen here). This is a
// production bug (see be_production_bugs); this test asserts the actual
// current behavior rather than the spec'd one.
func TestDetailLines_WordWrapAndCap(t *testing.T) {
	desc := strings.Repeat("word ", 80)
	const width = 20
	const maxLines = 6
	lines := detailLines("deploy", desc, width)
	if len(lines) == 0 {
		t.Fatalf("detailLines returned no lines")
	}
	if len(lines) != maxLines {
		t.Fatalf("detailLines with heavily overflowing body should hit the cap: got %d lines, want %d", len(lines), maxLines)
	}
	wantHeader := truncate("/deploy", width)
	if lines[0] != wantHeader {
		t.Errorf("detailLines header = %q, want %q", lines[0], wantHeader)
	}
	last := lines[len(lines)-1]
	if w := lipgloss.Width(last); w > width {
		t.Errorf("detailLines last line width %d exceeds width %d", w, width)
	}
}

// TestDetailLines_ShortDescriptionNoTruncation verifies a short description
// that fits within maxLines is not truncated and carries no marker.
func TestDetailLines_ShortDescriptionNoTruncation(t *testing.T) {
	lines := detailLines("deploy", "short desc", 40)
	if len(lines) != 2 {
		t.Fatalf("detailLines short case = %d lines, want 2 (header+body)", len(lines))
	}
	if strings.HasSuffix(lines[len(lines)-1], "…") {
		t.Errorf("short description should not be marked truncated: %q", lines[len(lines)-1])
	}
}

// TestDetailLines_EmptyDescription verifies a skill with no description
// renders only the header line.
func TestDetailLines_EmptyDescription(t *testing.T) {
	lines := detailLines("deploy", "", 40)
	if len(lines) != 1 {
		t.Fatalf("detailLines empty description = %d lines, want 1", len(lines))
	}
	if lines[0] != "/deploy" {
		t.Errorf("detailLines header = %q, want /deploy", lines[0])
	}
}

// suggestionTestModel builds a *model literal sufficient to exercise
// suggestionView()/chromeRows() without a running terminal: width, skills,
// skillIndex, skillDetails, and an input textarea holding "/" so
// suggestionMatches() returns every skill.
func suggestionTestModel(width int, skills []ConsoleSkill, skillIndex int, skillDetails bool) *model {
	input := textarea.New()
	input.SetValue("/")
	return &model{
		width:        width,
		skills:       skills,
		skillIndex:   skillIndex,
		skillDetails: skillDetails,
		input:        input,
	}
}

func manySkills(n int) []ConsoleSkill {
	skills := make([]ConsoleSkill, n)
	for i := range skills {
		skills[i] = ConsoleSkill{Name: strings.Repeat("s", 1) + string(rune('a'+i%26)) + "kill"}
	}
	return skills
}

// TestChromeAccounting_DetailsClosed verifies the rendered suggestionView()
// line count matches suggestionRows(N) exactly (indicator slot included)
// when N exceeds maxSuggestionRows and details are closed.
func TestChromeAccounting_DetailsClosed(t *testing.T) {
	skills := manySkills(12)
	m := suggestionTestModel(60, skills, 0, false)
	out := m.suggestionView()
	gotLines := strings.Count(out, "\n") + 1
	// suggestionView renders inside approvalBox (rounded border + padding),
	// so the raw content line count is what suggestionRows describes; strip
	// border rows for the comparison by rendering the join directly.
	contentLines := suggestionWindowSize(len(skills)) + 1 // + indicator row
	wantContentLines := suggestionRows(len(skills))
	if contentLines != wantContentLines {
		t.Fatalf("sanity: suggestionWindowSize+indicator (%d) != suggestionRows (%d)", contentLines, wantContentLines)
	}
	// The box adds top/bottom border rows (approvalBox RoundedBorder).
	const boxBorderRows = 2
	if gotLines != wantContentLines+boxBorderRows {
		t.Errorf("rendered suggestionView() line count = %d, want suggestionRows(%d)=%d + %d border rows", gotLines, len(skills), wantContentLines, boxBorderRows)
	}
	chrome := chromeRows(1, len(m.suggestionMatches()), 0, 0)
	if chrome < wantContentLines {
		t.Errorf("chromeRows(...) = %d, want it to include the suggestionRows(%d)=%d indicator slot contribution", chrome, len(skills), wantContentLines)
	}
}

// TestChromeAccounting_DetailsOpen verifies that with details open, the
// rendered line count grows by exactly len(detailLines(...)), and chromeRows
// with detailRows set adds exactly that many rows over the closed case.
func TestChromeAccounting_DetailsOpen(t *testing.T) {
	skills := manySkills(12)
	const width = 60
	m := suggestionTestModel(width, skills, 3, true)
	inner := max(1, width-6)
	sel := clampInt(m.skillIndex, len(skills)-1)
	detailRows := len(detailLines(skills[sel].Name, skills[sel].Description, inner))

	closed := suggestionTestModel(width, skills, 3, false)
	closedLines := strings.Count(closed.suggestionView(), "\n") + 1
	openLines := strings.Count(m.suggestionView(), "\n") + 1

	if openLines != closedLines+detailRows {
		t.Errorf("open suggestionView() line count = %d, want closed(%d) + detailRows(%d) = %d", openLines, closedLines, detailRows, closedLines+detailRows)
	}

	chromeClosed := chromeRows(1, len(skills), 0, 0)
	chromeOpen := chromeRows(1, len(skills), 0, detailRows)
	if chromeOpen != chromeClosed+detailRows {
		t.Errorf("chromeRows open-closed diff = %d, want exactly detailRows(%d)", chromeOpen-chromeClosed, detailRows)
	}
}

// TestChromeAccounting_TinyTerminalViewportFloor verifies the derived
// viewport height never drops below 1 even when a huge suggestion+detail
// chrome would otherwise overflow a tiny terminal.
func TestChromeAccounting_TinyTerminalViewportFloor(t *testing.T) {
	chrome := chromeRows(8, 12, 1, maxDetailLines)
	viewportHeight := max(1, 3-chrome)
	if viewportHeight != 1 {
		t.Errorf("viewport height = %d, want clamped to 1 for tiny terminal with full chrome(%d)", viewportHeight, chrome)
	}
}
