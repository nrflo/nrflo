package consoleui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"

	"be/internal/types"
)

// brandDisplay names the server's model-family grouping keys; unknown brands
// fall back to the key itself.
var brandDisplay = map[string]string{"claude": "Claude", "gpt": "GPT"}

// selectionItems builds the drill-down tree from the server catalog:
// resumable sessions first, then one branch per brand (Claude/GPT) →
// type (CLI/Direct API) → models, which the server sends newest-first.
func selectionItems(catalog Catalog) []list.Item {
	items := make([]list.Item, 0, len(catalog.Sessions)+4)
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
	return append(items, brandItems(catalog)...)
}

// brandItems groups the enabled engines by brand, in catalog order.
func brandItems(catalog Catalog) []list.Item {
	var order []string
	groups := map[string][]selectionItem{}
	add := func(brand string, item selectionItem) {
		if _, ok := groups[brand]; !ok {
			order = append(order, brand)
		}
		groups[brand] = append(groups[brand], item)
	}
	for _, engine := range catalog.Engines {
		if !engine.Enabled {
			continue
		}
		if engine.Kind == "api" {
			brands, byBrand := apiModelsByBrand(engine)
			for _, brand := range brands {
				add(brand, apiTypeItem(engine, byBrand[brand]))
			}
		} else {
			add(engine.Brand, cliTypeItem(engine))
		}
	}
	items := make([]list.Item, 0, len(order))
	for _, brand := range order {
		types := groups[brand]
		names := make([]string, len(types))
		for i, t := range types {
			names[i] = t.title
		}
		display := brandDisplay[brand]
		if display == "" {
			display = brand
		}
		items = append(items, selectionItem{
			title: display, detail: strings.Join(names, " · "),
			children: toListItems(types), crumb: display,
		})
	}
	return items
}

// apiModelsByBrand splits the mixed-family api engine's models by brand,
// preserving the server's newest-first order and first-seen brand order.
func apiModelsByBrand(engine types.ConsoleEngineOption) ([]string, map[string][]types.ConsoleModelOption) {
	var order []string
	byBrand := map[string][]types.ConsoleModelOption{}
	for _, model := range engine.Models {
		if _, ok := byBrand[model.Brand]; !ok {
			order = append(order, model.Brand)
		}
		byBrand[model.Brand] = append(byBrand[model.Brand], model)
	}
	return order, byBrand
}

func cliTypeItem(engine types.ConsoleEngineOption) selectionItem {
	children := make([]list.Item, 0, len(engine.Models)+1)
	if !engine.RequiresModel {
		children = append(children, selectionItem{
			selection: Selection{Engine: engine.ID},
			title:     "Default", detail: "provider default model",
		})
	}
	children = append(children, modelItems(engine.ID, engine.Models)...)
	return selectionItem{
		title: "CLI", detail: fmt.Sprintf("%s · %d models", engine.DisplayName, len(engine.Models)),
		children: children, crumb: "CLI",
	}
}

func apiTypeItem(engine types.ConsoleEngineOption, models []types.ConsoleModelOption) selectionItem {
	return selectionItem{
		title: "Direct API", detail: fmt.Sprintf("%d models", len(models)),
		children: modelItems(engine.ID, models), crumb: "API",
	}
}

// modelItems groups registry rows that map to the same underlying model
// (differing only by reasoning effort) into one branch with an effort
// level; single-variant models stay leaves. First-seen (newest-first)
// group order is preserved.
func modelItems(engineID string, models []types.ConsoleModelOption) []list.Item {
	var order []string
	groups := map[string][]types.ConsoleModelOption{}
	for _, model := range models {
		key := model.MappedModel
		if key == "" {
			key = model.ID
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], model)
	}
	items := make([]list.Item, 0, len(order))
	for _, key := range order {
		variants := groups[key]
		if len(variants) == 1 {
			items = append(items, modelItem(engineID, variants[0], variants[0].DisplayName))
			continue
		}
		sort.SliceStable(variants, func(i, j int) bool {
			return effortRank(variants[i].ReasoningEffort) < effortRank(variants[j].ReasoningEffort)
		})
		children := make([]list.Item, len(variants))
		for i, variant := range variants {
			children[i] = modelItem(engineID, variant, effortTitle(variant))
		}
		base := baseDisplayName(variants[0])
		items = append(items, selectionItem{
			title: base, detail: fmt.Sprintf("%s · %d efforts", key, len(variants)),
			children: children, crumb: base,
		})
	}
	return items
}

func modelItem(engineID string, model types.ConsoleModelOption, title string) selectionItem {
	detail := model.MappedModel
	if detail == "" {
		detail = model.Provider
	}
	if model.ReasoningEffort != "" {
		detail = strings.TrimSpace(detail + " · " + model.ReasoningEffort)
	}
	return selectionItem{
		selection: Selection{Engine: engineID, Model: model.ID},
		title:     title, detail: detail,
	}
}

func effortRank(effort string) int {
	switch effort {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	}
	return 3
}

// effortTitle labels an effort leaf ("High"); rows without an effort keep
// their display name.
func effortTitle(model types.ConsoleModelOption) string {
	if model.ReasoningEffort == "" {
		return model.DisplayName
	}
	return strings.ToUpper(model.ReasoningEffort[:1]) + model.ReasoningEffort[1:]
}

// baseDisplayName strips a trailing "(<effort>)" qualifier from a variant's
// display name ("GPT-5.4 (High)" → "GPT-5.4"); other parentheticals stay.
func baseDisplayName(model types.ConsoleModelOption) string {
	name := strings.TrimSpace(model.DisplayName)
	if model.ReasoningEffort == "" {
		return name
	}
	suffix := "(" + strings.ToLower(model.ReasoningEffort) + ")"
	if lower := strings.ToLower(name); strings.HasSuffix(lower, suffix) {
		return strings.TrimSpace(name[:len(name)-len(suffix)])
	}
	return name
}

func toListItems(items []selectionItem) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
