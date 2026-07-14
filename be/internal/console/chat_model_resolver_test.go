package console

import (
	"testing"

	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

// TestModelResolverFor_SonnetCollision verifies the regression the ticket
// names: cli_models and api_models both seed an id="sonnet" row (migrations
// 000043/000128, kept in sync on mapped_model by later migrations), and
// modelResolverFor(engine) must resolve against the correct table. The seeded
// rows currently happen to share the same mapped_model, so a resolver
// silently reading the wrong table wouldn't be caught by comparing seeded
// values alone — this test proves table selection directly by disabling only
// the api_models "sonnet" row and confirming the api resolver (and only the
// api resolver) is affected.
func TestModelResolverFor_SonnetCollision(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	cliSpec := &spawner.EngineSpec{}
	if err := modelResolverFor("claude").Resolve(pool, clk, cliSpec, "sonnet"); err != nil {
		t.Fatalf("cli resolver for sonnet (before disabling api_models row): %v", err)
	}
	apiSpec := &spawner.EngineSpec{}
	if err := modelResolverFor("api").Resolve(pool, clk, apiSpec, "sonnet"); err != nil {
		t.Fatalf("api resolver for sonnet (before disabling api_models row): %v", err)
	}
	if apiSpec.APIProvider != "anthropic" {
		t.Errorf("api_models sonnet -> APIProvider = %q, want anthropic", apiSpec.APIProvider)
	}

	// Disable ONLY the api_models "sonnet" row. A read_only built-in row only
	// accepts reasoning_effort/fallback_models updates through the service
	// layer (service/CLAUDE.md), so flip enabled directly via SQL — this test
	// simulates two independent tables diverging, not an admin-facing flow.
	if _, err := pool.Exec(`UPDATE api_models SET enabled = 0 WHERE id = 'sonnet'`); err != nil {
		t.Fatalf("disable api_models sonnet: %v", err)
	}

	// The cli resolver must be unaffected — it never touches api_models.
	cliSpecAfter := &spawner.EngineSpec{}
	if err := modelResolverFor("claude").Resolve(pool, clk, cliSpecAfter, "sonnet"); err != nil {
		t.Fatalf("cli resolver for sonnet (after disabling api_models row): %v", err)
	}
	if cliSpecAfter.Model != cliSpec.Model {
		t.Errorf("cli resolver's Model changed after disabling the api_models row (%q -> %q) — it must be reading api_models, not cli_models",
			cliSpec.Model, cliSpecAfter.Model)
	}

	// The api resolver must now error — proving it read api_models, not the
	// still-enabled cli_models row for the same id.
	if err := modelResolverFor("api").Resolve(pool, clk, &spawner.EngineSpec{}, "sonnet"); err == nil {
		t.Error("api resolver for sonnet (after disabling api_models row): want error, got nil — it must be reading api_models, not cli_models")
	}
}

// TestModelResolverFor_HaikuCollision mirrors the sonnet case for "haiku",
// from the cli_models side: disabling only the cli_models "haiku" row must
// break the cli resolver without affecting the api resolver.
func TestModelResolverFor_HaikuCollision(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	apiSpec := &spawner.EngineSpec{}
	if err := modelResolverFor("api").Resolve(pool, clk, apiSpec, "haiku"); err != nil {
		t.Fatalf("api resolver for haiku (before disabling cli_models row): %v", err)
	}

	// See TestModelResolverFor_SonnetCollision: flip enabled via SQL since a
	// read_only row rejects an Enabled update through the service layer.
	if _, err := pool.Exec(`UPDATE cli_models SET enabled = 0 WHERE id = 'haiku'`); err != nil {
		t.Fatalf("disable cli_models haiku: %v", err)
	}

	if err := modelResolverFor("claude").Resolve(pool, clk, &spawner.EngineSpec{}, "haiku"); err == nil {
		t.Error("cli resolver for haiku (after disabling cli_models row): want error, got nil")
	}

	apiSpecAfter := &spawner.EngineSpec{}
	if err := modelResolverFor("api").Resolve(pool, clk, apiSpecAfter, "haiku"); err != nil {
		t.Fatalf("api resolver for haiku (after disabling cli_models row): want success (unaffected), got error: %v", err)
	}
	if apiSpecAfter.Model != apiSpec.Model {
		t.Errorf("api resolver's Model changed after disabling the cli_models row (%q -> %q) — it must be reading cli_models, not api_models",
			apiSpec.Model, apiSpecAfter.Model)
	}
}

// TestAPIModelResolver_UnknownID_Errors verifies the api resolver always
// errors on an unknown id — unlike cli_models, a direct-API call cannot pass
// a raw model name through without a resolved provider.
func TestAPIModelResolver_UnknownID_Errors(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	spec := &spawner.EngineSpec{}
	err := modelResolverFor("api").Resolve(pool, clk, spec, "no-such-api-model")
	if err == nil {
		t.Fatal("api resolver for unknown id: want error, got nil")
	}
}

// TestAPIModelResolver_DisabledID_Errors verifies a disabled api_models row
// is rejected.
func TestAPIModelResolver_DisabledID_Errors(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	apiSvc := service.NewAPIModelService(pool, clk)
	if _, err := apiSvc.Create(types.APIModelCreateRequest{
		ID: "custom-disabled", Provider: "anthropic", DisplayName: "Disabled",
		MappedModel: "claude-x", ContextLength: 200000,
	}); err != nil {
		t.Fatalf("seed api_models row: %v", err)
	}
	disabled := false
	if _, err := apiSvc.Update("custom-disabled", types.APIModelUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable api_models row: %v", err)
	}

	spec := &spawner.EngineSpec{}
	err := modelResolverFor("api").Resolve(pool, clk, spec, "custom-disabled")
	if err == nil {
		t.Fatal("api resolver for disabled id: want error, got nil")
	}
}

// TestAPIModelResolver_KnownEnabled_ResolvesReasoningEffortAndContext verifies
// the api resolver carries ReasoningEffort/MaxContext through, not just Model.
func TestAPIModelResolver_KnownEnabled_ResolvesReasoningEffortAndContext(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	apiSvc := service.NewAPIModelService(pool, clk)
	if _, err := apiSvc.Create(types.APIModelCreateRequest{
		ID: "custom-reasoning", Provider: "openai", DisplayName: "Custom",
		MappedModel: "gpt-5.5", ReasoningEffort: "high", ContextLength: 300000,
	}); err != nil {
		t.Fatalf("seed api_models row: %v", err)
	}

	spec := &spawner.EngineSpec{}
	if err := modelResolverFor("api").Resolve(pool, clk, spec, "custom-reasoning"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if spec.Model != "gpt-5.5" {
		t.Errorf("Model = %q, want gpt-5.5", spec.Model)
	}
	if spec.APIProvider != "openai" {
		t.Errorf("APIProvider = %q, want openai", spec.APIProvider)
	}
	if spec.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want high", spec.ReasoningEffort)
	}
	if spec.MaxContext != 300000 {
		t.Errorf("MaxContext = %d, want 300000", spec.MaxContext)
	}
}

// TestCLIModelResolver_UnknownID_PassesThroughRaw verifies the cli resolver's
// distinct behavior from the api resolver: an unknown id is not an error, it
// passes through as a raw model name (already covered end-to-end by
// chat_spec_test.go's TestBuildChatEngineSpec_UnknownModelID_PassesThroughRaw;
// asserted directly here against the resolver in isolation).
func TestCLIModelResolver_UnknownID_PassesThroughRaw(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	spec := &spawner.EngineSpec{Model: "unused"}
	if err := modelResolverFor("claude").Resolve(pool, clk, spec, "totally-unknown-id"); err != nil {
		t.Fatalf("cli resolver for unknown id: %v", err)
	}
	if spec.Model != "unused" {
		t.Errorf("Model = %q, want unchanged (raw passthrough happens at the caller, resolver is a no-op on unknown ids)", spec.Model)
	}
}
