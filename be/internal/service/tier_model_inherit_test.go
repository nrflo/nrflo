package service

import (
	"testing"

	"be/internal/types"
)

// createModeOnlyModel creates a model enabled in exactly one mode (cli or
// api) — the counterpart to createTestModel/createModeOnlyModel is used to
// prove an inherit-mode (”) tier_models entry must validate against BOTH
// registry modes.
func createModeOnlyModel(t *testing.T, svc *TierModelService, id string, cliOnly bool) {
	t.Helper()
	req := types.ModelCreateRequest{
		ID: id, Provider: "anthropic", DisplayName: id,
		DefaultEffort: "low",
	}
	if cliOnly {
		req.CLIModel = id
		req.CLIEfforts = []string{"low"}
	} else {
		req.APIModel = id
		req.APIEfforts = []string{"low"}
	}
	if _, err := svc.modelSvc.Create(req); err != nil {
		t.Fatalf("create mode-only model %s: %v", id, err)
	}
}

// TestTierModel_SetTierChain_EmptyExecutionModeAccepted verifies an entry
// with execution_mode==” (inherit) is accepted and stored verbatim when its
// model is valid for both cli and api registry modes.
func TestTierModel_SetTierChain_EmptyExecutionModeAccepted(t *testing.T) {
	t.Parallel()
	svc, modelSvc, cleanup := setupTierModelTestEnv(t)
	defer cleanup()
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID: "both-modes-model", Provider: "anthropic", DisplayName: "both-modes-model",
		CLIModel: "both-modes-model", APIModel: "both-modes-model",
		CLIEfforts: []string{"low"}, APIEfforts: []string{"low"},
		DefaultEffort: "low",
	}); err != nil {
		t.Fatalf("create model: %v", err)
	}

	if err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "", ModelID: "both-modes-model"},
	}); err != nil {
		t.Fatalf("SetTierChain with execution_mode='': %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.Tier == 2 && r.Position == 0 {
			found = true
			if r.ExecutionMode != "" {
				t.Errorf("stored ExecutionMode = %q, want '' (verbatim)", r.ExecutionMode)
			}
			if r.ModelID != "both-modes-model" {
				t.Errorf("stored ModelID = %q, want both-modes-model", r.ModelID)
			}
		}
	}
	if !found {
		t.Fatal("tier=2 position=0 row not found after SetTierChain")
	}
}

// TestTierModel_SetTierChain_EmptyExecutionModeRejectsModeOnlyModel verifies
// an inherit-mode entry (”) referencing a model valid in only ONE of
// cli/api is rejected — it must be valid for both.
func TestTierModel_SetTierChain_EmptyExecutionModeRejectsModeOnlyModel(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()
	createModeOnlyModel(t, svc, "cli-only-model", true)

	err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "", ModelID: "cli-only-model"},
	})
	if err == nil {
		t.Fatal("SetTierChain with a cli-only model at execution_mode='': expected error, got nil")
	}
}

// TestTierModel_SetTierChain_EmptyExecutionModeRejectsAPIOnlyModel is the
// api-only counterpart.
func TestTierModel_SetTierChain_EmptyExecutionModeRejectsAPIOnlyModel(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()
	createModeOnlyModel(t, svc, "api-only-model", false)

	err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "", ModelID: "api-only-model"},
	})
	if err == nil {
		t.Fatal("SetTierChain with an api-only model at execution_mode='': expected error, got nil")
	}
}
