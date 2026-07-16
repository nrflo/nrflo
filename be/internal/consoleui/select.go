package consoleui

import (
	"context"
	"fmt"

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
}

func Select(ctx context.Context, catalog Catalog) (Selection, error) {
	items := selectionItems(catalog)
	if len(items) == 0 {
		return Selection{}, fmt.Errorf("server reported no available console engines or sessions")
	}
	delegate := list.NewDefaultDelegate()
	model := &selectionModel{list: list.New(items, delegate, 80, 24)}
	model.list.Title = selectRootTitle
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
	m.stack = append(m.stack, levelFrame{items: m.list.Items(), title: m.list.Title, index: m.list.Index()})
	m.list.ResetFilter()
	cmd := m.list.SetItems(item.children)
	m.list.Select(0)
	m.list.Title = m.list.Title + " · " + item.crumb
	return cmd
}

// pop restores the parent level; returns false at the root.
func (m *selectionModel) pop() (tea.Cmd, bool) {
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
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "esc":
			if m.list.FilterState() != list.Filtering {
				if cmd, ok := m.pop(); ok {
					return m, cmd
				}
				m.cancelled = true
				return m, tea.Quit
			}
		case "enter":
			if m.list.FilterState() != list.Filtering {
				if item, ok := m.list.SelectedItem().(selectionItem); ok {
					if len(item.children) > 0 {
						return m, m.push(item)
					}
					m.selected = item.selection
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(message)
	return m, cmd
}

func (m *selectionModel) View() tea.View {
	view := tea.NewView(m.list.View())
	view.AltScreen = true
	view.WindowTitle = "nrflo console"
	return view
}

var _ list.DefaultItem = selectionItem{}
