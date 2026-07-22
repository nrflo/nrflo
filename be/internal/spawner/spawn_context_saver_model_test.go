package spawner

import "testing"

// TestContextSaverModel_APIDyingAgent verifies that an api-backend dying
// agent's own resolved bare model + reasoning effort (proc.modelID /
// proc.resolvedEffort — the values actually used for that spawn, reflecting
// any LowConsumptionMode/agentDef overrides) are inherited, NOT the passed
// defModel (which mirrors sysDef.Model in spawnContextSaver).
func TestContextSaverModel_APIDyingAgent(t *testing.T) {
	sp := New(Config{})
	proc := &processInfo{
		agentType:      "implementor",
		modelID:        "claude:sonnet-5",
		resolvedEffort: "high",
		backend:        &mockBackend{name: "api"},
	}

	model, effort := sp.contextSaverModel(proc, "haiku-4-5")

	if model != "sonnet-5" {
		t.Errorf("contextSaverModel() model = %q, want %q", model, "sonnet-5")
	}
	if effort == nil || *effort != "high" {
		t.Errorf("contextSaverModel() effort = %v, want %q", effort, "high")
	}
}

// TestContextSaverModel_CLIDyingAgent verifies that a cli-backend dying
// agent's bare model is inherited with a nil effort when none was resolved.
func TestContextSaverModel_CLIDyingAgent(t *testing.T) {
	sp := New(Config{})
	proc := &processInfo{
		agentType: "analyzer",
		modelID:   "claude:opus-4-8",
		backend:   &mockBackend{name: "cli"},
	}

	model, effort := sp.contextSaverModel(proc, "haiku-4-5")

	if model != "opus-4-8" {
		t.Errorf("contextSaverModel() model = %q, want %q", model, "opus-4-8")
	}
	if effort != nil {
		t.Errorf("contextSaverModel() effort = %v, want nil", effort)
	}
}

// TestContextSaverModel_FallbackEmptyModel verifies that when the dying
// agent's modelID is empty/unresolvable, contextSaverModel falls back to
// defModel with a nil effort.
func TestContextSaverModel_FallbackEmptyModel(t *testing.T) {
	sp := New(Config{})
	proc := &processInfo{agentType: "implementor", modelID: "", backend: &mockBackend{name: "api"}}

	model, effort := sp.contextSaverModel(proc, "haiku-4-5")

	if model != "haiku-4-5" {
		t.Errorf("contextSaverModel() model = %q, want fallback %q", model, "haiku-4-5")
	}
	if effort != nil {
		t.Errorf("contextSaverModel() effort = %v, want nil", effort)
	}
}
