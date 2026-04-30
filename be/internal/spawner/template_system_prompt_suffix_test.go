package spawner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestExpandInjectable_SystemPromptSuffix verifies that the seeded
// system-prompt-suffix injectable expands ${VAR} substitutions and
// contains the completion contract content.
func TestExpandInjectable_SystemPromptSuffix(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	vars := map[string]string{
		"AGENT":     "implementor",
		"TICKET_ID": "TICKET-123",
	}

	body := sp.expandInjectable("system-prompt-suffix", vars)

	if body == "" {
		t.Fatal("system-prompt-suffix returned empty body; template missing from DB")
	}

	// Must contain the completion contract header
	if !strings.Contains(body, "Completion Contract") {
		t.Errorf("system-prompt-suffix body missing 'Completion Contract' header; got: %q", body)
	}

	// Must contain success and failure instructions
	if !strings.Contains(body, "nrflo agent finished") {
		t.Errorf("system-prompt-suffix body missing 'nrflo agent finished'; got: %q", body)
	}
	if !strings.Contains(body, "nrflo agent fail") {
		t.Errorf("system-prompt-suffix body missing 'nrflo agent fail'; got: %q", body)
	}

	// Must not contain unresolved placeholders (vars not relevant to suffix, but no stray ${...})
	if strings.Contains(body, "${AGENT}") || strings.Contains(body, "${TICKET_ID}") {
		t.Errorf("system-prompt-suffix has unresolved placeholder: %q", body)
	}
}

// TestExpandInjectable_SystemPromptSuffix_EmptyVarsNoPanel verifies that
// passing an empty/nil vars map to system-prompt-suffix does not panic and
// returns a non-empty body (no ${VAR} in the template itself).
func TestExpandInjectable_SystemPromptSuffix_EmptyVarsNoPanel(t *testing.T) {
	env := newSpawnerTestEnv(t)
	sp := env.newSpawner()

	body := sp.expandInjectable("system-prompt-suffix", nil)

	if body == "" {
		t.Fatal("system-prompt-suffix returned empty with nil vars; template missing from DB")
	}
	// No stray ${...} placeholders (the template itself doesn't use them)
	if strings.Contains(body, "${") {
		t.Errorf("system-prompt-suffix has unresolved placeholder with nil vars: %q", body)
	}
}

// TestLoadTemplate_SuffixReturnedFromLoadTemplate verifies that loadTemplate
// returns a non-empty suffix equal to the system-prompt-suffix expansion.
func TestLoadTemplate_SuffixReturnedFromLoadTemplate(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "SS-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main body only")

	sp := env.newSpawner()
	body, suffix, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if body == "" {
		t.Error("loadTemplate body should not be empty")
	}
	if suffix == "" {
		t.Error("loadTemplate suffix should not be empty (system-prompt-suffix seeded by migration)")
	}
	if !strings.Contains(suffix, "Completion Contract") {
		t.Errorf("loadTemplate suffix missing 'Completion Contract'; got: %q", suffix)
	}
}

// TestLoadTemplate_SuffixNotInBody verifies that the system-prompt-suffix
// content is returned separately and not auto-prepended into the prompt body.
func TestLoadTemplate_SuffixNotInBody(t *testing.T) {
	env := newSpawnerTestEnv(t)
	ticketID := "SN-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Only the real body here")

	sp := env.newSpawner()
	body, suffix, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet", "", "", nil)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if suffix == "" {
		t.Skip("suffix empty; system-prompt-suffix not seeded")
	}

	// The suffix content must not appear inside the body
	if strings.Contains(body, "Completion Contract") {
		t.Errorf("loadTemplate body should not contain suffix 'Completion Contract'; body: %q", body)
	}
}
