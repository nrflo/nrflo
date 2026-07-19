package spawner

import (
	"context"
	"testing"

	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
)

// TestDelegateRecursionGuard_T2ExtractorNeverHasDelegate verifies the native
// tools-CSV omission: a _t2_extractor worker's registry never includes
// delegate, regardless of depth.
func TestDelegateRecursionGuard_T2ExtractorNeverHasDelegate(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())
	sp.config.Agents = map[string]AgentConfig{
		"_t2_extractor": {Model: "haiku-4-5", ExecutionMode: "api", Tools: "findings_add,agent_finished,agent_fail"},
	}

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "_t2_extractor",
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:haiku-4-5", "_delegate", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn: %v", err)
	}

	if _, ok := prep.apiHandlers["delegate"]; ok {
		t.Error("delegate handler must be excluded from a _t2_extractor worker's registry")
	}
	for _, spec := range prep.apiTools {
		if spec.Name == "delegate" {
			t.Error("delegate tool spec must be excluded from a _t2_extractor worker's registry")
		}
	}
}

// TestDelegateRecursionGuard_T1ExecutorAtDepthCap verifies the depth-based
// guard: a _t1_executor worker whose spawner's chain depth has reached
// service.DelegateMaxDepth loses the delegate tool (DelegateDepth+1 > cap).
func TestDelegateRecursionGuard_T1ExecutorAtDepthCap(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())
	sp.config.DelegateDepth = service.DelegateMaxDepth(env.pool, env.projectID)
	sp.config.Agents = map[string]AgentConfig{
		"_t1_executor": {Model: "sonnet-5", ExecutionMode: "api", Tools: "*"},
	}

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "_t1_executor",
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "_delegate", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn: %v", err)
	}

	if _, ok := prep.apiHandlers["delegate"]; ok {
		t.Error("delegate handler must be excluded once delegate_depth is at the cap")
	}
	for _, spec := range prep.apiTools {
		if spec.Name == "delegate" {
			t.Error("delegate tool spec must be excluded once delegate_depth is at the cap")
		}
	}
}

// TestDelegateRecursionGuard_T1ExecutorBelowDepthCap verifies the guard is
// depth-based, not a blanket removal: a _t1_executor worker below the cap
// keeps the delegate tool.
func TestDelegateRecursionGuard_T1ExecutorBelowDepthCap(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())
	sp.config.Agents = map[string]AgentConfig{
		"_t1_executor": {Model: "sonnet-5", ExecutionMode: "api", Tools: "*"},
	}

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "_t1_executor",
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "_delegate", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn: %v", err)
	}

	if _, ok := prep.apiHandlers["delegate"]; !ok {
		t.Error("delegate handler must be present for a _t1_executor worker below the depth cap")
	}
}
