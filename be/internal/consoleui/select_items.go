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

// modelItems groups registry rows by underlying model. A group whose rows
// carry supported_efforts becomes one branch with an effort level (its
// per-effort preset rows collapse into it); rows without capability data
// stay plain leaves. First-seen (newest-first) group order is preserved.
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
		if len(variants[0].SupportedEfforts) == 0 {
			for _, variant := range variants {
				items = append(items, modelItem(engineID, variant, variant.DisplayName, ""))
			}
			continue
		}
		items = append(items, effortGroupItem(engineID, key, variants))
	}
	return items
}

// effortGroupItem builds a model branch whose children are the effort
// levels: "Default" (the newest row, no override) plus one child per
// supported effort. An effort matching an existing preset row selects that
// row directly; anything else selects the newest row with a create-time
// override.
func effortGroupItem(engineID, mapped string, variants []types.ConsoleModelOption) selectionItem {
	rep := variants[0]
	defaultDetail := "provider default"
	if rep.ReasoningEffort != "" {
		defaultDetail = rep.ReasoningEffort
	}
	children := make([]list.Item, 0, len(rep.SupportedEfforts)+1)
	children = append(children, selectionItem{
		selection: Selection{Engine: engineID, Model: rep.ID},
		title:     "Default", detail: defaultDetail,
	})
	for _, effort := range rep.SupportedEfforts {
		row, override := rep, effort
		for _, variant := range variants {
			if variant.ReasoningEffort == effort {
				row, override = variant, ""
				break
			}
		}
		children = append(children, selectionItem{
			selection: Selection{Engine: engineID, Model: row.ID, Effort: override},
			title:     effortTitle(effort), detail: mapped,
		})
	}
	base := baseDisplayName(rep)
	return selectionItem{
		title: base, detail: fmt.Sprintf("%s · %d efforts", mapped, len(rep.SupportedEfforts)),
		children: children, crumb: base,
	}
}

func modelItem(engineID string, model types.ConsoleModelOption, title, effort string) selectionItem {
	detail := model.MappedModel
	if detail == "" {
		detail = model.Provider
	}
	if model.ReasoningEffort != "" {
		detail = strings.TrimSpace(detail + " · " + model.ReasoningEffort)
	}
	return selectionItem{
		selection: Selection{Engine: engineID, Model: model.ID, Effort: effort},
		title:     title, detail: detail,
	}
}

// effortTitle labels an effort child ("High").
func effortTitle(effort string) string {
	return strings.ToUpper(effort[:1]) + effort[1:]
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
