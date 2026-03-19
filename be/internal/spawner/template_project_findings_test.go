package spawner

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"

	"github.com/google/uuid"
)

// TestExpandProjectFindings_SingleKey tests that #{PROJECT_FINDINGS:key} is replaced
// with the stored value from the project_findings table.
func TestExpandProjectFindings_SingleKey(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert project finding
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "architecture",
		Value: "microservices with event-driven communication",
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	sp := env.newSpawner()
	template := "Project architecture: #{PROJECT_FINDINGS:architecture}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	expected := "Project architecture: microservices with event-driven communication"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestExpandProjectFindings_MultipleKeys tests that #{PROJECT_FINDINGS:k1,k2} is replaced
// with formatted key:value output for multiple keys.
func TestExpandProjectFindings_MultipleKeys(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert multiple project findings
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.AddBulk(env.project, &types.ProjectFindingsAddBulkRequest{
		KeyValues: map[string]string{
			"architecture": "microservices",
			"conventions":  "use snake_case for file names",
		},
	})
	if err != nil {
		t.Fatalf("failed to add project findings: %v", err)
	}

	sp := env.newSpawner()
	template := "## Project Context\n#{PROJECT_FINDINGS:architecture,conventions}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	// Verify both keys are present in the output (note: formatValue adds leading space)
	if !strings.Contains(result, "architecture: microservices") {
		t.Error("expected architecture key in output")
	}
	if !strings.Contains(result, "conventions: use snake_case for file names") {
		t.Error("expected conventions key in output")
	}
}

// TestExpandProjectFindings_MissingKey tests that missing keys produce a placeholder.
func TestExpandProjectFindings_MissingKey(t *testing.T) {
	env := newSpawnerTestEnv(t)

	sp := env.newSpawner()
	template := "Context: #{PROJECT_FINDINGS:nonexistent}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	expected := "Context: _No project finding for key 'nonexistent'_"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestExpandProjectFindings_PartialMissing tests that multi-key expansion with some
// missing keys returns found values and placeholders for missing keys.
func TestExpandProjectFindings_PartialMissing(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert only k1
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "k1",
		Value: "value1",
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	sp := env.newSpawner()
	template := "#{PROJECT_FINDINGS:k1,k2}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	// Verify k1 is present and k2 has placeholder
	if !strings.Contains(result, "k1: value1") {
		t.Error("expected k1 value in output")
	}
	if !strings.Contains(result, "_No project finding for key 'k2'_") {
		t.Error("expected placeholder for k2")
	}
}

// TestExpandProjectFindings_NoPatterns tests that templates without #{PROJECT_FINDINGS:...}
// patterns pass through unchanged.
func TestExpandProjectFindings_NoPatterns(t *testing.T) {
	env := newSpawnerTestEnv(t)

	sp := env.newSpawner()
	template := "This is a plain template with no project findings patterns."
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	if result != template {
		t.Errorf("template was modified when it should pass through unchanged")
	}
}

// TestExpandProjectFindings_WithWhitespace tests that keys with whitespace are trimmed.
func TestExpandProjectFindings_WithWhitespace(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert project finding
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "mykey",
		Value: "myvalue",
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	sp := env.newSpawner()
	// Note the whitespace around keys
	template := "#{PROJECT_FINDINGS: mykey , mykey }"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	// Both should resolve to the same value (whitespace trimmed)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.Contains(line, "mykey: myvalue") {
			t.Errorf("line %d: expected mykey: myvalue, got %q", i, line)
		}
	}
}

// TestExpandProjectFindings_MultiplePatterns tests that multiple #{PROJECT_FINDINGS:...}
// patterns in the same template are all expanded.
func TestExpandProjectFindings_MultiplePatterns(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert project findings
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.AddBulk(env.project, &types.ProjectFindingsAddBulkRequest{
		KeyValues: map[string]string{
			"arch": "microservices",
			"lang": "golang",
		},
	})
	if err != nil {
		t.Fatalf("failed to add project findings: %v", err)
	}

	sp := env.newSpawner()
	template := "Architecture: #{PROJECT_FINDINGS:arch}\nLanguage: #{PROJECT_FINDINGS:lang}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	if !strings.Contains(result, "Architecture: microservices") {
		t.Error("expected arch to be expanded")
	}
	if !strings.Contains(result, "Language: golang") {
		t.Error("expected lang to be expanded")
	}
}

// TestLoadTemplate_ProjectFindingsExpansion tests that #{PROJECT_FINDINGS:key} is expanded
// when present in an agent definition prompt template.
func TestLoadTemplate_ProjectFindingsExpansion(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "PF-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition with PROJECT_FINDINGS variable
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:         "analyzer",
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "sonnet",
		Timeout:    3600,
		Prompt:     "Agent: ${AGENT}\n\n## Project Context\n#{PROJECT_FINDINGS:architecture}\n\nProceed with analysis.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	// Insert project finding
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err = pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "architecture",
		Value: "event-driven microservices",
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	sp := env.newSpawner()
	template, err := sp.loadTemplate("analyzer", ticketID, env.project, "parent-1", "child-1", "test", "claude:sonnet", "investigation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// Verify PROJECT_FINDINGS was expanded
	if strings.Contains(template, "#{PROJECT_FINDINGS:architecture}") {
		t.Error("#{PROJECT_FINDINGS:architecture} variable was not expanded")
	}
	if !strings.Contains(template, "event-driven microservices") {
		t.Error("expected project finding value in expanded template")
	}
	if !strings.Contains(template, "Agent: analyzer") {
		t.Error("expected other variables to still be expanded")
	}
}

// TestLoadTemplate_MixedPatterns tests that templates with both #{FINDINGS:agent} and
// #{PROJECT_FINDINGS:key} patterns have both resolved independently.
func TestLoadTemplate_MixedPatterns(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "MIX-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition with both patterns
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:         "implementor",
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "opus",
		Timeout:    3600,
		Prompt:     "## Agent Findings\n#{FINDINGS:analyzer}\n\n## Project Context\n#{PROJECT_FINDINGS:architecture}\n\nImplement the feature.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	// Insert project finding
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err = pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "architecture",
		Value: "layered architecture",
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	// Note: We don't create any agent sessions, so #{FINDINGS:analyzer} should produce the placeholder

	sp := env.newSpawner()
	template, err := sp.loadTemplate("implementor", ticketID, env.project, "parent-1", "child-1", "test", "claude:opus", "implementation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// Verify both patterns were processed
	if strings.Contains(template, "#{FINDINGS:analyzer}") {
		t.Error("#{FINDINGS:analyzer} variable was not expanded")
	}
	if strings.Contains(template, "#{PROJECT_FINDINGS:architecture}") {
		t.Error("#{PROJECT_FINDINGS:architecture} variable was not expanded")
	}

	// FINDINGS should have placeholder (no agent sessions exist)
	if !strings.Contains(template, "_No findings yet available from analyzer_") {
		t.Error("expected placeholder for missing agent findings")
	}

	// PROJECT_FINDINGS should have the value
	if !strings.Contains(template, "layered architecture") {
		t.Error("expected project finding value in expanded template")
	}
}

// TestLoadTemplate_NoProjectFindings tests that templates without #{PROJECT_FINDINGS:...}
// patterns are unaffected and don't trigger DB queries.
func TestLoadTemplate_NoProjectFindings(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "NPF-" + uuid.New().String()[:6]

	// Create ticket and workflow
	env.initWorkflow(t, ticketID)

	// Create agent definition WITHOUT PROJECT_FINDINGS variable
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:         "analyzer",
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "sonnet",
		Timeout:    3600,
		Prompt:     "Agent: ${AGENT}\nTicket: ${TICKET_ID}\n\nProceed with analysis.",
	})
	if err != nil {
		t.Fatalf("failed to create agent definition: %v", err)
	}

	sp := env.newSpawner()
	template, err := sp.loadTemplate("analyzer", ticketID, env.project, "parent-1", "child-1", "test", "claude:sonnet", "investigation", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	// Verify template was expanded normally without PROJECT_FINDINGS patterns
	if !strings.Contains(template, "Agent: analyzer") {
		t.Error("expected AGENT variable to be expanded")
	}
	if !strings.Contains(template, "Ticket: "+ticketID) {
		t.Error("expected TICKET_ID variable to be expanded")
	}
}

// TestExpandProjectFindings_ComplexValue tests that project findings with complex JSON
// values (objects, arrays) are formatted correctly.
func TestExpandProjectFindings_ComplexValue(t *testing.T) {
	env := newSpawnerTestEnv(t)

	// Insert project finding with JSON object value
	pfSvc := service.NewProjectFindingsService(env.pool, clock.Real())
	err := pfSvc.Add(env.project, &types.ProjectFindingsAddRequest{
		Key:   "config",
		Value: `{"timeout": 30, "retries": 3}`,
	})
	if err != nil {
		t.Fatalf("failed to add project finding: %v", err)
	}

	sp := env.newSpawner()
	template := "Configuration: #{PROJECT_FINDINGS:config}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	// The complex value should be formatted (exact format depends on formatValue implementation)
	// Just verify it contains the key data
	if !strings.Contains(result, "timeout") || !strings.Contains(result, "30") {
		t.Errorf("expected config values in output, got %q", result)
	}
}

// TestExpandProjectFindings_AllMissingKeys tests that multi-key request with all keys missing
// produces placeholders for each key.
func TestExpandProjectFindings_AllMissingKeys(t *testing.T) {
	env := newSpawnerTestEnv(t)

	sp := env.newSpawner()
	template := "#{PROJECT_FINDINGS:missing1,missing2}"
	result, err := sp.expandProjectFindings(template, env.project)
	if err != nil {
		t.Fatalf("expandProjectFindings failed: %v", err)
	}

	// Should have placeholders for both missing keys
	if !strings.Contains(result, "_No project finding for key 'missing1'_") {
		t.Error("expected placeholder for missing1")
	}
	if !strings.Contains(result, "_No project finding for key 'missing2'_") {
		t.Error("expected placeholder for missing2")
	}
}
