package console

import (
	"context"
	"strings"
	"testing"

	"be/internal/spawner"
)

// TestBuildChatEngineSpec_CatalogueWithDelegate_AppendsGuidanceOnce verifies
// buildChatEngineSpec's Catalogue param feeds
// spawner.AppendDelegationGuidanceForTools: a catalogue enumerating
// "delegate" (e.g. t0-bare's) gets the delegation-guidance injectable
// appended after the rendered tier-t0-bare role template, exactly once.
func TestBuildChatEngineSpec_CatalogueWithDelegate_AppendsGuidanceOnce(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-delegation", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-delegation", Engine: "codex", ModelID: "",
		SpawnToken: "tok", SystemTemplateID: "tier-t0-bare",
		Catalogue: []string{"delegate", "get_delegation", "ticket_list"},
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if !strings.Contains(spec.SystemPrompt, "get_delegation") {
		t.Errorf("spec.SystemPrompt missing %q anchor; got %q", "get_delegation", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "extractor") {
		t.Errorf("spec.SystemPrompt missing %q anchor; got %q", "extractor", spec.SystemPrompt)
	}
	if got := strings.Count(spec.SystemPrompt, "## Role: T0 Bare"); got != 1 {
		t.Errorf("count(%q) = %d, want 1; spec.SystemPrompt = %q", "## Role: T0 Bare", got, spec.SystemPrompt)
	}
}

// TestBuildChatEngineSpec_CatalogueWithoutDelegate_ByteIdenticalToPlainRender
// verifies a catalogue without "delegate" (e.g. a t2-extractor-style
// catalogue) leaves spec.SystemPrompt byte-identical to the plain rendered
// template — no guidance appended.
func TestBuildChatEngineSpec_CatalogueWithoutDelegate_ByteIdenticalToPlainRender(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-nodelegation", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-nodelegation", Engine: "codex", ModelID: "",
		SpawnToken: "tok", SystemTemplateID: "tier-t2-extractor",
		Catalogue: []string{"ticket_list"},
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}

	vars := map[string]string{"PROJECT_ID": "proj-spec-nodelegation", "MODEL": "", "NODE_ID": "s1"}
	want := spawner.RenderInjectable(context.Background(), pool, "tier-t2-extractor", vars)
	if spec.SystemPrompt != want {
		t.Errorf("spec.SystemPrompt = %q, want byte-identical plain render %q", spec.SystemPrompt, want)
	}
	if strings.Contains(spec.SystemPrompt, "get_delegation") {
		t.Errorf("spec.SystemPrompt contains delegation guidance sentinel %q; want absent", "get_delegation")
	}
}

// TestBuildChatEngineSpec_NilCatalogue_NoDelegationAppend verifies the
// no-profile/t0-hands case (nil Catalogue) never appends guidance even when
// SystemTemplateID is set — the append gates on the enumerated catalogue,
// not the resolved registry.
func TestBuildChatEngineSpec_NilCatalogue_NoDelegationAppend(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)
	seedSpecProject(t, pool, "proj-spec-nilcatalogue", "")

	spec, err := buildChatEngineSpec(pool, clk, chatSpecParams{
		SessionID: "s1", ProjectID: "proj-spec-nilcatalogue", Engine: "codex", ModelID: "",
		SpawnToken: "tok", SystemTemplateID: "tier-t2-extractor", Catalogue: nil,
	})
	if err != nil {
		t.Fatalf("buildChatEngineSpec: %v", err)
	}
	if strings.Contains(spec.SystemPrompt, "get_delegation") {
		t.Errorf("spec.SystemPrompt contains delegation guidance sentinel with nil Catalogue; got %q", spec.SystemPrompt)
	}
}
