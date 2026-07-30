package consoleui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"

	"be/internal/types"
)

// brandDisplay names the server's model-family grouping keys; unknown brands
// fall back to the key itself.
var brandDisplay = map[string]string{"claude": "Claude", "gpt": "GPT"}

type modelMode struct {
	engine types.ConsoleEngineOption
	model  types.ConsoleModelOption
}

type modelGroup struct {
	model types.ConsoleModelOption
	modes []modelMode
}

// selectionItems builds resume leaves, then profile branches, then brand →
// model → mode → effort branches. Disabled engines are absent, so every
// displayed mode is both supported by the model and currently available on
// the server.
func selectionItems(catalog Catalog) []list.Item {
	items := make([]list.Item, 0, len(catalog.Sessions)+len(catalog.Profiles)+4)
	for _, session := range catalog.Sessions {
		model := session.Model
		if model == "" {
			model = "default"
		}
		detail := fmt.Sprintf("%s / %s", session.Engine, model)
		if session.Profile != "" {
			detail += " · " + session.Profile
		}
		if session.ContextLeft != nil {
			detail += fmt.Sprintf(" · context %d%%", *session.ContextLeft)
		}
		items = append(items, selectionItem{
			selection: Selection{ResumeID: session.SessionID},
			title:     "Resume " + shortSessionID(session.SessionID), detail: detail,
		})
	}
	items = append(items, profileItems(catalog)...)
	return append(items, brandItems(catalog)...)
}

// profileItems builds one branch per built-in console profile: a "Profile
// defaults" leaf (the profile's own engine/model/effort, mirroring the web
// form's prefill) followed by the brand tree with the profile stamped on
// every leaf, so any available model/effort can still be picked under it.
func profileItems(catalog Catalog) []list.Item {
	items := make([]list.Item, 0, len(catalog.Profiles))
	for _, p := range catalog.Profiles {
		defaults := fmt.Sprintf("%s / %s", p.DefaultEngine, p.DefaultModelID)
		if p.DefaultEffort != "" {
			defaults += " · " + p.DefaultEffort
		}
		children := make([]list.Item, 0, 1)
		children = append(children, selectionItem{
			selection: Selection{Engine: p.DefaultEngine, Model: p.DefaultModelID, Effort: p.DefaultEffort, Profile: p.Name},
			title:     "Profile defaults", detail: defaults,
		})
		children = append(children, stampProfile(brandItems(catalog), p.Name)...)
		items = append(items, selectionItem{
			title: p.DisplayName, detail: p.Description,
			children: children, crumb: p.DisplayName,
		})
	}
	return items
}

// stampProfile tags every leaf Selection in a picker subtree with the profile
// name, rebuilding branch child slices so the profile-scoped tree never
// aliases the profile-less root tree.
func stampProfile(items []list.Item, profile string) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, raw := range items {
		item := raw.(selectionItem)
		if len(item.children) > 0 {
			item.children = stampProfile(item.children, profile)
		} else {
			item.selection.Profile = profile
		}
		out = append(out, item)
	}
	return out
}

func brandItems(catalog Catalog) []list.Item {
	brandOrder, grouped := groupModels(catalog)
	items := make([]list.Item, 0, len(brandOrder))
	for _, brand := range brandOrder {
		groups := grouped[brand]
		children := make([]list.Item, 0, len(groups))
		for _, group := range groups {
			children = append(children, modelItem(group))
		}
		display := brandDisplay[brand]
		if display == "" {
			display = brand
		}
		items = append(items, selectionItem{
			title: display, detail: fmt.Sprintf("%d models", len(groups)),
			children: children, crumb: display,
		})
	}
	return items
}

// groupModels combines the same canonical model exposed by multiple engine
// entries while preserving first-seen brand and model order from the catalog.
func groupModels(catalog Catalog) ([]string, map[string][]modelGroup) {
	var brandOrder []string
	grouped := map[string][]modelGroup{}
	indices := map[string]map[string]int{}
	for _, engine := range catalog.Engines {
		if !engine.Enabled {
			continue
		}
		for _, model := range engine.Models {
			brand := model.Brand
			if brand == "" {
				brand = engine.Brand
			}
			if _, ok := indices[brand]; !ok {
				brandOrder = append(brandOrder, brand)
				indices[brand] = map[string]int{}
			}
			idx, ok := indices[brand][model.ID]
			if !ok {
				idx = len(grouped[brand])
				indices[brand][model.ID] = idx
				grouped[brand] = append(grouped[brand], modelGroup{model: model})
			}
			grouped[brand][idx].modes = append(grouped[brand][idx].modes, modelMode{engine: engine, model: model})
		}
	}
	return brandOrder, grouped
}

func modelItem(group modelGroup) selectionItem {
	children := make([]list.Item, 0, len(group.modes))
	modeNames := make([]string, 0, len(group.modes))
	for _, mode := range group.modes {
		item := modeItem(mode)
		children = append(children, item)
		modeNames = append(modeNames, item.title)
	}
	return selectionItem{
		title: group.model.DisplayName, detail: strings.Join(modeNames, " · "),
		children: children, crumb: group.model.DisplayName,
	}
}

func modeItem(mode modelMode) selectionItem {
	title, crumb := "CLI", "CLI"
	if mode.engine.Kind == "api" {
		title, crumb = "Direct API", "API"
	}
	children := effortItems(mode.engine.ID, mode.model)
	detail := mode.model.MappedModel
	if mode.engine.Kind == "cli" {
		detail = strings.TrimSpace(mode.engine.DisplayName + " · " + detail)
	}
	return selectionItem{title: title, detail: detail, children: children, crumb: crumb}
}

func effortItems(engineID string, model types.ConsoleModelOption) []list.Item {
	defaultDetail := "provider default"
	if model.ReasoningEffort != "" {
		defaultDetail = effortTitle(model.ReasoningEffort)
	}
	items := make([]list.Item, 0, len(model.SupportedEfforts)+1)
	items = append(items, selectionItem{
		selection: Selection{Engine: engineID, Model: model.ID},
		title:     "Default", detail: defaultDetail,
	})
	for _, effort := range model.SupportedEfforts {
		items = append(items, selectionItem{
			selection: Selection{Engine: engineID, Model: model.ID, Effort: effort},
			title:     effortTitle(effort), detail: model.MappedModel,
		})
	}
	return items
}

func effortTitle(effort string) string {
	if effort == "" {
		return ""
	}
	return strings.ToUpper(effort[:1]) + effort[1:]
}

func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
