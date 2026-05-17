package orchestrator

import (
	"context"
	"testing"
)

// TestOrchestrator_Start_ProjectScope_SeedFindings verifies that SeedFindings on
// RunRequest are written into the findings table before the run goroutine starts,
// and that the orchestrator's own _orchestration key is also present.
func TestOrchestrator_Start_ProjectScope_SeedFindings(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	// Create a project-scoped workflow with a single agent.
	env.createWorkflowWithAgents(t, "spec-import", "Spec import workflow", "project", []struct {
		ID    string
		Layer int
	}{
		{ID: "spec-normalizer", Layer: 0},
	})

	seed := map[string]string{
		"import_id":  "spec-99",
		"source_url": "https://example.com/spec",
	}

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "spec-import",
		ScopeType:    "project",
		SeedFindings: seed,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if result.InstanceID == "" {
		t.Fatalf("InstanceID is empty after Start")
	}

	// Stop the run goroutine to avoid interfering with other tests.
	env.stopAndWaitRun(t, result.InstanceID)

	findings := getWFIFindings(t, env, result.InstanceID)

	// Seeded keys must be present.
	if findings["import_id"] != "spec-99" {
		t.Errorf("findings[import_id] = %v, want %q", findings["import_id"], "spec-99")
	}
	if findings["source_url"] != "https://example.com/spec" {
		t.Errorf("findings[source_url] = %v, want %q", findings["source_url"], "https://example.com/spec")
	}

	// Orchestrator merges _orchestration key after seeding.
	if _, ok := findings["_orchestration"]; !ok {
		t.Errorf("_orchestration key missing from findings; got keys: %v", findingKeys(findings))
	}
}

// TestOrchestrator_Start_TicketScope_SeedFindings verifies SeedFindings are persisted
// for ticket-scoped workflow instances.
func TestOrchestrator_Start_TicketScope_SeedFindings(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.createTicket(t, "tkt-seed", "Seed findings ticket")

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		TicketID:     "tkt-seed",
		WorkflowName: "test",
		ScopeType:    "ticket",
		Force:        true,
		SeedFindings: map[string]string{"spec_ref": "https://docs.example.com"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	env.stopAndWaitRun(t, result.InstanceID)

	findings := getWFIFindings(t, env, result.InstanceID)

	if findings["spec_ref"] != "https://docs.example.com" {
		t.Errorf("findings[spec_ref] = %v, want %q", findings["spec_ref"], "https://docs.example.com")
	}
	if _, ok := findings["_orchestration"]; !ok {
		t.Errorf("_orchestration key missing from findings")
	}
}

// TestOrchestrator_Start_EmptySeedFindings verifies that nil/empty SeedFindings
// produces valid findings with at least the _orchestration key.
func TestOrchestrator_Start_EmptySeedFindings(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.createWorkflowWithAgents(t, "empty-seed-wf", "Empty seed workflow", "project", []struct {
		ID    string
		Layer int
	}{
		{ID: "agent-a", Layer: 0},
	})

	result, err := env.orch.Start(context.Background(), RunRequest{
		ProjectID:    env.project,
		WorkflowName: "empty-seed-wf",
		ScopeType:    "project",
		SeedFindings: nil,
	})
	if err != nil {
		t.Fatalf("Start with nil SeedFindings: %v", err)
	}

	env.stopAndWaitRun(t, result.InstanceID)

	findings := getWFIFindings(t, env, result.InstanceID)
	if _, ok := findings["_orchestration"]; !ok {
		t.Errorf("_orchestration missing from findings with empty seed")
	}
}

// findingKeys returns the keys of a findings map for error messages.
func findingKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
