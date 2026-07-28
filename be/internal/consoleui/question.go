package consoleui

import (
	"encoding/json"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// askUserQuestionTool mirrors spawner.AskUserQuestionTool by value (consoleui
// talks to the server over REST/WS only, never imports spawner).
const askUserQuestionTool = "AskUserQuestion"

// chatQuestion is one entry of an AskUserQuestion payload (Approval.Input).
type chatQuestion struct {
	Question    string `json:"question"`
	Header      string `json:"header"`
	MultiSelect bool   `json:"multiSelect"`
	Options     []struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

// questionState drives the interactive card for the front approval when it is
// an AskUserQuestion: questions are answered one at a time (number keys pick
// an option, typed text is a free-form answer, multiSelect toggles + enter
// confirms), and the combined answer resolves the approval with
// decision=answer. Keyed by approval id; syncQuestion rebuilds it whenever
// the front approval changes and clears it when the card resolves.
type questionState struct {
	id        string
	questions []chatQuestion
	idx       int
	answers   []string
	picks     map[int]bool
	sent      bool
}

// syncQuestion keeps m.qa keyed to the front approval. An AskUserQuestion
// whose payload fails to parse keeps questions empty — questionActive stays
// false and the generic approval card takes over (its allow maps to the
// server-side plain-text redirect, so the chat cannot deadlock).
func (m *model) syncQuestion() {
	if len(m.approvals) == 0 || m.approvals[0].Tool != askUserQuestionTool {
		m.qa = questionState{}
		return
	}
	front := m.approvals[0]
	if m.qa.id == front.ID {
		return
	}
	m.qa = questionState{id: front.ID, questions: parseQuestions(front.Input), picks: map[int]bool{}}
}

func parseQuestions(input string) []chatQuestion {
	var payload struct {
		Questions []chatQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return nil
	}
	return payload.Questions
}

func (m *model) questionActive() bool {
	return m.qa.id != "" && len(m.qa.questions) > 0
}

// handleQuestionKey consumes option/confirm keys for the active question.
// Number keys act only while the composer is empty, so typing a free-form
// answer containing digits is never hijacked; every other key falls through
// to the composer.
func (m *model) handleQuestionKey(key string) (tea.Cmd, bool) {
	if m.qa.sent {
		return nil, false
	}
	q := m.qa.questions[m.qa.idx]
	composerEmpty := strings.TrimSpace(m.input.Value()) == ""
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && composerEmpty {
		i := int(key[0] - '1')
		if i >= len(q.Options) {
			return nil, true
		}
		if q.MultiSelect {
			m.qa.picks[i] = !m.qa.picks[i]
			return nil, true
		}
		return m.recordAnswer(q.Options[i].Label), true
	}
	if key == "enter" {
		if text := strings.TrimSpace(m.input.Value()); text != "" {
			m.input.Reset()
			return m.recordAnswer(text), true
		}
		if q.MultiSelect {
			labels := make([]string, 0, len(q.Options))
			for i, o := range q.Options {
				if m.qa.picks[i] {
					labels = append(labels, o.Label)
				}
			}
			if len(labels) > 0 {
				return m.recordAnswer(strings.Join(labels, ", ")), true
			}
		}
		// Nothing picked and nothing typed — swallow the enter rather than
		// sending an empty chat message under the card.
		return nil, true
	}
	return nil, false
}

// recordAnswer stores the current question's answer, advances to the next
// question, and on the last one submits the combined answer.
func (m *model) recordAnswer(answer string) tea.Cmd {
	m.qa.answers = append(m.qa.answers, answer)
	m.qa.picks = map[int]bool{}
	if m.qa.idx+1 < len(m.qa.questions) {
		m.qa.idx++
		return nil
	}
	m.qa.sent = true
	id := m.qa.id
	final := composeAnswer(m.qa.questions, m.qa.answers)
	return action("answer", func() error { return m.client.Answer(m.ctx, id, final) })
}

// composeAnswer flattens per-question answers into the single string the
// model receives: bare for one question, "Header: answer" pairs for several.
func composeAnswer(questions []chatQuestion, answers []string) string {
	if len(answers) == 1 {
		return answers[0]
	}
	parts := make([]string, 0, len(answers))
	for i, answer := range answers {
		label := questions[i].Header
		if label == "" {
			label = questions[i].Question
		}
		parts = append(parts, label+": "+answer)
	}
	return strings.Join(parts, "; ")
}

// questionView renders the interactive card for the active question.
func (m *model) questionView() string {
	q := m.qa.questions[m.qa.idx]
	width := max(20, m.width-6)
	title := "Question"
	if len(m.qa.questions) > 1 {
		title += " " + strconv.Itoa(m.qa.idx+1) + "/" + strconv.Itoa(len(m.qa.questions))
	}
	if q.Header != "" {
		title += " · " + q.Header
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(warn).Render(truncate(title, width)),
		fitWidth(q.Question, width),
	}
	for i, o := range q.Options {
		marker := "[" + strconv.Itoa(i+1) + "]"
		if q.MultiSelect && m.qa.picks[i] {
			marker = "[x]"
		}
		line := marker + " " + o.Label
		if o.Description != "" {
			line += " — " + o.Description
		}
		lines = append(lines, truncate(line, width))
	}
	hint := "1-" + strconv.Itoa(min(9, len(q.Options))) + " select · type + enter custom answer"
	if q.MultiSelect {
		hint = "1-" + strconv.Itoa(min(9, len(q.Options))) + " toggle · enter confirm · type + enter custom answer"
	}
	if m.qa.sent {
		hint = "answer sent…"
	}
	lines = append(lines, mutedStyle.Render(hint))
	return approvalBox.Width(max(1, m.width-2)).Render(strings.Join(lines, "\n"))
}
