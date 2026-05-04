package integration

import (
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestChainItemTokensUsed_PerModelContext verifies that chain item token totals
// use the per-model context_length from cli_models rather than the 200000 default.
// opus_4_7_1m has context_length=1000000; two completed sessions with context_left=50
// each yield 1000000*(100-50)/100 * 2 = 1_000_000.
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

	// PM-A: 2 sessions at context_left=50 with opus_4_7_1m (context_length=1000000)
	// 2 × (1000000*(100-50)/100) = 1_000_000
	wfiA := "wfi-pma-001"
	env.InitWorkflowWithID(t, "PM-A", wfiA)
	insertSessionWithContextLeft(t, env, "sess-pma-1", "PM-A", wfiA,
		"analyzer", "setup-analyzer", "opus_4_7_1m", "completed", "pass", 50)
	insertSessionWithContextLeft(t, env, "sess-pma-2", "PM-A", wfiA,
		"builder", "implementor", "opus_4_7_1m", "completed", "pass", 50)

	// PM-B: 1 session at context_left=50 with opus_4_7_1m → 500000
	wfiB := "wfi-pmb-001"
	env.InitWorkflowWithID(t, "PM-B", wfiB)
	insertSessionWithContextLeft(t, env, "sess-pmb-1", "PM-B", wfiB,
		"analyzer", "setup-analyzer", "opus_4_7_1m", "completed", "pass", 50)

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
		"pm-a": 1_000_000, // 2 × (1000000*(100-50)/100)
		"pm-b": 500_000,   // 1 × (1000000*(100-50)/100)
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
