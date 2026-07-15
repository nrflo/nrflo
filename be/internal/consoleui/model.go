package consoleui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type model struct {
	ctx        context.Context
	client     *Client
	events     <-chan streamUpdate
	detail     ChatDetail
	messages   []Message
	deltas     map[string]string
	deltaOrder []string
	thinking   string
	thinkingID string

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

	input    textarea.Model
	viewport viewport.Model
}

type historyMsg struct {
	messages []Message
	err      error
}

type actionMsg struct {
	action string
	err    error
}

func Run(ctx context.Context, cfg Config) error {
	client := NewClient(cfg)
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	detail, err := client.Detail(loadCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("load console chat: %w", err)
	}
	messages, err := client.Messages(loadCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("load console history: %w", err)
	}

	input := textarea.New()
	input.Placeholder = "Ask nrflo…"
	input.ShowLineNumbers = false
	input.SetHeight(3)
	input.CharLimit = 64 * 1024
	input.KeyMap.InsertNewline.SetKeys("shift+enter", "alt+enter", "ctrl+j")
	input.Focus()
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()

	m := &model{
		ctx: ctx, client: client, events: client.Stream(streamCtx), detail: detail,
		messages: messages, approvals: detail.PendingApprovals,
		deltas: make(map[string]string), connected: false,
		status: detail.Turn, input: input, viewport: viewport.New(), historyDirty: true,
	}
	program := tea.NewProgram(m, tea.WithContext(ctx))
	_, err = program.Run()
	if err == tea.ErrInterrupted {
		return nil
	}
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), waitForStream(m.events))
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
		messages, err := m.client.Messages(m.ctx)
		return historyMsg{messages: messages, err: err}
	}
}

func action(name string, call func() error) tea.Cmd {
	return func() tea.Msg { return actionMsg{action: name, err: call()} }
}
