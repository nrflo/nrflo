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

// createAgentDef inserts a project-scoped agent definition for the "test" workflow.
func createAgentDef(t *testing.T, env *spawnerTestEnv, agentID, prompt string) {
	t.Helper()
	database, err := db.Open(env.dbPath)
	if err != nil {
		t.Fatalf("createAgentDef: open db: %v", err)
	}
	defer database.Close()

	adRepo := repo.NewAgentDefinitionRepo(database, clock.Real())
	err = adRepo.Create(&model.AgentDefinition{
		ID:         agentID,
		ProjectID:  env.project,
		WorkflowID: "test",
		Model:      "sonnet",
		Timeout:    3600,
		Prompt:     prompt,
	})
	if err != nil {
		t.Fatalf("createAgentDef: %v", err)
	}
}

// createSystemAgentDef inserts a global system agent definition.
// Deletes any existing row first (e.g. from migration seed data).
func createSystemAgentDef(t *testing.T, env *spawnerTestEnv, agentID, prompt string) {
	t.Helper()
	svc := service.NewSystemAgentDefinitionService(env.pool, clock.Real())
	// Remove seeded row if present so Create doesn't fail on duplicate.
	_ = svc.Delete(agentID)
	_, err := svc.Create(&types.SystemAgentDefCreateRequest{
		ID:     agentID,
		Model:  "sonnet",
		Prompt: prompt,
	})
	if err != nil {
		t.Fatalf("createSystemAgentDef: %v", err)
	}
}

// ---- ExtraVars expansion ----

func TestLoadTemplate_ExtraVars_BasicExpansion(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Branch: ${BRANCH_NAME}, Error: ${MERGE_ERROR}")

	sp := env.newSpawner()
	extraVars := map[string]string{
		"BRANCH_NAME": "feat/my-branch",
		"MERGE_ERROR": "conflict in main.go",
	}
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", extraVars)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "feat/my-branch") {
		t.Errorf("BRANCH_NAME not expanded; got: %s", result)
	}
	if !strings.Contains(result, "conflict in main.go") {
		t.Errorf("MERGE_ERROR not expanded; got: %s", result)
	}
}

func TestLoadTemplate_ExtraVars_MultipleVars(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "B=${BRANCH_NAME} D=${DEFAULT_BRANCH} E=${MERGE_ERROR}")

	sp := env.newSpawner()
	extraVars := map[string]string{
		"BRANCH_NAME":    "feature/x",
		"DEFAULT_BRANCH": "main",
		"MERGE_ERROR":    "merge failed",
	}
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", extraVars)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "B=feature/x") {
		t.Errorf("BRANCH_NAME not expanded; got: %s", result)
	}
	if !strings.Contains(result, "D=main") {
		t.Errorf("DEFAULT_BRANCH not expanded; got: %s", result)
	}
	if !strings.Contains(result, "E=merge failed") {
		t.Errorf("MERGE_ERROR not expanded; got: %s", result)
	}
}

func TestLoadTemplate_ExtraVars_Nil_NoPanic(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Static template with no extra vars")

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate with nil extraVars failed: %v", err)
	}
	if !strings.HasPrefix(result, "Static template with no extra vars") {
		t.Errorf("expected template starting with 'Static template with no extra vars', got: %s", result)
	}
}

func TestLoadTemplate_ExtraVars_EmptyMap_NoPanic(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Static template")

	sp := env.newSpawner()
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", map[string]string{})
	if err != nil {
		t.Fatalf("loadTemplate with empty extraVars failed: %v", err)
	}
	if !strings.HasPrefix(result, "Static template") {
		t.Errorf("expected template starting with 'Static template', got: %s", result)
	}
}

// TestLoadTemplate_ExtraVars_StandardVarsRunFirst verifies that standard ${VAR}
// substitution happens before ExtraVars. An ExtraVars key matching a standard var
// name (e.g. "AGENT") does not replace the already-expanded value.
func TestLoadTemplate_ExtraVars_StandardVarsRunFirst(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "agent=${AGENT}")

	sp := env.newSpawner()
	// ExtraVars key "AGENT" – but ${AGENT} is already replaced before ExtraVars runs.
	extraVars := map[string]string{"AGENT": "overridden"}
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", extraVars)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	// Standard expansion replaced ${AGENT} → "analyzer"; ExtraVars had no placeholder left.
	if !strings.Contains(result, "agent=analyzer") {
		t.Errorf("expected standard ${AGENT} to expand to 'analyzer', got: %s", result)
	}
}

func TestLoadTemplate_ExtraVars_UnusedVarsIgnored(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Hello world")

	sp := env.newSpawner()
	// ExtraVars present but template has no matching placeholders — no effect.
	extraVars := map[string]string{
		"BRANCH_NAME":    "unused",
		"DEFAULT_BRANCH": "also-unused",
	}
	result, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", extraVars)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.HasPrefix(result, "Hello world") {
		t.Errorf("expected template starting with 'Hello world', got: %s", result)
	}
}

// ---- System agent definition fallback ----

func TestLoadPromptContent_SystemDefFallback(t *testing.T) {
	env := newSpawnerTestEnv(t)
	// Only system def — no project-scoped def.
	createSystemAgentDef(t, env, "conflict-resolver", "Resolve merge conflicts for ${BRANCH_NAME}")

	sp := env.newSpawner()
	prompt, err := sp.loadPromptContent("conflict-resolver", env.project, "test")
	if err != nil {
		t.Fatalf("expected fallback to system def; got error: %v", err)
	}
	if prompt != "Resolve merge conflicts for ${BRANCH_NAME}" {
		t.Errorf("expected system def prompt, got: %s", prompt)
	}
}

func TestLoadPromptContent_ProjectDefPriority(t *testing.T) {
	env := newSpawnerTestEnv(t)
	// Both project-scoped and system def exist with different prompts.
	createAgentDef(t, env, "conflict-resolver", "project-scoped prompt")
	createSystemAgentDef(t, env, "conflict-resolver", "system-level prompt")

	sp := env.newSpawner()
	prompt, err := sp.loadPromptContent("conflict-resolver", env.project, "test")
	if err != nil {
		t.Fatalf("loadPromptContent failed: %v", err)
	}
	if prompt != "project-scoped prompt" {
		t.Errorf("expected project-scoped def to win, got: %s", prompt)
	}
}

func TestLoadPromptContent_BothMissing_ReturnsError(t *testing.T) {
	env := newSpawnerTestEnv(t)
	// Neither project-scoped nor system def exists.

	sp := env.newSpawner()
	_, err := sp.loadPromptContent("nonexistent-agent", env.project, "test")
	if err == nil {
		t.Fatal("expected error when both lookups fail")
	}
	if !strings.Contains(err.Error(), "agent definition not found") {
		t.Errorf("expected 'agent definition not found' in error, got: %v", err)
	}
}

func TestLoadPromptContent_ProjectDefEmptyPrompt_ReturnsError(t *testing.T) {
	env := newSpawnerTestEnv(t)
	createAgentDef(t, env, "empty-agent", "")

	sp := env.newSpawner()
	_, err := sp.loadPromptContent("empty-agent", env.project, "test")
	if err == nil {
		t.Fatal("expected error for project def with empty prompt")
	}
	if !strings.Contains(err.Error(), "empty prompt") {
		t.Errorf("expected 'empty prompt' in error, got: %v", err)
	}
}

func TestLoadPromptContent_SystemDefEmptyPrompt_ReturnsError(t *testing.T) {
	env := newSpawnerTestEnv(t)
	// Only system def, but it has an empty prompt.
	createSystemAgentDef(t, env, "empty-sys-agent", "")

	sp := env.newSpawner()
	_, err := sp.loadPromptContent("empty-sys-agent", env.project, "test")
	if err == nil {
		t.Fatal("expected error for system def with empty prompt")
	}
	if !strings.Contains(err.Error(), "empty prompt") {
		t.Errorf("expected 'empty prompt' in error, got: %v", err)
	}
}

// TestLoadTemplate_SystemDefFallback_WithExtraVars tests the full flow:
// system def is loaded as fallback, then ExtraVars are expanded.
func TestLoadTemplate_SystemDefFallback_WithExtraVars(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "EV-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	// No project-scoped def — only system def.
	createSystemAgentDef(t, env, "conflict-resolver", "Fix ${BRANCH_NAME} against ${DEFAULT_BRANCH}: ${MERGE_ERROR}")

	sp := env.newSpawner()
	extraVars := map[string]string{
		"BRANCH_NAME":    "feat/my-feature",
		"DEFAULT_BRANCH": "main",
		"MERGE_ERROR":    "CONFLICT in main.go",
	}
	result, err := sp.loadTemplate("conflict-resolver", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", "", extraVars)
	if err != nil {
		t.Fatalf("loadTemplate with system def fallback + extraVars failed: %v", err)
	}
	if !strings.Contains(result, "feat/my-feature") {
		t.Errorf("BRANCH_NAME not expanded; got: %s", result)
	}
	if !strings.Contains(result, "main") {
		t.Errorf("DEFAULT_BRANCH not expanded; got: %s", result)
	}
	if !strings.Contains(result, "CONFLICT in main.go") {
		t.Errorf("MERGE_ERROR not expanded; got: %s", result)
	}
}
