package consoleui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// clearScreenSeq erases the visible screen (2J), erases scrollback (3J), then
// parks the cursor on the terminal's bottom row (999 clamps). The inline
// renderer anchors its region at the cursor, and insertAbove's math assumes
// the frame's last row is the terminal's last row — starting at the bottom
// makes that hold from the first frame, with no padding needed.
const clearScreenSeq = "\x1b[2J\x1b[3J\x1b[999;1H"

type model struct {
	ctx            context.Context
	client         *Client
	events         <-chan streamUpdate
	detail         ChatDetail
	printedTotal   int
	pendingUser    string
	historyPrinted bool
	initialPage    MessagePage
	deltas         map[string]string
	deltaOrder     []string
	thinking       string
	thinkingID     string

	liveBand  int
	approvals []Approval
	connected bool
	status    string
	tool      runningTool
	bgRunning int
	lastErr   string
	width     int
	height    int
	ready     bool
	notice    string

	input   textarea.Model
	spin    spinner.Model
	history inputHistory

	skills          []ConsoleSkill
	skillIndex      int
	skillsDismissed bool
	skillsFetched   bool
	skillDetails    bool

	tools        []ConsoleTool
	toolsFetched bool
	invoke       invokeState
}

type historyMsg struct {
	page MessagePage
	err  error
}

type syncMsg struct {
	detail ChatDetail
	page   MessagePage
	err    error
}

type actionMsg struct {
	action string
	err    error
}

type toolsMsg struct {
	tools []ConsoleTool
	err   error
}

func Run(ctx context.Context, cfg Config) error {
	client := NewClient(cfg)
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	detail, err := client.Detail(loadCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("load console chat: %w", err)
	}
	if client.project == "" {
		// Remote attach may omit the project; the chat row knows it, and the
		// WS project subscription (bg counter) needs it.
		client.project = detail.ProjectID
	}
	page, err := client.TailMessages(loadCtx, historyPageSize)
	inputHist := newHistory(page.Messages)
	if err == nil && cfg.Project != "" {
		if contents, histErr := client.History(loadCtx, historyLimit); histErr == nil {
			inputHist = newHistoryFromContents(contents)
		}
	}
	cancel()
	if err != nil {
		return fmt.Errorf("load console history: %w", err)
	}

	input := textarea.New()
	input.Placeholder = "Ask nrflo…"
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.MinHeight = 1
	input.MaxHeight = 8
	input.DynamicHeight = true
	input.SetHeight(1)
	input.CharLimit = 64 * 1024
	input.KeyMap.InsertNewline.SetKeys("shift+enter", "alt+enter", "ctrl+j")
	input.Focus()
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()

	m := &model{
		ctx: ctx, client: client, events: client.Stream(streamCtx), detail: detail,
		initialPage: page,
		approvals:   detail.PendingApprovals,
		deltas:      make(map[string]string), connected: false,
		status: detail.Turn, input: input,
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(mutedStyle)),
		history: inputHist,
	}
	m.applyDetail(detail)
	program := tea.NewProgram(m, tea.WithContext(ctx))
	clearTerminal(os.Stdout)
	_, err = program.Run()
	if err == tea.ErrInterrupted {
		return nil
	}
	return err
}

func (m *model) Init() tea.Cmd {
	commands := []tea.Cmd{m.input.Focus(), waitForStream(m.events)}
	if m.status == "running" { // resumed mid-turn — animate immediately
		commands = append(commands, m.spin.Tick)
	}
	return tea.Batch(commands...)
}

func waitForStream(events <-chan streamUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-events
		if !ok {
			return streamUpdate{Err: context.Canceled}
		}
		return update
	}
}

func (m *model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		page, err := m.client.TailMessages(m.ctx, historyWindowSize)
		return historyMsg{page: page, err: err}
	}
}

func (m *model) syncState() tea.Cmd {
	return func() tea.Msg {
		page, err := m.client.TailMessages(m.ctx, historyWindowSize)
		if err != nil {
			return syncMsg{err: err}
		}
		detail, err := m.client.Detail(m.ctx)
		return syncMsg{detail: detail, page: page, err: err}
	}
}

func action(name string, call func() error) tea.Cmd {
	return func() tea.Msg { return actionMsg{action: name, err: call()} }
}

// clearTerminal writes the erase-screen/scrollback/cursor-home sequence, best-effort.
func clearTerminal(w io.Writer) {
	_, _ = io.WriteString(w, clearScreenSeq)
}
