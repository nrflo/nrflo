package console

import (
	"testing"

	"be/internal/service"
	"be/internal/spawner"
	"be/internal/types"
)

func TestModelResolversUseModeSpecificFields(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	cliSpec := &spawner.EngineSpec{}
	if err := modelResolverFor("claude").Resolve(pool, clk, cliSpec, "opus-4-6", "high"); err != nil {
		t.Fatalf("CLI Resolve: %v", err)
	}
	if cliSpec.Model != "claude-opus-4-6" || cliSpec.MaxContext != 200000 || cliSpec.ReasoningEffort != "high" {
		t.Fatalf("CLI spec = %+v", cliSpec)
	}

	apiSpec := &spawner.EngineSpec{}
	if err := modelResolverFor("api").Resolve(pool, clk, apiSpec, "opus-4-6", "max"); err != nil {
		t.Fatalf("API Resolve: %v", err)
	}
	if apiSpec.Model != "claude-opus-4-6" || apiSpec.APIProvider != "anthropic" ||
		apiSpec.MaxContext != 1000000 || apiSpec.ReasoningEffort != "max" {
		t.Fatalf("API spec = %+v", apiSpec)
	}
}

func TestCLIModelResolverValidatesProviderEngine(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	if err := modelResolverFor("claude").Resolve(pool, clk, &spawner.EngineSpec{}, "gpt-5.4", ""); err == nil {
		t.Fatal("openai model with claude engine: want error")
	}
	if err := modelResolverFor("codex").Resolve(pool, clk, &spawner.EngineSpec{}, "sonnet-5", ""); err == nil {
		t.Fatal("anthropic model with codex engine: want error")
	}
}

func TestCLIModelResolverUnknownIDPassesThroughRaw(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	spec := &spawner.EngineSpec{}
	if err := modelResolverFor("claude").Resolve(pool, clk, spec, "totally-unknown-id", "high"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if spec.Model != "totally-unknown-id" || spec.ReasoningEffort != "high" {
		t.Fatalf("raw spec = %+v", spec)
	}
}

func TestAPIModelResolverRejectsUnknownDisabledAndUnsupported(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	resolver := modelResolverFor("api")
	if err := resolver.Resolve(pool, clk, &spawner.EngineSpec{}, "no-such-model", ""); err == nil {
		t.Fatal("unknown model: want error")
	}
	if err := resolver.Resolve(pool, clk, &spawner.EngineSpec{}, "gpt-5.5-mini", ""); err == nil {
		t.Fatal("disabled model: want error")
	}

	models := service.NewModelService(pool, clk)
	if _, err := models.Create(types.ModelCreateRequest{
		ID: "custom-disabled", Provider: "anthropic", DisplayName: "Disabled",
		APIModel: "claude-x", APIContext: 200000,
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}
	disabled := false
	if _, err := models.Update("custom-disabled", types.ModelUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable model: %v", err)
	}
	if err := resolver.Resolve(pool, clk, &spawner.EngineSpec{}, "custom-disabled", ""); err == nil {
		t.Fatal("disabled model: want error")
	}
}

func TestModelResolversApplyDefaultAndValidateOverride(t *testing.T) {
	t.Parallel()
	pool, clk := newSpecTestPool(t)

	for _, tc := range []struct {
		name, engine, id, wantDefault, allowed, rejected string
	}{
		{name: "api", engine: "api", id: "gpt-5.4", wantDefault: "medium", allowed: "xhigh", rejected: "ultra"},
		{name: "cli", engine: "codex", id: "gpt-5.6-sol", wantDefault: "low", allowed: "ultra", rejected: "impossible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := &spawner.EngineSpec{}
			if err := modelResolverFor(tc.engine).Resolve(pool, clk, spec, tc.id, ""); err != nil {
				t.Fatalf("default Resolve: %v", err)
			}
			if spec.ReasoningEffort != tc.wantDefault {
				t.Fatalf("default effort = %q, want %q", spec.ReasoningEffort, tc.wantDefault)
			}
			if err := modelResolverFor(tc.engine).Resolve(pool, clk, spec, tc.id, tc.allowed); err != nil {
				t.Fatalf("allowed override: %v", err)
			}
			if spec.ReasoningEffort != tc.allowed {
				t.Fatalf("override effort = %q, want %q", spec.ReasoningEffort, tc.allowed)
			}
			if err := modelResolverFor(tc.engine).Resolve(pool, clk, &spawner.EngineSpec{}, tc.id, tc.rejected); err == nil {
				t.Fatal("unsupported override: want error")
			}
		})
	}
}
