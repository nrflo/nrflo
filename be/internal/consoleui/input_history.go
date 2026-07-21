package consoleui

import "fmt"

// historyLimit caps how many prior sends are retained for Up/Down recall.
const historyLimit = 100

// inputHistory is the pure state machine backing shell/Claude-CLI-style
// Up/Down recall in the composer: no *model, no terminal, fully
// unit-testable. entries are oldest→newest; index == len(entries) is the
// draft slot (not browsing, the composer holds the user's in-progress text);
// index in [0,len(entries)) means browsing that entry, with draft holding the
// composer text that was in flight when browsing started (restored on
// stepping past the newest entry).
type inputHistory struct {
	entries []string
	index   int
	draft   string
}

// newHistory seeds an inputHistory from a message page's persisted
// "user_input" rows, in order, deduped/capped via appendEntry. The result
// starts at the draft slot (not browsing).
func newHistory(messages []Message) inputHistory {
	var entries []string
	for _, msg := range messages {
		if msg.Category != "user_input" {
			continue
		}
		entries = appendEntry(entries, msg.Content)
	}
	return inputHistory{entries: entries, index: len(entries)}
}

// newHistoryFromContents seeds an inputHistory from a flat, oldest→newest
// list of raw message contents (a project-scoped aggregate fetched via
// Client.History), applying the same global keep-last dedup and
// historyLimit cap as dedupSeed. The result starts at the draft slot.
func newHistoryFromContents(contents []string) inputHistory {
	entries := dedupSeed(contents)
	return inputHistory{entries: entries, index: len(entries)}
}

// dedupSeed applies a global keep-last dedup (a duplicate entry is dropped in
// favor of its most recent occurrence, order otherwise preserved) then caps
// the result to the most recent historyLimit entries. Pure and independent
// of appendEntry's consecutive-only dedup, which stays as-is for record().
func dedupSeed(contents []string) []string {
	last := make(map[string]int, len(contents))
	for i, c := range contents {
		last[c] = i
	}
	out := make([]string, 0, len(contents))
	for i, c := range contents {
		if last[c] == i {
			out = append(out, c)
		}
	}
	if len(out) > historyLimit {
		out = out[len(out)-historyLimit:]
	}
	return out
}

// appendEntry appends msg to entries with consecutive-dedup (a repeat of the
// last entry is a no-op) and drops the oldest entries beyond historyLimit.
// Copy-on-write: never mutates the input slice.
func appendEntry(entries []string, msg string) []string {
	if len(entries) > 0 && entries[len(entries)-1] == msg {
		return entries
	}
	next := make([]string, 0, len(entries)+1)
	next = append(next, entries...)
	next = append(next, msg)
	if len(next) > historyLimit {
		next = next[len(next)-historyLimit:]
	}
	return next
}

// historyPrev steps to an older entry. current is the composer's live text,
// saved as the draft the first time browsing starts. Returns handled=false
// when there is no history to browse.
func historyPrev(h inputHistory, current string) (inputHistory, string, bool) {
	if len(h.entries) == 0 {
		return h, "", false
	}
	if h.index >= len(h.entries) {
		h.draft = current
		h.index = len(h.entries) - 1
		return h, h.entries[h.index], true
	}
	if h.index > 0 {
		h.index--
	}
	return h, h.entries[h.index], true
}

// historyNext steps to a newer entry, restoring the saved draft once stepped
// past the newest entry. Returns handled=false when already at the draft
// slot (not browsing) so the caller can fall back to normal cursor movement.
func historyNext(h inputHistory) (inputHistory, string, bool) {
	if h.index >= len(h.entries) {
		return h, "", false
	}
	h.index++
	if h.index >= len(h.entries) {
		return h, h.draft, true
	}
	return h, h.entries[h.index], true
}

// record appends a sent message and resets browsing back to the draft slot.
func (h inputHistory) record(msg string) inputHistory {
	h.entries = appendEntry(h.entries, msg)
	h.index = len(h.entries)
	h.draft = ""
	return h
}

// indicator returns the footer label ("History <pos>/<total>", 1-based from
// oldest) and whether it should be shown; false while at the draft slot.
func (h inputHistory) indicator() (string, bool) {
	total := len(h.entries)
	if h.index >= total {
		return "", false
	}
	return fmt.Sprintf("History %d/%d", h.index+1, total), true
}

// handleHistoryKey recalls prev/next history entries when Up/Down is pressed
// at the composer's first/last-line boundary (interior Up/Down move the
// cursor instead). Mirrors handleSuggestionKey's bool-consumed contract.
func (m *model) handleHistoryKey(key string) bool {
	if m.suggestionsOpen() || m.invoke.active {
		return false
	}
	var (
		h       inputHistory
		value   string
		handled bool
	)
	switch key {
	case "up":
		if m.input.Line() != 0 {
			return false
		}
		h, value, handled = historyPrev(m.history, m.input.Value())
	case "down":
		if m.input.Line() != m.input.LineCount()-1 {
			return false
		}
		h, value, handled = historyNext(m.history)
	default:
		return false
	}
	if !handled {
		return false
	}
	m.history = h
	m.input.SetValue(value)
	m.input.MoveToEnd()
	return true
}
