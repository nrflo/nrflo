package integration

import (
	"encoding/json"
	"testing"

	"be/internal/repo"
	"be/internal/types"
)

// TestUserInstructionsEndToEnd validates the full flow: orchestrator stores
// user instructions as a direct string in workflow instance findings, and the
// spawner can retrieve them as a plain string (not a nested map).
func TestUserInstructionsEndToEnd(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-1", "User instructions test")
	env.InitWorkflow(t, "UI-1")

	wfiID := env.GetWorkflowInstanceID(t, "UI-1", "test")
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool)

	// --- Simulate what the orchestrator does (orchestrator.go:152-161) ---
	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-1", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	findings := wi.GetFindings()
	findings["user_instructions"] = "Build the login page with OAuth support"
	findings["_orchestration"] = map[string]interface{}{"status": "running"}
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("failed to marshal findings: %v", err)
	}
	if err := wfiRepo.UpdateFindings(wfiID, string(findingsJSON)); err != nil {
		t.Fatalf("failed to update findings: %v", err)
	}

	// --- Simulate what the spawner does (spawner.go:855-867) ---
	wi2, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-1", "test")
	if err != nil {
		t.Fatalf("spawner failed to fetch workflow instance: %v", err)
	}
	retrievedFindings := wi2.GetFindings()

	// The spawner does: findings["user_instructions"].(string)
	raw, ok := retrievedFindings["user_instructions"]
	if !ok {
		t.Fatal("user_instructions key missing from findings")
	}
	str, ok := raw.(string)
	if !ok {
		t.Fatalf("user_instructions is not a string, got %T: %v", raw, raw)
	}
	if str != "Build the login page with OAuth support" {
		t.Fatalf("expected 'Build the login page with OAuth support', got %q", str)
	}

	// Verify orchestration status is also present
	orch, ok := retrievedFindings["_orchestration"].(map[string]interface{})
	if !ok {
		t.Fatal("_orchestration key missing or wrong type")
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
	apiFindings, ok := status["findings"].(map[string]interface{})
	if !ok {
		t.Fatalf("findings not in status response, got %T", status["findings"])
	}
	if apiFindings["user_instructions"] != "Build the login page with OAuth support" {
		t.Fatalf("API returned wrong instructions: %v", apiFindings["user_instructions"])
	}
}

// TestUserInstructionsMissing verifies that when no instructions are stored,
// the spawner logic returns the placeholder string.
func TestUserInstructionsMissing(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "UI-2", "No instructions test")
	env.InitWorkflow(t, "UI-2")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool)
	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-2", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}

	findings := wi.GetFindings()

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
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool)

	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-3", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	findings := wi.GetFindings()
	findings["user_instructions"] = ""
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON))

	// Re-read and check spawner logic
	wi2, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-3", "test")
	if err != nil {
		t.Fatalf("failed to re-read: %v", err)
	}
	result := fetchInstructionsFromFindings(wi2.GetFindings())
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
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool)

	instructions := "Use \"double quotes\" and 'single quotes'.\nAlso newlines\tand tabs.\n## Markdown heading\n- bullet point"

	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-4", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	findings := wi.GetFindings()
	findings["user_instructions"] = instructions
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON))

	// Re-read
	wi2, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-4", "test")
	if err != nil {
		t.Fatalf("failed to re-read: %v", err)
	}
	result := fetchInstructionsFromFindings(wi2.GetFindings())
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
	wfiRepo := repo.NewWorkflowInstanceRepo(env.Pool)

	// Step 1: Store instructions (like orchestrator Start does)
	wi, err := wfiRepo.GetByTicketAndWorkflow(env.ProjectID, "UI-5", "test")
	if err != nil {
		t.Fatalf("failed to get workflow instance: %v", err)
	}
	findings := wi.GetFindings()
	findings["user_instructions"] = "Implement the feature"
	findings["_orchestration"] = map[string]interface{}{"status": "running"}
	findingsJSON, _ := json.Marshal(findings)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON))

	// Step 2: Simulate orchestrator marking completed (read-modify-write)
	wi2, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("failed to re-read: %v", err)
	}
	findings2 := wi2.GetFindings()
	findings2["_orchestration"] = map[string]interface{}{"status": "completed"}
	findingsJSON2, _ := json.Marshal(findings2)
	wfiRepo.UpdateFindings(wfiID, string(findingsJSON2))

	// Step 3: Verify user_instructions survived the update
	wi3, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("failed to final read: %v", err)
	}
	result := fetchInstructionsFromFindings(wi3.GetFindings())
	if result != "Implement the feature" {
		t.Fatalf("user_instructions lost after orchestration update, got %q", result)
	}
}

// fetchInstructionsFromFindings mirrors the spawner's fetchUserInstructions
// logic (spawner.go:861-867) without needing a Spawner instance.
func fetchInstructionsFromFindings(findings map[string]interface{}) string {
	if instructions, ok := findings["user_instructions"]; ok {
		if str, ok := instructions.(string); ok && str != "" {
			return str
		}
	}
	return "_No user instructions provided_"
}
