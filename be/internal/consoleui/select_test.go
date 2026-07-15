package consoleui

import (
	"testing"

	"be/internal/types"
)

func TestSelectionItemsUseServerCatalog(t *testing.T) {
	contextLeft := 71
	items := selectionItems(Catalog{
		Sessions: []types.ConsoleSessionOption{{SessionID: "session-1234567890", Engine: "codex", ContextLeft: &contextLeft}},
		Engines: []types.ConsoleEngineOption{
			{ID: "claude", DisplayName: "Claude", Enabled: true, Models: []types.ConsoleModelOption{{ID: "sonnet", DisplayName: "Sonnet"}}},
			{ID: "api", DisplayName: "API", Enabled: false, DisabledReason: "disabled"},
		},
	})
	if len(items) != 3 {
		t.Fatalf("selection item count = %d, want resume + default + model", len(items))
	}
	resume := items[0].(selectionItem).selection
	model := items[2].(selectionItem).selection
	if resume.ResumeID != "session-1234567890" || model.Engine != "claude" || model.Model != "sonnet" {
		t.Fatalf("resume=%+v model=%+v", resume, model)
	}
}
