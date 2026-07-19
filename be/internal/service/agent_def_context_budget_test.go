package service

import (
	"fmt"
	"testing"

	"be/internal/types"
)

// TestAgentDef_ContextBudgetTokens_CreateGetRoundTrip covers the nullable-int
// context_budget_tokens field through Create + Get: a set value round-trips,
// an omitted field stays nil (inherit global default), and explicit 0 stays
// a non-nil pointer to 0 ("disabled") — the same NULL/0/value shape as the
// stall-timeout fields.
func TestAgentDef_ContextBudgetTokens_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		budget *int
		want   *int
	}{
		{name: "set_value", budget: intPtr(50000), want: intPtr(50000)},
		{name: "nil_sentinel_inherits_default", budget: nil, want: nil},
		{name: "zero_sentinel_disables", budget: intPtr(0), want: intPtr(0)},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, svc, wfID := setupAgentDefTestEnv(t, nil)
			id := fmt.Sprintf("ctxbudget-%d", i)

			def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:                  id,
				Prompt:              "do work",
				ContextBudgetTokens: tc.budget,
			})
			if err != nil {
				t.Fatalf("CreateAgentDef: %v", err)
			}
			assertIntPtr(t, "create ContextBudgetTokens", def.ContextBudgetTokens, tc.want)

			got, err := svc.GetAgentDef("proj1", wfID, id)
			if err != nil {
				t.Fatalf("GetAgentDef: %v", err)
			}
			assertIntPtr(t, "get ContextBudgetTokens", got.ContextBudgetTokens, tc.want)
		})
	}
}

// TestAgentDef_ContextBudgetTokens_CreateNegative_Rejected verifies Create
// rejects a negative value instead of silently persisting it.
func TestAgentDef_ContextBudgetTokens_CreateNegative_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "ctxbudget-neg", Prompt: "do work", ContextBudgetTokens: intPtr(-1),
	})
	if err == nil {
		t.Fatal("CreateAgentDef with negative context_budget_tokens = nil error, want validation error")
	}
}

// TestAgentDef_ContextBudgetTokens_Update verifies UpdateAgentDef can set the
// budget on a def that lacked one, and rejects a negative update value.
func TestAgentDef_ContextBudgetTokens_Update(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "upd-ctxbudget", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.UpdateAgentDef("proj1", wfID, "upd-ctxbudget", &types.AgentDefUpdateRequest{
		ContextBudgetTokens: intPtr(80000),
	}); err != nil {
		t.Fatalf("UpdateAgentDef: %v", err)
	}

	def, err := svc.GetAgentDef("proj1", wfID, "upd-ctxbudget")
	if err != nil {
		t.Fatalf("GetAgentDef after update: %v", err)
	}
	assertIntPtr(t, "ContextBudgetTokens after update", def.ContextBudgetTokens, intPtr(80000))

	if err := svc.UpdateAgentDef("proj1", wfID, "upd-ctxbudget", &types.AgentDefUpdateRequest{
		ContextBudgetTokens: intPtr(-5),
	}); err == nil {
		t.Error("UpdateAgentDef with negative context_budget_tokens = nil error, want validation error")
	}
}

// TestAgentDef_ContextBudgetTokens_List verifies ListAgentDefs returns the
// budget for set and unset defs.
func TestAgentDef_ContextBudgetTokens_List(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-ctxbudget-agent", Prompt: "do work", ContextBudgetTokens: intPtr(60000),
	}); err != nil {
		t.Fatalf("create with budget: %v", err)
	}
	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID: "list-no-ctxbudget-agent", Prompt: "do work",
	}); err != nil {
		t.Fatalf("create without budget: %v", err)
	}

	defs, err := svc.ListAgentDefs("proj1", wfID)
	if err != nil {
		t.Fatalf("ListAgentDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	var withBudget, withoutBudget int
	for _, d := range defs {
		if d.ContextBudgetTokens != nil {
			withBudget++
			if *d.ContextBudgetTokens != 60000 {
				t.Errorf("ContextBudgetTokens = %d, want 60000", *d.ContextBudgetTokens)
			}
		} else {
			withoutBudget++
		}
	}
	if withBudget != 1 || withoutBudget != 1 {
		t.Errorf("withBudget=%d withoutBudget=%d, want 1 each", withBudget, withoutBudget)
	}
}
