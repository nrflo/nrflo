package consoleui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const selectRootTitle = "nrflo console · resume or start"

// selectionItem is one row of the drill-down picker. Leaves carry a
// Selection; branches carry children the picker descends into on enter
// (esc walks back up).
type selectionItem struct {
	selection Selection
	title     string
	detail    string
	children  []list.Item
	crumb     string // breadcrumb segment shown in the list title while inside
}

func (i selectionItem) Title() string       { return i.title }
func (i selectionItem) Description() string { return i.detail }
func (i selectionItem) FilterValue() string { return i.title + " " + i.detail }

// levelFrame snapshots one picker level so esc can restore it exactly.
type levelFrame struct {
	items []list.Item
	title string
	index int
}

type selectionModel struct {
	list      list.Model
	stack     []levelFrame
	selected  Selection
	cancelled bool

	ctx      context.Context
	deleteFn func(context.Context, string) error

	// deleteArmed is the index of a resume row pending a second `d` press
	// to confirm deletion, or -1 when nothing is armed.
	deleteArmed    int
	deleteOriginal selectionItem
	// deletePending is true while a confirmed delete's deleteFn call is in
	// flight, blocking re-arming/re-confirming until deleteResultMsg lands.
	deletePending bool
}

// Select runs the drill-down picker. deleteFn deletes (closes) a live chat by
// session id; the picker calls it from a two-press `d` confirm on a resume row.
func Select(ctx context.Context, catalog Catalog, deleteFn func(context.Context, string) error) (Selection, error) {
	items := selectionItems(catalog)
	if len(items) == 0 {
		return Selection{}, fmt.Errorf("server reported no available console engines or sessions")
	}
	delegate := compactDelegate{}
	model := &selectionModel{list: list.New(items, delegate, 80, 24), ctx: ctx, deleteFn: deleteFn, deleteArmed: -1}
	model.list.Title = selectRootTitle
	model.list.AdditionalShortHelpKeys = func() []key.Binding {
		if !isResumeRow(model.list.SelectedItem()) {
			return nil
		}
		if model.deleteArmed >= 0 {
			return []key.Binding{deleteConfirmKey}
		}
		return []key.Binding{deleteKey}
	}
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return Selection{}, err
	}
	final := result.(*selectionModel)
	if final.cancelled || (final.selected == Selection{}) {
		return Selection{}, context.Canceled
	}
	return final.selected, nil
}

// push descends into a branch item, snapshotting the current level.
func (m *selectionModel) push(item selectionItem) tea.Cmd {
	m.cancelDelete()
	m.stack = append(m.stack, levelFrame{items: m.list.Items(), title: m.list.Title, index: m.list.Index()})
	m.list.ResetFilter()
	cmd := m.list.SetItems(item.children)
	m.list.Select(0)
	m.list.Title = m.list.Title + " · " + item.crumb
	return cmd
}

// pop restores the parent level; returns false at the root.
func (m *selectionModel) pop() (tea.Cmd, bool) {
	m.cancelDelete()
	if len(m.stack) == 0 {
		return nil, false
	}
	frame := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.list.ResetFilter()
	cmd := m.list.SetItems(frame.items)
	m.list.Select(frame.index)
	m.list.Title = frame.title
	return cmd, true
}

func (m *selectionModel) Init() tea.Cmd { return nil }

func (m *selectionModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	case deleteResultMsg:
		return m, m.applyDeleteResult(msg)
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "esc":
			if m.list.FilterState() != list.Filtering {
				if m.deleteArmed >= 0 {
					return m, m.cancelDelete()
				}
				if cmd, ok := m.pop(); ok {
					return m, cmd
				}
				m.cancelled = true
				return m, tea.Quit
			}
		case "enter":
			if m.list.FilterState() != list.Filtering {
				if item, ok := m.list.SelectedItem().(selectionItem); ok {
					m.cancelDelete()
					if len(item.children) > 0 {
						return m, m.push(item)
					}
					m.selected = item.selection
					return m, tea.Quit
				}
			}
		case "d":
			if m.list.FilterState() != list.Filtering && !m.deletePending && isResumeRow(m.list.SelectedItem()) {
				if m.deleteArmed == m.list.Index() {
					return m, m.confirmDelete()
				}
				return m, m.armDelete()
			}
		default:
			if m.deleteArmed >= 0 {
				return m, tea.Batch(m.cancelDelete(), m.forwardListUpdate(message))
			}
		}
	}
	return m, m.forwardListUpdate(message)
}

// forwardListUpdate is the shared list.Update trailer every branch of Update
// ends with, factored out so the "d" disarm-on-navigation branch can chain it.
func (m *selectionModel) forwardListUpdate(message tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(message)
	return cmd
}

func (m *selectionModel) View() tea.View {
	view := tea.NewView(m.list.View())
	view.AltScreen = true
	view.WindowTitle = "nrflo console"
	return view
}

var _ list.DefaultItem = selectionItem{}
