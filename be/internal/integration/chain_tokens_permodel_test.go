package integration

import (
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestChainItemTokensUsed_PerModelContext verifies that chain item token totals
// resolve the per-model context from the models table even though production
// stores prefixed model_id values (`<cli>:<slug>`). The chain query must strip
// the `<prefix>:` before joining models, pick api_context vs cli_context by the
// session's effective_mode, and fall back to 200000 for unknown ids.
func TestChainItemTokensUsed_PerModelContext(t *testing.T) {
	env := NewTestEnv(t)

	base := time.Now()
	createChainTickets(t, env, map[string]time.Time{
		"PM-A": base,
		"PM-B": base.Add(time.Second),
	})

	chainSvc := service.NewChainService(env.Pool, env.Clock)
	chain, err := chainSvc.CreateChain(env.ProjectID, &types.ChainCreateRequest{
		Name:         "Per-Model Context Chain",
		WorkflowName: "test",
		TicketIDs:    []string{"PM-A", "PM-B"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}

	// PM-A: two sessions with PREFIXED model ids, exactly as the spawner writes them.
	//   sess-pma-1: claude:opus-4-7-1m, cli mode, context_left=80
	//     → cli_context 1000000 * (100-80)/100 = 200000
	//       (a bare-id join would miss the prefixed row and fall back to 200000*20/100=40000)
	//   sess-pma-2: claude:opus-4-7, api mode, context_left=50
	//     → api_context 1000000 * (100-50)/100 = 500000
	//       (opus-4-7 cli_context is only 200000, so this also proves per-mode selection)
	// Total PM-A = 700000
	wfiA := "wfi-pma-001"
	env.InitWorkflowWithID(t, "PM-A", wfiA)
	insertSessionWithContextLeft(t, env, "sess-pma-1", "PM-A", wfiA,
		"analyzer", "setup-analyzer", "claude:opus-4-7-1m", "completed", "pass", 80)
	insertSessionWithContextLeft(t, env, "sess-pma-2", "PM-A", wfiA,
		"builder", "implementor", "claude:opus-4-7", "completed", "pass", 50)
	setSessionEffectiveMode(t, env, "sess-pma-2", "api")

	// PM-B: one session whose model is unknown to the catalog → falls back to 200000.
	//   codex:gpt-does-not-exist, context_left=50 → 200000 * (100-50)/100 = 100000
	wfiB := "wfi-pmb-001"
	env.InitWorkflowWithID(t, "PM-B", wfiB)
	insertSessionWithContextLeft(t, env, "sess-pmb-1", "PM-B", wfiB,
		"analyzer", "setup-analyzer", "codex:gpt-does-not-exist", "completed", "pass", 50)

	itemRepo := repo.NewChainItemRepo(env.Pool, env.Clock)
	items, err := itemRepo.ListByChain(chain.ID)
	if err != nil {
		t.Fatalf("failed to list chain items: %v", err)
	}

	for _, item := range items {
		switch item.TicketID {
		case "pm-a":
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiA); err != nil {
				t.Fatalf("failed to set wfi for PM-A: %v", err)
			}
		case "pm-b":
			if err := itemRepo.SetWorkflowInstanceID(item.ID, wfiB); err != nil {
				t.Fatalf("failed to set wfi for PM-B: %v", err)
			}
		}
	}

	retrieved, err := chainSvc.GetChainWithItems(chain.ID)
	if err != nil {
		t.Fatalf("GetChainWithItems failed: %v", err)
	}

	if len(retrieved.Items) != 2 {
		t.Fatalf("expected 2 chain items, got %d", len(retrieved.Items))
	}

	expected := map[string]int64{
		"pm-a": 700_000, // 1000000*0.20 (cli) + 1000000*0.50 (api)
		"pm-b": 100_000, // 200000*0.50 (unknown id fallback)
	}

	for _, item := range retrieved.Items {
		want, ok := expected[item.TicketID]
		if !ok {
			t.Errorf("unexpected ticket ID: %s", item.TicketID)
			continue
		}
		if item.TotalTokensUsed != want {
			t.Errorf("ticket %s: expected total_tokens_used %d, got %d",
				item.TicketID, want, item.TotalTokensUsed)
		}
	}
}

// setSessionEffectiveMode sets effective_mode on an already-inserted session so
// the chain query selects api_context instead of cli_context.
func setSessionEffectiveMode(t *testing.T, env *TestEnv, sessionID, mode string) {
	t.Helper()
	if _, err := env.Pool.Exec(
		`UPDATE agent_sessions SET effective_mode = ? WHERE id = ?`, mode, sessionID,
	); err != nil {
		t.Fatalf("failed to set effective_mode for %s: %v", sessionID, err)
	}
}
