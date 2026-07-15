package consoleui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type selectionItem struct {
	selection Selection
	title     string
	detail    string
}

func (i selectionItem) Title() string       { return i.title }
func (i selectionItem) Description() string { return i.detail }
func (i selectionItem) FilterValue() string { return i.title + " " + i.detail }

type selectionModel struct {
	list      list.Model
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
	model.list.Title = "nrflo console · resume or start"
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

func selectionItems(catalog Catalog) []list.Item {
	items := make([]list.Item, 0, len(catalog.Sessions)+16)
	for _, session := range catalog.Sessions {
		model := session.Model
		if model == "" {
			model = "default"
		}
		detail := fmt.Sprintf("%s / %s", session.Engine, model)
		if session.ContextLeft != nil {
			detail += fmt.Sprintf(" · context %d%%", *session.ContextLeft)
		}
		items = append(items, selectionItem{
			selection: Selection{ResumeID: session.SessionID},
			title:     "Resume " + shortSessionID(session.SessionID), detail: detail,
		})
	}
	for _, engine := range catalog.Engines {
		if !engine.Enabled {
			continue
		}
		if !engine.RequiresModel {
			items = append(items, selectionItem{
				selection: Selection{Engine: engine.ID},
				title:     "New " + engine.DisplayName, detail: "provider default model",
			})
		}
		for _, model := range engine.Models {
			detail := model.Provider
			if detail == "" {
				detail = model.MappedModel
			}
			if model.ReasoningEffort != "" {
				detail = strings.TrimSpace(detail + " · " + model.ReasoningEffort)
			}
			items = append(items, selectionItem{
				selection: Selection{Engine: engine.ID, Model: model.ID},
				title:     fmt.Sprintf("New %s · %s", engine.DisplayName, model.DisplayName),
				detail:    detail,
			})
		}
	}
	return items
}

func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
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
				m.cancelled = true
				return m, tea.Quit
			}
		case "enter":
			if m.list.FilterState() != list.Filtering {
				if item, ok := m.list.SelectedItem().(selectionItem); ok {
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
