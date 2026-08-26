// Tests for the frame-band ratchet: the whole-frame height hold that keeps
// the chrome bottom-anchored across shrinks, and its release on print.
package consoleui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestFrameBand_HoldsHeightUntilPrintReleases verifies the whole frame's
// height ratchets (frameBand): ANY shrink — live region clearing, the
// composer losing a row, an approval box closing — pads back with blank top
// rows (an unpaired shrink would float the chrome, the renderer top-anchors
// shrinks), and a print releases exactly its own row count from the band.
func TestFrameBand_HoldsHeightUntilPrintReleases(t *testing.T) {
	m := anchorTestModel(t, 80, 24)
	m.deltaOrder = []string{"a"}
	m.deltas = map[string]string{"a": "one\ntwo\nthree"}
	tall := lipgloss.Height(m.View().Content)

	m.deltas = map[string]string{}
	m.deltaOrder = nil
	content := m.View().Content
	if got := lipgloss.Height(content); got != tall {
		t.Fatalf("frame height after live clear = %d, want band-held %d", got, tall)
	}
	// Padding rows carry a single space (a fully empty row is skipped by the
	// renderer's diff, which then never blanks vacated rows).
	if !strings.HasPrefix(content, " \n") {
		t.Errorf("band-held frame = %q..., want space-padded top rows", content[:40])
	}

	// A 1-row print releases 1 row of band.
	if cmd := m.printNewMessages(MessagePage{Messages: []Message{{Category: "user_input", Content: "hi"}}, Total: 1}); cmd == nil {
		t.Fatal("printNewMessages returned nil cmd")
	}
	if got := lipgloss.Height(m.View().Content); got != tall-1 {
		t.Errorf("frame height after 1-row print = %d, want %d", got, tall-1)
	}
}

// bandGapRows counts the blank padding rows the band adds above the frame —
// the visible gap between the last transcript row and the bottom panel.
func bandGapRows(m *model) int {
	pad := 0
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.TrimSpace(l) != "" {
			break
		}
		pad++
	}
	return pad
}

// bandTurn runs one full turn: a reply streams into the live region, the turn
// ends and the region clears (the shrink the band absorbs), then the finalized
// reply prints.
func bandTurn(m *model, reply string, total int) {
	m.status = "running"
	m.deltaOrder = []string{"a"}
	m.deltas = map[string]string{"a": strings.Repeat("streamed\n", 20)}
	m.View()
	m.status = "idle"
	m.deltas = map[string]string{}
	m.deltaOrder = nil
	m.View()
	m.printNewMessages(MessagePage{Messages: []Message{{Category: "assistant", Content: reply}}, Total: total})
}

// TestFrameBand_ReplyClosesTheGap is the regression guard for the blank band
// between the transcript and the bottom panel. The band inflates by the live
// region's height every turn and shrinks ONLY through printNewMessages'
// release, which is capped at the rows that print actually emits (each
// released row must be refilled by an insert or the shrink top-anchors and
// floats the chrome). An earlier "decay" pass tried to drain the remainder by
// emitting filler rows without decrementing the band: the padding survived, so
// the gap was permanent AND every turn dumped its filler into scrollback.
func TestFrameBand_ReplyClosesTheGap(t *testing.T) {
	m := anchorTestModel(t, 80, 24)

	for turn := 1; turn <= 3; turn++ {
		bandTurn(m, strings.Repeat("a line of the reply\n", 10), turn)
		if gap := bandGapRows(m); gap != 0 {
			t.Errorf("turn %d: %d blank rows between the transcript and the bottom panel, want 0", turn, gap)
		}
	}
}

// TestFrameBand_ShortReplyDeficitHealsOnNextRealReply pins the bounded
// worst case: replies too short to fund the full release leave a residual
// band, which must drain as soon as any normal-length reply prints — never
// ratchet up, never persist once there are rows to fund it.
func TestFrameBand_ShortReplyDeficitHealsOnNextRealReply(t *testing.T) {
	m := anchorTestModel(t, 80, 24)

	for turn := 1; turn <= 3; turn++ {
		bandTurn(m, "ok", turn)
	}
	short := bandGapRows(m)
	if short > liveRegionCap {
		t.Errorf("residual gap after one-line replies = %d, want <= liveRegionCap %d (the band must not ratchet up)", short, liveRegionCap)
	}

	bandTurn(m, strings.Repeat("a longer reply line\n", 10), 4)
	if gap := bandGapRows(m); gap != 0 {
		t.Errorf("gap after a normal reply = %d, want 0 — the deficit must drain once a print can fund it", gap)
	}
}
