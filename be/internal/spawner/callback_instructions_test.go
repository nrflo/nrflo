package spawner

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestFetchCallbackInstructions_Present tests that callback instructions are formatted
// correctly when _callback metadata is present in workflow instance findings.
func TestFetchCallbackInstructions_Present(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store callback metadata
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        1,
			"instructions": "The authentication middleware is not checking token expiry correctly. Fix the validation logic.",
			"from_layer":   3,
			"from_agent":   "qa-verifier",
		},
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, ticketID, "test", "")

	if gotInstr != "The authentication middleware is not checking token expiry correctly. Fix the validation logic." {
		t.Errorf("expected raw instructions, got %q", gotInstr)
	}
	if gotFrom != "qa-verifier" {
		t.Errorf("expected from_agent 'qa-verifier', got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_Missing tests that the placeholder is returned
// when no _callback key exists in workflow instance findings.
func TestFetchCallbackInstructions_Missing(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// No _callback in findings
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"some_other_key": "value",
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, ticketID, "test", "")

	if gotInstr != "" {
		t.Errorf("expected empty instructions, got %q", gotInstr)
	}
	if gotFrom != "" {
		t.Errorf("expected empty fromAgent, got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_EmptyInstructions tests that empty values are returned
// when _callback is present but instructions field is empty.
func TestFetchCallbackInstructions_EmptyInstructions(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store _callback with empty instructions
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        1,
			"instructions": "",
			"from_layer":   3,
			"from_agent":   "qa-verifier",
		},
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, ticketID, "test", "")

	if gotInstr != "" {
		t.Errorf("expected empty instructions, got %q", gotInstr)
	}
	if gotFrom != "" {
		t.Errorf("expected empty fromAgent, got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_NoWorkflow tests that empty values are returned
// when the workflow instance doesn't exist.
func TestFetchCallbackInstructions_NoWorkflow(t *testing.T) {
	env := newSpawnerTestEnv(t)

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, "NONEXISTENT", "test", "")

	if gotInstr != "" {
		t.Errorf("expected empty instructions, got %q", gotInstr)
	}
	if gotFrom != "" {
		t.Errorf("expected empty fromAgent, got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_ProjectScope tests that callback instructions work
// for project-scoped workflows (empty ticketID).
func TestFetchCallbackInstructions_ProjectScope(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Update test workflow to project scope
	_, err := env.pool.Exec(`UPDATE workflows SET scope_type = 'project' WHERE id = 'test' AND LOWER(project_id) = LOWER(?)`, env.project)
	if err != nil {
		t.Fatalf("failed to update workflow scope: %v", err)
	}

	// Create project-scoped workflow instance
	var wfiID string
	err = env.pool.QueryRow(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, '', 'test', 'active', 'project', '{}', 0, datetime('now'), datetime('now'))
		RETURNING id`, uuid.New().String(), env.project).Scan(&wfiID)
	if err != nil {
		t.Fatalf("failed to create project workflow instance: %v", err)
	}

	// Store callback metadata
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        0,
			"instructions": "Project-level callback instructions",
			"from_layer":   1,
			"from_agent":   "project-verifier",
		},
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, "", "test", "")

	if gotInstr != "Project-level callback instructions" {
		t.Errorf("expected raw instructions, got %q", gotInstr)
	}
	if gotFrom != "project-verifier" {
		t.Errorf("expected from_agent 'project-verifier', got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_NoFromAgent tests that callback instructions
// work even when from_agent is missing from metadata.
func TestFetchCallbackInstructions_NoFromAgent(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store callback metadata without from_agent
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        1,
			"instructions": "Fix the implementation",
			"from_layer":   2,
		},
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, ticketID, "test", "")

	if gotInstr != "Fix the implementation" {
		t.Errorf("expected raw instructions, got %q", gotInstr)
	}
	if gotFrom != "" {
		t.Errorf("expected empty fromAgent when from_agent is missing, got %q", gotFrom)
	}
}

// TestFetchCallbackInstructions_InvalidCallbackType tests that the placeholder
// is returned when _callback is not a map.
func TestFetchCallbackInstructions_InvalidCallbackType(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)

	// Store _callback as a string instead of map
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": "invalid type",
	})

	sp := env.newSpawner()
	gotInstr, gotFrom := sp.fetchCallbackRaw(env.project, ticketID, "test", "")

	if gotInstr != "" {
		t.Errorf("expected empty instructions, got %q", gotInstr)
	}
	if gotFrom != "" {
		t.Errorf("expected empty fromAgent, got %q", gotFrom)
	}
}

// TestLoadTemplate_CallbackInstructionsExpansion tests that ${CALLBACK_INSTRUCTIONS}
// is expanded when present in the template.
func TestLoadTemplate_CallbackInstructionsExpansion(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition with CALLBACK_INSTRUCTIONS variable
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:          "analyzer",
		ProjectID:   env.project,
		WorkflowID:  "test",
		Model:       "sonnet",
		Timeout:     3600,
		Prompt:      "Agent: ${AGENT}\nTicket: ${TICKET_ID}\n\n${CALLBACK_INSTRUCTIONS}\n\nProceed with analysis.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	// Store callback metadata
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        0,
			"instructions": "Fix the validation logic in auth middleware",
			"from_layer":   2,
			"from_agent":   "verifier",
		},
	})

	sp := env.newSpawner()
	template, err := sp.loadTemplate("analyzer", ticketID, env.project, "parent-1", "child-1", "test", "claude:sonnet", "investigation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// Verify ${CALLBACK_INSTRUCTIONS} was stripped and callback injectable prepended
	if strings.Contains(template, "${CALLBACK_INSTRUCTIONS}") {
		t.Error("${CALLBACK_INSTRUCTIONS} variable was not stripped")
	}
	if !strings.Contains(template, "## Callback Instructions") {
		t.Error("expected callback injectable header prepended")
	}
	if !strings.Contains(template, "Fix the validation logic in auth middleware") {
		t.Error("expected callback instructions content in prepended block")
	}
	if !strings.Contains(template, "**verifier**") {
		t.Error("expected from_agent in prepended callback block")
	}
	if !strings.Contains(template, "Agent: analyzer") {
		t.Error("expected other variables to still be expanded")
	}
	if !strings.Contains(template, "Ticket: "+ticketID) {
		t.Error("expected ticket ID to be expanded")
	}
}

// TestLoadTemplate_NoCallbackInstructions tests that templates without
// ${CALLBACK_INSTRUCTIONS} are unaffected.
func TestLoadTemplate_NoCallbackInstructions(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition WITHOUT CALLBACK_INSTRUCTIONS variable
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:          "analyzer",
		ProjectID:   env.project,
		WorkflowID:  "test",
		Model:       "sonnet",
		Timeout:     3600,
		Prompt:      "Agent: ${AGENT}\nTicket: ${TICKET_ID}\n\nProceed with analysis.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	// Store callback metadata (should not affect template without the variable)
	wfiID := env.getWfiID(t, ticketID)
	env.setFindings(t, wfiID, map[string]interface{}{
		"_callback": map[string]interface{}{
			"level":        0,
			"instructions": "This should not appear",
			"from_agent":   "verifier",
		},
	})

	sp := env.newSpawner()
	template, err := sp.loadTemplate("analyzer", ticketID, env.project, "parent-1", "child-1", "test", "claude:sonnet", "investigation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// Callback is now always prepended when data exists (regardless of template content)
	if !strings.Contains(template, "## Callback Instructions") {
		t.Error("expected callback injectable to be prepended")
	}
	if !strings.Contains(template, "This should not appear") {
		t.Error("expected callback instructions content in prepended block")
	}
}

// TestLoadTemplate_CallbackInstructionsDefault tests that ${CALLBACK_INSTRUCTIONS}
// expands to the default placeholder when no callback metadata exists.
func TestLoadTemplate_CallbackInstructionsDefault(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "CB-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition with CALLBACK_INSTRUCTIONS variable
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:          "analyzer",
		ProjectID:   env.project,
		WorkflowID:  "test",
		Model:       "sonnet",
		Timeout:     3600,
		Prompt:      "Agent: ${AGENT}\n\n${CALLBACK_INSTRUCTIONS}\n\nProceed.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	// No callback metadata - should get default placeholder
	sp := env.newSpawner()
	template, err := sp.loadTemplate("analyzer", ticketID, env.project, "parent-1", "child-1", "test", "claude:sonnet", "investigation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// ${CALLBACK_INSTRUCTIONS} stripped, no prepend when no callback data
	if strings.Contains(template, "${CALLBACK_INSTRUCTIONS}") {
		t.Error("${CALLBACK_INSTRUCTIONS} variable was not stripped")
	}
	if strings.Contains(template, "_No callback instructions_") {
		t.Error("legacy placeholder should be stripped")
	}
	if strings.Contains(template, "## Callback Instructions") {
		t.Error("should not have callback header when no callback data exists")
	}
}
