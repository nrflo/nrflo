package consoleui

import (
	"testing"

	"charm.land/bubbles/v2/list"

	"be/internal/types"
)

func newSelectionModelForTest(items []list.Item) *selectionModel {
	// deleteArmed must start at the -1 sentinel (no delete pending); the zero
	// value 0 is itself a valid row index, and push/pop/enter unconditionally
	// call cancelDelete(), which would otherwise wipe row 0's item.
	model := &selectionModel{list: list.New(items, list.NewDefaultDelegate(), 80, 24), deleteArmed: -1}
	model.list.Title = selectRootTitle
	return model
}

func TestSelectionItemsGroupBrandModelModeEffort(t *testing.T) {
	contextLeft := 71
	items := selectionItems(Catalog{
		Sessions: []types.ConsoleSessionOption{{SessionID: "session-1234567890", Engine: "codex", ContextLeft: &contextLeft}},
		Engines: []types.ConsoleEngineOption{
			{ID: "claude", DisplayName: "Claude", Kind: "cli", Brand: "claude", Enabled: true,
				Models: []types.ConsoleModelOption{{
					ID: "sonnet-5", DisplayName: "Sonnet 5", Brand: "claude", MappedModel: "claude-sonnet-5",
					SupportedEfforts: []string{"low", "high"},
				}}},
			{ID: "codex", DisplayName: "Codex", Kind: "cli", Brand: "gpt", Enabled: false,
				Models: []types.ConsoleModelOption{{ID: "gpt-5.4", DisplayName: "GPT-5.4", Brand: "gpt"}}},
			{ID: "api", DisplayName: "Direct API", Kind: "api", Enabled: true, RequiresModel: true,
				Models: []types.ConsoleModelOption{
					{ID: "gpt-5.4", DisplayName: "GPT-5.4", Brand: "gpt", Provider: "openai", MappedModel: "gpt-5.4", ReasoningEffort: "medium", SupportedEfforts: []string{"low", "medium", "high"}},
					{ID: "sonnet-5", DisplayName: "Sonnet 5", Brand: "claude", Provider: "anthropic", MappedModel: "claude-sonnet-5", SupportedEfforts: []string{"low", "high"}},
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
	if claude.title != "Claude" || len(claude.children) != 1 {
		t.Fatalf("claude brand = %+v", claude)
	}
	sonnet := claude.children[0].(selectionItem)
	if sonnet.title != "Sonnet 5" || len(sonnet.children) != 2 {
		t.Fatalf("sonnet model = %+v, want CLI + Direct API", sonnet)
	}
	cli := sonnet.children[0].(selectionItem)
	if cli.title != "CLI" || len(cli.children) != 3 {
		t.Fatalf("CLI mode = %+v, want Default + 2 efforts", cli)
	}
	if def := cli.children[0].(selectionItem); def.selection != (Selection{Engine: "claude", Model: "sonnet-5"}) {
		t.Fatalf("CLI default = %+v", def)
	}
	api := sonnet.children[1].(selectionItem)
	if api.title != "Direct API" || api.children[2].(selectionItem).selection != (Selection{Engine: "api", Model: "sonnet-5", Effort: "high"}) {
		t.Fatalf("API mode = %+v", api)
	}

	gpt := items[2].(selectionItem)
	if gpt.title != "GPT" || len(gpt.children) != 1 {
		t.Fatalf("gpt brand = %+v", gpt)
	}
	gpt54 := gpt.children[0].(selectionItem)
	if len(gpt54.children) != 1 || gpt54.children[0].(selectionItem).title != "Direct API" {
		t.Fatalf("gpt model = %+v, disabled CLI mode must be absent", gpt54)
	}
	def := gpt54.children[0].(selectionItem).children[0].(selectionItem)
	if def.title != "Default" || def.detail != "Medium" || def.selection != (Selection{Engine: "api", Model: "gpt-5.4"}) {
		t.Fatalf("gpt default effort = %+v", def)
	}
}

func TestEffortItemsAlwaysStartWithDefault(t *testing.T) {
	items := effortItems("codex", types.ConsoleModelOption{
		ID: "gpt-5.6-sol", MappedModel: "gpt-5.6-sol", ReasoningEffort: "medium",
		SupportedEfforts: []string{"low", "medium", "ultra"},
	})
	if len(items) != 4 {
		t.Fatalf("effort count = %d, want Default + 3", len(items))
	}
	if got := items[0].(selectionItem); got.title != "Default" || got.detail != "Medium" || got.selection.Effort != "" {
		t.Fatalf("default item = %+v", got)
	}
	if got := items[3].(selectionItem); got.title != "Ultra" || got.selection != (Selection{Engine: "codex", Model: "gpt-5.6-sol", Effort: "ultra"}) {
		t.Fatalf("ultra item = %+v", got)
	}

	noEfforts := effortItems("api", types.ConsoleModelOption{ID: "plain"})
	if len(noEfforts) != 1 || noEfforts[0].(selectionItem).title != "Default" {
		t.Fatalf("no-effort model items = %+v", noEfforts)
	}
}

func TestSelectionModelPushPop(t *testing.T) {
	items := selectionItems(Catalog{Engines: []types.ConsoleEngineOption{
		{ID: "claude", DisplayName: "Claude", Kind: "cli", Brand: "claude", Enabled: true,
			Models: []types.ConsoleModelOption{{ID: "sonnet-5", DisplayName: "Sonnet 5", Brand: "claude"}}},
	}})
	model := newSelectionModelForTest(items)

	brand := model.list.SelectedItem().(selectionItem)
	model.push(brand)
	if model.list.Title != selectRootTitle+" · Claude" {
		t.Fatalf("title after push = %q", model.list.Title)
	}
	if got := model.list.SelectedItem().(selectionItem).title; got != "Sonnet 5" {
		t.Fatalf("selected after push = %q, want Sonnet 5", got)
	}
	if _, ok := model.pop(); !ok {
		t.Fatal("pop at depth 1 should succeed")
	}
	if got := model.list.SelectedItem().(selectionItem).title; got != "Claude" {
		t.Fatalf("selected after pop = %q, want Claude", got)
	}
	if _, ok := model.pop(); ok {
		t.Fatal("pop at root should report false")
	}
}
