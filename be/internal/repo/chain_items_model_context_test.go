package repo

import (
	"testing"

	"be/internal/clock"
)

func TestChainItemsTotalTokensUsesEffectiveModeContext(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	statements := []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('chain-model-proj', 'P', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('chain-model-proj', 'wf', '', 'ticket', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		 VALUES ('wfi-chain-model', 'chain-model-proj', 'T-1', 'wf', 'completed', 'ticket', datetime('now'), datetime('now'))`,
		`INSERT INTO chain_executions (id, project_id, name, status, workflow_name, created_by, created_at, updated_at)
		 VALUES ('chain-model', 'chain-model-proj', 'C', 'completed', 'wf', '', datetime('now'), datetime('now'))`,
		`INSERT INTO chain_execution_items (id, chain_id, ticket_id, position, status, workflow_instance_id)
		 VALUES ('item-model', 'chain-model', 'T-1', 0, 'completed', 'wfi-chain-model')`,
		`INSERT INTO agent_sessions
		 (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, effective_mode, context_left, created_at, updated_at)
		 VALUES ('sess-cli-context', 'chain-model-proj', 'T-1', 'wfi-chain-model', 'p1', 'a1', 'opus-4-8', 'completed', 'cli_interactive', 50, datetime('now'), datetime('now'))`,
		`INSERT INTO agent_sessions
		 (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, effective_mode, context_left, created_at, updated_at)
		 VALUES ('sess-api-context', 'chain-model-proj', 'T-1', 'wfi-chain-model', 'p2', 'a2', 'opus-4-8', 'completed', 'api', 50, datetime('now'), datetime('now'))`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(statement); err != nil {
			t.Fatalf("seed chain context fixture: %v", err)
		}
	}

	items, err := NewChainItemRepo(pool, clock.Real()).ListByChain("chain-model")
	if err != nil {
		t.Fatalf("ListByChain: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	// opus-4-8 is 200k in CLI mode and 1M in API mode. At 50%% context
	// remaining, the two sessions consumed 100k + 500k tokens.
	if got := items[0].TotalTokensUsed; got != 600000 {
		t.Fatalf("TotalTokensUsed = %d, want 600000", got)
	}
}
