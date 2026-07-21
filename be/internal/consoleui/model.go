package consoleui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
)

type model struct {
	ctx           context.Context
	client        *Client
	events        <-chan streamUpdate
	detail        ChatDetail
	messages      []Message
	historyOffset int
	historyTotal  int
	deltas        map[string]string
	deltaOrder    []string
	thinking      string
	thinkingID    string

	approvals       []Approval
	connected       bool
	status          string
	lastErr         string
	width           int
	height          int
	ready           bool
	historyDirty    bool
	renderedHistory string
	renderedWidth   int
	renderCache     map[string]string
	copyMode        bool
	searchMode      bool
	searchStatus    string
	notice          string

	input    textarea.Model
	search   textinput.Model
	viewport viewport.Model
	spin     spinner.Model

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
	page    MessagePage
	prepend bool
	offset  int
	err     error
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
	page, err := client.TailMessages(loadCtx, historyPageSize)
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
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "search transcript"
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()

	m := &model{
		ctx: ctx, client: client, events: client.Stream(streamCtx), detail: detail,
		messages: page.Messages, historyOffset: max(0, page.Total-len(page.Messages)), historyTotal: page.Total,
		approvals: detail.PendingApprovals,
		deltas:    make(map[string]string), connected: false,
		status: detail.Turn, input: input, search: search, viewport: viewport.New(), historyDirty: true,
		renderCache: make(map[string]string),
		spin:        spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(mutedStyle)),
	}
	m.applyDetail(detail)
	program := tea.NewProgram(m, tea.WithContext(ctx))
	_, err = program.Run()
	if isatty.IsTerminal(os.Stdout.Fd()) {
		io.WriteString(os.Stdout, altScrollDisable) //nolint:errcheck // best-effort terminal mode reset
	}
	if err == tea.ErrInterrupted {
		return nil
	}
	return err
}

func (m *model) Init() tea.Cmd {
	commands := []tea.Cmd{m.input.Focus(), waitForStream(m.events), tea.Raw(altScrollEnable)}
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

func (m *model) loadOlder() tea.Cmd {
	offset := max(0, m.historyOffset-historyPageSize)
	limit := m.historyOffset - offset
	return func() tea.Msg {
		page, err := m.client.MessagesPage(m.ctx, limit, offset)
		return historyMsg{page: page, prepend: true, offset: offset, err: err}
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
