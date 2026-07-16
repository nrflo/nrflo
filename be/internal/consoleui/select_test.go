package consoleui

import (
	"testing"

	"charm.land/bubbles/v2/list"

	"be/internal/types"
)

func newSelectionModelForTest(items []list.Item) *selectionModel {
	model := &selectionModel{list: list.New(items, list.NewDefaultDelegate(), 80, 24)}
	model.list.Title = selectRootTitle
	return model
}

// TestSelectionItemsGroupBrandThenType asserts the drill-down tree: resume
// rows at the root, then one branch per brand → type (CLI/Direct API) →
// models in the server's (newest-first) order. Disabled engines vanish.
func TestSelectionItemsGroupBrandThenType(t *testing.T) {
	contextLeft := 71
	items := selectionItems(Catalog{
		Sessions: []types.ConsoleSessionOption{{SessionID: "session-1234567890", Engine: "codex", ContextLeft: &contextLeft}},
		Engines: []types.ConsoleEngineOption{
			{ID: "claude", DisplayName: "Claude", Kind: "cli", Brand: "claude", Enabled: true,
				Models: []types.ConsoleModelOption{{ID: "sonnet", DisplayName: "Sonnet", Brand: "claude"}}},
			{ID: "codex", DisplayName: "Codex", Kind: "cli", Brand: "gpt", Enabled: false, DisabledReason: "not installed"},
			{ID: "api", DisplayName: "Direct API", Kind: "api", Enabled: true, RequiresModel: true,
				Models: []types.ConsoleModelOption{
					{ID: "gpt54", DisplayName: "GPT 5.4", Brand: "gpt", Provider: "openai"},
					{ID: "haiku", DisplayName: "Haiku", Brand: "claude", Provider: "anthropic"},
				}},
		},
	})
	if len(items) != 3 {
		t.Fatalf("root item count = %d, want resume + Claude + GPT", len(items))
	}
	resume := items[0].(selectionItem)
	if resume.selection.ResumeID != "session-1234567890" || len(resume.children) != 0 {
		t.Fatalf("resume item = %+v", resume)
	}

	claude := items[1].(selectionItem)
	if claude.title != "Claude" || len(claude.children) != 2 {
		t.Fatalf("claude brand = %+v, want CLI + Direct API children", claude)
	}
	cli := claude.children[0].(selectionItem)
	if cli.title != "CLI" || len(cli.children) != 2 {
		t.Fatalf("claude CLI = %+v, want default + sonnet", cli)
	}
	if def := cli.children[0].(selectionItem).selection; def != (Selection{Engine: "claude"}) {
		t.Fatalf("default selection = %+v", def)
	}
	if model := cli.children[1].(selectionItem).selection; model != (Selection{Engine: "claude", Model: "sonnet"}) {
		t.Fatalf("cli model selection = %+v", model)
	}
	api := claude.children[1].(selectionItem)
	if api.title != "Direct API" || len(api.children) != 1 {
		t.Fatalf("claude Direct API = %+v, want haiku only", api)
	}
	if model := api.children[0].(selectionItem).selection; model != (Selection{Engine: "api", Model: "haiku"}) {
		t.Fatalf("api model selection = %+v", model)
	}

	gpt := items[2].(selectionItem)
	if gpt.title != "GPT" || len(gpt.children) != 1 {
		t.Fatalf("gpt brand = %+v, want Direct API only (codex disabled)", gpt)
	}
	if model := gpt.children[0].(selectionItem).children[0].(selectionItem).selection; model != (Selection{Engine: "api", Model: "gpt54"}) {
		t.Fatalf("gpt api model selection = %+v", model)
	}
}

// TestModelItemsGroupEfforts: registry rows sharing a mapped_model collapse
// into one branch with an effort level (high → medium → low), while
// single-variant models stay leaves.
func TestModelItemsGroupEfforts(t *testing.T) {
	items := modelItems("api", []types.ConsoleModelOption{
		{ID: "gpt54_low", DisplayName: "GPT-5.4 (Low)", MappedModel: "gpt-5.4", ReasoningEffort: "low"},
		{ID: "sonnet", DisplayName: "Sonnet", MappedModel: "claude-sonnet-5"},
		{ID: "gpt54_high", DisplayName: "GPT-5.4 (High)", MappedModel: "gpt-5.4", ReasoningEffort: "high"},
	})
	if len(items) != 2 {
		t.Fatalf("item count = %d, want grouped gpt-5.4 + sonnet leaf", len(items))
	}
	group := items[0].(selectionItem)
	if group.title != "GPT-5.4" || len(group.children) != 2 {
		t.Fatalf("group = %+v, want GPT-5.4 with 2 efforts", group)
	}
	high := group.children[0].(selectionItem)
	low := group.children[1].(selectionItem)
	if high.title != "High" || high.selection != (Selection{Engine: "api", Model: "gpt54_high"}) {
		t.Fatalf("first effort = %+v, want High/gpt54_high", high)
	}
	if low.title != "Low" || low.selection != (Selection{Engine: "api", Model: "gpt54_low"}) {
		t.Fatalf("second effort = %+v, want Low/gpt54_low", low)
	}
	leaf := items[1].(selectionItem)
	if leaf.title != "Sonnet" || len(leaf.children) != 0 || leaf.selection != (Selection{Engine: "api", Model: "sonnet"}) {
		t.Fatalf("leaf = %+v", leaf)
	}
}

// TestSelectionModelPushPop drives push/pop directly: descending into a
// branch swaps the list to its children and extends the title; pop restores
// the parent level and cursor, and returns false at the root.
func TestSelectionModelPushPop(t *testing.T) {
	items := selectionItems(Catalog{Engines: []types.ConsoleEngineOption{
		{ID: "claude", DisplayName: "Claude", Kind: "cli", Brand: "claude", Enabled: true,
			Models: []types.ConsoleModelOption{{ID: "sonnet", DisplayName: "Sonnet", Brand: "claude"}}},
	}})
	model := newSelectionModelForTest(items)

	brand := model.list.SelectedItem().(selectionItem)
	model.push(brand)
	if model.list.Title != selectRootTitle+" · Claude" {
		t.Fatalf("title after push = %q", model.list.Title)
	}
	if got := model.list.SelectedItem().(selectionItem).title; got != "CLI" {
		t.Fatalf("selected after push = %q, want CLI", got)
	}
	if _, ok := model.pop(); !ok {
		t.Fatal("pop at depth 1 should succeed")
	}
	if model.list.Title != selectRootTitle {
		t.Fatalf("title after pop = %q", model.list.Title)
	}
	if got := model.list.SelectedItem().(selectionItem).title; got != "Claude" {
		t.Fatalf("selected after pop = %q, want Claude", got)
	}
	if _, ok := model.pop(); ok {
		t.Fatal("pop at root should report false")
	}
}
