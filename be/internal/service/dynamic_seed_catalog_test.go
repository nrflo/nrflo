package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// TestDynAgents_FanoutTemplatesHaveDescriptions guards the planner's
// selection surface: renderTemplateLibrary shows a template's description
// (not its prompt), so an undescribed fanout_template would be unusable in
// the planner's ${TEMPLATE_LIBRARY}.
func TestDynAgents_FanoutTemplatesHaveDescriptions(t *testing.T) {
	t.Parallel()
	for _, a := range dynAgents {
		if a.NodeRole != "" && a.NodeRole != "fanout_template" {
			continue
		}
		if strings.TrimSpace(a.Description) == "" {
			t.Errorf("dynAgents[%q]: fanout_template with empty description", a.ID)
		}
	}
}

// TestDynAgents_PlannerHasEmitFindingsAndRole verifies the workflow-local
// planner override carries node_role='planner' and grants emit_findings —
// validateNodeRole would otherwise reject it at CreateAgentDef time (the
// seed bypasses that service layer, so nothing else would catch a drift).
func TestDynAgents_PlannerHasEmitFindingsAndRole(t *testing.T) {
	t.Parallel()
	var planners []dynAgent
	for _, a := range dynAgents {
		if a.NodeRole == "planner" {
			planners = append(planners, a)
		}
	}
	if len(planners) != 1 {
		t.Fatalf("planner entries in dynAgents = %d, want 1", len(planners))
	}
	p := planners[0]
	if p.ID != "dynamic-planner" {
		t.Errorf("planner ID = %q, want dynamic-planner", p.ID)
	}
	if !csvGrantsTool(p.Tools, "emit_findings") {
		t.Errorf("planner Tools = %q, must grant emit_findings", p.Tools)
	}
}

// TestDynAgents_ModelsResolveToEnabledCLIModels verifies every seeded model
// id (planner + fanout templates) resolves to an enabled CLI-capable model row —
// the seed bypasses AgentDefinitionService's model validation (shipped
// data), so this is the only guard against a typo'd or retired model id.
func TestDynAgents_ModelsResolveToEnabledCLIModels(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dyn_catalog_models.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	modelSvc := NewModelService(pool, clock.Real())
	for _, a := range dynAgents {
		valid, err := modelSvc.IsValidModelForMode(a.Model, "cli")
		if err != nil {
			t.Fatalf("IsValidModel(%q) for %s: %v", a.Model, a.ID, err)
		}
		if !valid {
			t.Errorf("dynAgents[%q]: model %q is not an enabled cli model", a.ID, a.Model)
		}
		if (a.ID == "module-reviewer-codex" || a.ID == "finding-verifier-codex") && a.ReasoningEffort != "high" {
			t.Errorf("dynAgents[%q]: reasoning effort = %q, want high", a.ID, a.ReasoningEffort)
		}
	}
}

// TestDynAgents_PromptsResolveAndCarryNodeInstructions guards two silent
// failures: dynPrompt returns "" for an id with no switch case (a new dynAgents
// entry would seed a blank-prompt agent), and a fanout_template prompt without
// the ${NODE_INSTRUCTIONS} slot silently drops its per-node instructions —
// orchestrator_spawn.go substitutes them into that var and nothing else.
func TestDynAgents_PromptsResolveAndCarryNodeInstructions(t *testing.T) {
	t.Parallel()
	for _, a := range dynAgents {
		prompt := dynPrompt(a.ID)
		if strings.TrimSpace(prompt) == "" {
			t.Errorf("dynPrompt(%q) is empty: missing switch case in dynPrompt", a.ID)
			continue
		}
		if a.NodeRole == "planner" {
			continue // the planner is fed PLAN_* vars, never NODE_INSTRUCTIONS
		}
		if !strings.Contains(prompt, "${NODE_INSTRUCTIONS}") {
			t.Errorf("dynAgents[%q]: fanout_template prompt lacks the ${NODE_INSTRUCTIONS} slot", a.ID)
		}
	}
}

// TestDynAgents_ReasoningEffortDefaults pins the cheap-tier-default effort
// assignments: the 7 non-codex worker/verifier templates default to "low",
// the synthesizer (the single mid-tier synthesis node) to "medium" — the soft
// complement to the server-side EnforcePremiumWorkerCap guardrail. Codex
// twins staying "high" is locked separately by
// TestDynAgents_ModelsResolveToEnabledCLIModels.
func TestDynAgents_ReasoningEffortDefaults(t *testing.T) {
	t.Parallel()
	lowEffortIDs := map[string]bool{
		"codebase-explorer": true, "module-reviewer": true, "implementor-worker": true,
		"web-researcher": true, "finding-verifier": true, "generic-worker": true, "cross-checker": true,
		"web-researcher-cheap": true, "premise-auditor": true,
	}
	byID := make(map[string]dynAgent, len(dynAgents))
	for _, a := range dynAgents {
		byID[a.ID] = a
	}

	for id := range lowEffortIDs {
		a, ok := byID[id]
		if !ok {
			t.Fatalf("dynAgents missing expected low-effort worker/verifier id %q", id)
		}
		if a.ReasoningEffort != "low" {
			t.Errorf("dynAgents[%q].ReasoningEffort = %q, want %q", id, a.ReasoningEffort, "low")
		}
	}

	synth, ok := byID["synthesizer"]
	if !ok {
		t.Fatal("dynAgents missing the synthesizer entry")
	}
	if synth.ReasoningEffort != "medium" {
		t.Errorf("dynAgents[%q].ReasoningEffort = %q, want %q", "synthesizer", synth.ReasoningEffort, "medium")
	}
}

// TestDynAgents_RosterShape pins the catalog size the plan specifies: 12
// fanout templates + 1 workflow-local planner override, 13 total.
func TestDynAgents_RosterShape(t *testing.T) {
	t.Parallel()
	if len(dynAgents) != 13 {
		t.Fatalf("len(dynAgents) = %d, want 13 (12 fanout_template + 1 planner)", len(dynAgents))
	}
	seen := make(map[string]bool, len(dynAgents))
	for _, a := range dynAgents {
		if seen[a.ID] {
			t.Errorf("duplicate dynAgents id %q", a.ID)
		}
		seen[a.ID] = true
	}
}
