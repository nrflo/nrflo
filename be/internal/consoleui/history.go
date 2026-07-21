package consoleui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	historyPageSize   = 200
	historyWindowSize = 1000
	maxDeltaItems     = 64
	maxDeltaBytes     = 128 * 1024
)

func (m *model) applyDetail(detail ChatDetail) {
	m.detail = detail
	m.status = detail.Turn
	m.approvals = append([]Approval{}, detail.PendingApprovals...)
	m.deltas = make(map[string]string, len(detail.LiveItems))
	m.deltaOrder = m.deltaOrder[:0]
	start := max(0, len(detail.LiveItems)-maxDeltaItems)
	for _, item := range detail.LiveItems[start:] {
		m.deltaOrder = append(m.deltaOrder, item.ID)
		m.deltas[item.ID] = trimDeltaTail(item.Text)
	}
	m.thinking, m.thinkingID = "", ""
	if detail.Thinking != nil {
		m.thinking, m.thinkingID = trimDeltaTail(detail.Thinking.Text), detail.Thinking.ID
	}
}

func (m *model) applySync(detail ChatDetail, page MessagePage) {
	m.applyHistory(page, false, 0)
	m.applyDetail(detail)
}

func (m *model) appendOptimisticMessage(message Message) {
	m.messages = append(m.messages, message)
	if len(m.messages) > historyWindowSize {
		m.messages = m.messages[len(m.messages)-historyWindowSize:]
		m.historyOffset++
	}
	m.historyDirty = true
}

func (m *model) applyHistory(page MessagePage, prepend bool, offset int) {
	if prepend {
		m.messages = append(append([]Message{}, page.Messages...), m.messages...)
		m.historyOffset = offset
		if len(m.messages) > historyWindowSize {
			m.messages = m.messages[:historyWindowSize]
		}
	} else {
		m.messages = append([]Message{}, page.Messages...)
		m.historyOffset = max(0, page.Total-len(page.Messages))
		m.deltas = make(map[string]string)
		m.deltaOrder = nil
	}
	m.historyTotal = page.Total
	if len(m.renderCache) > historyWindowSize*2 {
		m.renderCache = make(map[string]string)
	}
	m.historyDirty = true
	m.refreshTranscript()
}

func (m *model) appendDelta(id, text string) {
	if id == "" {
		id = "assistant"
	}
	if _, exists := m.deltas[id]; !exists {
		m.deltaOrder = append(m.deltaOrder, id)
		if len(m.deltaOrder) > maxDeltaItems {
			delete(m.deltas, m.deltaOrder[0])
			m.deltaOrder = m.deltaOrder[1:]
		}
	}
	m.deltas[id] = trimDeltaTail(m.deltas[id] + text)
}

func trimDeltaTail(value string) string {
	if len(value) <= maxDeltaBytes {
		return value
	}
	return strings.ToValidUTF8(value[len(value)-maxDeltaBytes:], "")
}

func renderCacheKey(message Message, width int) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", width, message.Category, message.Content, message.Payload)
}

func (m *model) applySearch() {
	query := strings.TrimSpace(m.search.Value())
	m.viewport.ClearHighlights()
	if query == "" {
		m.searchStatus = ""
		return
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(query))
	matches := re.FindAllStringIndex(m.viewport.GetContent(), -1)
	m.viewport.SetHighlights(matches)
	m.searchStatus = fmt.Sprintf("%d matches for %q", len(matches), query)
}

func (m *model) visibleTranscript() string {
	return normalizeCopyText(ansi.Strip(m.viewport.View()))
}

// normalizeCopyText strips non-breaking spaces, trims trailing whitespace
// from each line, and trims the overall result.
func normalizeCopyText(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// rawTranscript joins each message's non-empty raw Content with a blank-line
// separator, bypassing the rendered viewport entirely.
func rawTranscript(messages []Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Content == "" {
			continue
		}
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n\n")
}
