package integration

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

// upsertWFIFinding stores a single finding for a workflow_instance scope.
func upsertWFIFinding(t *testing.T, env *TestEnv, wfiID, key string, value interface{}) {
	t.Helper()
	fr := repo.NewFindingRepo(env.Pool, clock.Real())
	val, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("upsertWFIFinding: marshal: %v", err)
	}
	if err := fr.Upsert("workflow_instance", wfiID, key, json.RawMessage(val), repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsertWFIFinding: upsert %s: %v", key, err)
	}
}

// getWFIFindings returns all findings for a workflow instance.
func getWFIFindings(t *testing.T, env *TestEnv, wfiID string) map[string]json.RawMessage {
	t.Helper()
	fr := repo.NewFindingRepo(env.Pool, clock.Real())
	raw, err := fr.GetOwn("workflow_instance", wfiID)
	if err != nil {
		t.Fatalf("getWFIFindings: %v", err)
	}
	return raw
}

// TestUserInstructionsEndToEnd validates the full flow: orchestrator stores
// user instructions as a direct string in workflow instance findings, and the
// spawner can retrieve them as a plain string (not a nested map).
func TestUserInstructionsEndToEnd(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-1", "User instructions test")
	env.InitWorkflow(t, "UI-1")

	wfiID := env.GetWorkflowInstanceID(t, "UI-1", "test")

	// --- Simulate what the orchestrator does: store findings via FindingRepo ---
	upsertWFIFinding(t, env, wfiID, "user_instructions", "Build the login page with OAuth support")
	upsertWFIFinding(t, env, wfiID, "_orchestration", map[string]interface{}{"status": "running"})

	// --- Simulate what the spawner does: read findings via FindingRepo ---
	retrievedFindings := getWFIFindings(t, env, wfiID)

	// The spawner does: unmarshal findings["user_instructions"] as string
	rawInstr, ok := retrievedFindings["user_instructions"]
	if !ok {
		t.Fatal("user_instructions key missing from findings")
	}
	var str string
	if err := json.Unmarshal(rawInstr, &str); err != nil {
		t.Fatalf("user_instructions is not a string: %v", err)
	}
	if str != "Build the login page with OAuth support" {
		t.Fatalf("expected 'Build the login page with OAuth support', got %q", str)
	}

	// Verify orchestration status is also present
	rawOrch, ok := retrievedFindings["_orchestration"]
	if !ok {
		t.Fatal("_orchestration key missing")
	}
	var orch map[string]interface{}
	if err := json.Unmarshal(rawOrch, &orch); err != nil {
		t.Fatalf("_orchestration unmarshal: %v", err)
	}
	if orch["status"] != "running" {
		t.Fatalf("expected orchestration status 'running', got %v", orch["status"])
	}

	// --- Verify via workflow status API (what the UI sees) ---
	status, err := getWorkflowStatus(t, env, "UI-1", &types.WorkflowGetRequest{
		Workflow: "test",
	})
	if err != nil {
		t.Fatalf("failed to get workflow status: %v", err)
	}
	// user_instructions should NOT appear in combined findings (agent findings only)
	if apiFindings, ok := status["findings"].(map[string]interface{}); ok {
		if _, exists := apiFindings["user_instructions"]; exists {
			t.Fatalf("user_instructions should not be in combined findings, got %v", apiFindings["user_instructions"])
		}
	}
	// user_instructions should appear in workflow_findings
	wfFindings, ok := status["workflow_findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("workflow_findings not in status response, got %T", status["workflow_findings"])
	}
	if wfFindings["user_instructions"] != "Build the login page with OAuth support" {
		t.Fatalf("workflow_findings returned wrong instructions: %v", wfFindings["user_instructions"])
	}
}

// TestUserInstructionsMissing verifies that when no instructions are stored,
// the spawner logic returns the placeholder string.
func TestUserInstructionsMissing(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-2", "No instructions test")
	env.InitWorkflow(t, "UI-2")

	wfiID := env.GetWorkflowInstanceID(t, "UI-2", "test")
	findings := getWFIFindings(t, env, wfiID)

	// No user_instructions key at all — spawner should return placeholder
	_, exists := findings["user_instructions"]
	if exists {
		t.Fatal("fresh workflow should not have user_instructions")
	}

	// Simulate spawner fallback: if key missing, return placeholder
	result := fetchInstructionsFromFindings(findings)
	if result != "_No user instructions provided_" {
		t.Fatalf("expected placeholder, got %q", result)
	}
}

// TestUserInstructionsEmptyString verifies that an empty string is treated
// the same as missing instructions.
func TestUserInstructionsEmptyString(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-3", "Empty instructions test")
	env.InitWorkflow(t, "UI-3")

	wfiID := env.GetWorkflowInstanceID(t, "UI-3", "test")
	upsertWFIFinding(t, env, wfiID, "user_instructions", "")

	// Re-read and check spawner logic
	findings := getWFIFindings(t, env, wfiID)
	result := fetchInstructionsFromFindings(findings)
	if result != "_No user instructions provided_" {
		t.Fatalf("expected placeholder for empty string, got %q", result)
	}
}

// TestUserInstructionsSpecialCharacters verifies that instructions with
// special characters (JSON-sensitive, markdown, newlines) survive the
// roundtrip through JSON serialization in the DB.
func TestUserInstructionsSpecialCharacters(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-4", "Special chars test")
	env.InitWorkflow(t, "UI-4")

	wfiID := env.GetWorkflowInstanceID(t, "UI-4", "test")

	instructions := "Use \"double quotes\" and 'single quotes'.\nAlso newlines\tand tabs.\n## Markdown heading\n- bullet point"
	upsertWFIFinding(t, env, wfiID, "user_instructions", instructions)

	// Re-read
	findings := getWFIFindings(t, env, wfiID)
	result := fetchInstructionsFromFindings(findings)
	if result != instructions {
		t.Fatalf("instructions corrupted during roundtrip:\nexpected: %q\ngot:      %q", instructions, result)
	}
}

// TestUserInstructionsNotOverwrittenByOrchestrationUpdate verifies that
// updating _orchestration status (e.g., marking completed) does not
// clobber user_instructions from findings.
func TestUserInstructionsNotOverwrittenByOrchestrationUpdate(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-5", "No clobber test")
	env.InitWorkflow(t, "UI-5")

	wfiID := env.GetWorkflowInstanceID(t, "UI-5", "test")

	// Step 1: Store instructions (like orchestrator Start does)
	upsertWFIFinding(t, env, wfiID, "user_instructions", "Implement the feature")
	upsertWFIFinding(t, env, wfiID, "_orchestration", map[string]interface{}{"status": "running"})

	// Step 2: Simulate orchestrator marking completed (individual upsert — no clobber)
	upsertWFIFinding(t, env, wfiID, "_orchestration", map[string]interface{}{"status": "completed"})

	// Step 3: Verify user_instructions survived the update
	findings := getWFIFindings(t, env, wfiID)
	result := fetchInstructionsFromFindings(findings)
	if result != "Implement the feature" {
		t.Fatalf("user_instructions lost after orchestration update, got %q", result)
	}
}

// fetchInstructionsFromFindings mirrors the spawner's fetchUserInstructions
// logic without needing a Spawner instance.
func fetchInstructionsFromFindings(findings map[string]json.RawMessage) string {
	if raw, ok := findings["user_instructions"]; ok {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil && str != "" {
			return str
		}
	}
	return "_No user instructions provided_"
}
