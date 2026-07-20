package spawner

// Tests for the readonly "delegation-guidance" injectable (template_injectable.go's
// appendDelegationGuidance): appended to the rendered system prompt at every
// prompt-assembly seam (api, cli_interactive) only when the def's effective
// tool specs include "delegate"; byte-identical to pre-feature behavior
// otherwise. Also proves tier-t1-executor + the injectable compose without
// duplicating the delegation how-to.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"be/internal/db"
)

// ── api mode ─────────────────────────────────────────────────────────────

// TestAppendDelegationGuidance_APIMode_DelegateInTools verifies prep.apiSystem
// carries the delegation-guidance injectable's anchors when the def's tools
// CSV includes "delegate".
func TestAppendDelegationGuidance_APIMode_DelegateInTools(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDefWithTools(t, env, "impl", "sonnet-5", "delegate,agent_finished")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if !strings.Contains(prep.apiSystem, "get_delegation") {
		t.Errorf("prep.apiSystem missing %q anchor; got %q", "get_delegation", prep.apiSystem)
	}
	if !strings.Contains(prep.apiSystem, "extractor") {
		t.Errorf("prep.apiSystem missing %q anchor; got %q", "extractor", prep.apiSystem)
	}
}

// TestAppendDelegationGuidance_APIMode_NoDelegate verifies that a def without
// "delegate" in its tools CSV gets a byte-identical prompt to before the
// feature: no guidance sentinel, and prep.apiSystem equals the plain
// api-system-prompt + suffix join.
func TestAppendDelegationGuidance_APIMode_NoDelegate(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDef(t, env, "impl", "sonnet-5") // tools='agent_finished', no delegate

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if strings.Contains(prep.apiSystem, "get_delegation") {
		t.Errorf("prep.apiSystem contains delegation guidance sentinel %q; want absent: %q", "get_delegation", prep.apiSystem)
	}

	pool := db.WrapAsPool(env.database)
	want := renderInjectable(context.Background(), pool, "api-system-prompt", nil) +
		"\n\n" + renderInjectable(context.Background(), pool, "system-prompt-suffix", nil)
	if prep.apiSystem != want {
		t.Errorf("prep.apiSystem = %q, want byte-identical %q", prep.apiSystem, want)
	}
}

// ── cli_interactive mode ────────────────────────────────────────────────

// TestAppendDelegationGuidance_CLIInteractive_DelegateInTools verifies the
// Claude system-prompt suffix file gets the guidance appended when the def's
// tools CSV includes "delegate".
func TestAppendDelegationGuidance_CLIInteractive_DelegateInTools(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertCLIAgentDefWithTools(t, env, "impl", "sonnet-5", "delegate,agent_finished")

	_, prep, err := cliMCPSpawner(env).prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile == "" {
		t.Fatal("prep.suffixFile is empty, want a written suffix file")
	}
	t.Cleanup(func() { os.Remove(prep.suffixFile) })

	body, err := os.ReadFile(prep.suffixFile)
	if err != nil {
		t.Fatalf("ReadFile(suffixFile): %v", err)
	}
	if !strings.Contains(string(body), "get_delegation") {
		t.Errorf("suffixFile body missing %q anchor; got %q", "get_delegation", string(body))
	}
}

// TestAppendDelegationGuidance_CLIInteractive_NoDelegate verifies the Claude
// suffix file stays byte-identical to the plain rendered system-prompt-suffix
// injectable when "delegate" is absent from the def's tools.
func TestAppendDelegationGuidance_CLIInteractive_NoDelegate(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertCLIAgentDefWithTools(t, env, "impl", "sonnet-5", "agent_finished")

	_, prep, err := cliMCPSpawner(env).prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "impl",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile == "" {
		t.Fatal("prep.suffixFile is empty, want a written suffix file")
	}
	t.Cleanup(func() { os.Remove(prep.suffixFile) })

	body, err := os.ReadFile(prep.suffixFile)
	if err != nil {
		t.Fatalf("ReadFile(suffixFile): %v", err)
	}

	pool := db.WrapAsPool(env.database)
	want := renderInjectable(context.Background(), pool, "system-prompt-suffix", nil)
	if string(body) != want {
		t.Errorf("suffixFile body = %q, want byte-identical %q", string(body), want)
	}
}

// ── composition with tier templates ─────────────────────────────────────

// insertAPIAgentDefWithSystemTemplateAndTools inserts an execution_mode='api'
// agent_definition row carrying both a system_template_id override and an
// explicit tools CSV, so tests can exercise a tier template + the delegation
// guidance composing together.
func insertAPIAgentDefWithSystemTemplateAndTools(t *testing.T, env *contextSaveTestEnv, agentID, model, systemTemplateID, tools string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, system_template_id, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', 'api', ?, ?, ?, ?)`,
		agentID, env.projectID, model, tools, systemTemplateID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

// TestAppendDelegationGuidance_ComposesWithTierT1Executor_NoDuplication
// verifies a def with system_template_id='tier-t1-executor' AND "delegate" in
// its tools (api mode) gets both the tier role framing and the delegation
// guidance exactly once each — proving the migration's trim of tier-t1's own
// delegation how-to (now owned by the injectable) prevents duplication.
func TestAppendDelegationGuidance_ComposesWithTierT1Executor_NoDuplication(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()
	insertAPIAgentDefWithSystemTemplateAndTools(t, env, "impl", "sonnet-5", "tier-t1-executor", "delegate,agent_finished")

	prep := prepareAPISpawn(t, newAPIModeSpawner(env), env)

	if got := strings.Count(prep.apiSystem, "## Role: T1 Executor"); got != 1 {
		t.Errorf("count(%q) = %d, want 1; prep.apiSystem = %q", "## Role: T1 Executor", got, prep.apiSystem)
	}
	if got := strings.Count(prep.apiSystem, "get_delegation"); got != 1 {
		t.Errorf("count(%q) = %d, want 1; prep.apiSystem = %q", "get_delegation", got, prep.apiSystem)
	}
}
