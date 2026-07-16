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

// TestModelItemsEffortLevels: the effort level is synthesized from the
// row's supported_efforts — preset rows for the same mapped_model collapse
// into it (matching efforts select the preset row, others select the newest
// row with an override), single rows get the full level too, and rows
// without capability data stay plain leaves.
func TestModelItemsEffortLevels(t *testing.T) {
	efforts := []string{"low", "medium", "high", "xhigh"}
	items := modelItems("api", []types.ConsoleModelOption{
		{ID: "gpt54_high", DisplayName: "GPT-5.4 (High)", MappedModel: "gpt-5.4", ReasoningEffort: "high", SupportedEfforts: efforts},
		{ID: "gpt54_low", DisplayName: "GPT-5.4 (Low)", MappedModel: "gpt-5.4", ReasoningEffort: "low", SupportedEfforts: efforts},
		{ID: "sonnet", DisplayName: "Sonnet", MappedModel: "claude-sonnet-5",
			SupportedEfforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{ID: "legacy", DisplayName: "Legacy", MappedModel: "old-model"},
	})
	if len(items) != 3 {
		t.Fatalf("item count = %d, want gpt-5.4 group + sonnet group + legacy leaf", len(items))
	}

	gpt := items[0].(selectionItem)
	if gpt.title != "GPT-5.4" || len(gpt.children) != 5 {
		t.Fatalf("gpt group = %+v, want Default + 4 efforts", gpt)
	}
	if def := gpt.children[0].(selectionItem); def.title != "Default" || def.selection != (Selection{Engine: "api", Model: "gpt54_high"}) {
		t.Fatalf("default child = %+v, want newest row without override", def)
	}
	// "low" matches the gpt54_low preset row → selected directly, no override.
	if low := gpt.children[1].(selectionItem); low.title != "Low" || low.selection != (Selection{Engine: "api", Model: "gpt54_low"}) {
		t.Fatalf("low child = %+v, want preset row gpt54_low", low)
	}
	// "xhigh" has no preset row → newest row + create-time override.
	if xh := gpt.children[4].(selectionItem); xh.title != "Xhigh" || xh.selection != (Selection{Engine: "api", Model: "gpt54_high", Effort: "xhigh"}) {
		t.Fatalf("xhigh child = %+v, want gpt54_high with override", xh)
	}

	sonnet := items[1].(selectionItem)
	if sonnet.title != "Sonnet" || len(sonnet.children) != 6 {
		t.Fatalf("sonnet group = %+v, want Default + 5 efforts", sonnet)
	}
	if mx := sonnet.children[5].(selectionItem); mx.selection != (Selection{Engine: "api", Model: "sonnet", Effort: "max"}) {
		t.Fatalf("sonnet max child = %+v", mx)
	}

	legacy := items[2].(selectionItem)
	if legacy.title != "Legacy" || len(legacy.children) != 0 || legacy.selection != (Selection{Engine: "api", Model: "legacy"}) {
		t.Fatalf("legacy leaf = %+v", legacy)
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
