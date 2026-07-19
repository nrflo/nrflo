package spawner

// Resolution-matrix tests for agent_definitions.system_template_id: a
// def/profile-level injectable id that wins over the existing
// claude_system_prompt_override_enabled gate (claude CLI), api-system-prompt
// default (api mode), and is prepended to the prompt body for adapters
// without --system-prompt-file support (codex). Empty system_template_id must
// stay byte-identical to today's behavior across all three delivery channels.
// Per-delivery-channel prepareSpawn tests split out to keep each file under
// the 300-line cap: template_system_template_id_claude_test.go,
// _codex_test.go, _api_test.go.

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/db"
)

// insertAgentDefWithSystemTemplate inserts an agent_definition row carrying a
// non-default system_template_id, for any execution_mode.
func insertAgentDefWithSystemTemplate(t *testing.T, env *contextSaveTestEnv, agentID, model, executionMode, systemTemplateID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, system_template_id, created_at, updated_at)
		VALUES (?, ?, 'feature', ?, 20, '# prompt', ?, 'agent_finished', ?, ?, ?)`,
		agentID, env.projectID, model, executionMode, systemTemplateID, now, now,
	); err != nil {
		t.Fatalf("insert agent_definition %q: %v", agentID, err)
	}
}

func mustRenderInjectable(t *testing.T, env *contextSaveTestEnv, id string) string {
	t.Helper()
	pool := db.WrapAsPool(env.database)
	rendered := renderInjectable(context.Background(), pool, id, nil)
	if rendered == "" {
		t.Fatalf("renderInjectable(%q) = empty; want seeded content", id)
	}
	return rendered
}

// ── resolveSystemPromptOverride precedence (unit-level) ─────────────────────

// TestResolveSystemPromptOverride_DefTemplateWinsOverGate verifies the def's
// system_template_id wins over the global claude_system_prompt_override_enabled
// gate regardless of the gate's state.
func TestResolveSystemPromptOverride_DefTemplateWinsOverGate(t *testing.T) {
	t.Parallel()

	for _, gateOn := range []bool{true, false} {
		t.Run(map[bool]string{true: "gate=on", false: "gate=off"}[gateOn], func(t *testing.T) {
			t.Parallel()
			env := setupContextSaveTestEnv(t)
			defer env.cleanup()
			insertAgentDefWithSystemTemplate(t, env, "analyzer", "sonnet-5", "cli_interactive", "tier-t2-extractor")

			pool := db.WrapAsPool(env.database)
			if err := pool.SetConfig("claude_system_prompt_override_enabled", map[bool]string{true: "true", false: "false"}[gateOn]); err != nil {
				t.Fatalf("SetConfig: %v", err)
			}

			sp := New(Config{
				DataPath: env.dbPath,
				Pool:     pool,
				ModelConfigs: map[string]ModelConfig{
					"sonnet-5": {Provider: "anthropic", CLIModel: "raw"},
				},
			})

			want := mustRenderInjectable(t, env, "tier-t2-extractor")
			got := sp.resolveSystemPromptOverride("analyzer", env.projectID, "feature", "sonnet-5", nil)
			if got != want {
				t.Errorf("resolveSystemPromptOverride = %q, want rendered tier-t2-extractor %q", got, want)
			}
			if strings.Contains(got, "T0 Decider") || strings.Contains(got, "T1 Executor") {
				t.Errorf("resolveSystemPromptOverride returned wrong template content: %q", got)
			}
		})
	}
}

// TestResolveSystemPromptOverride_EmptyTemplateID_FallsBackToGate verifies
// byte-identical fallback to systemPromptOverrideFor when system_template_id
// is unset (regression: today's behavior unchanged).
func TestResolveSystemPromptOverride_EmptyTemplateID_FallsBackToGate(t *testing.T) {
	t.Parallel()

	for _, gateOn := range []bool{true, false} {
		t.Run(map[bool]string{true: "gate=on", false: "gate=off"}[gateOn], func(t *testing.T) {
			t.Parallel()
			env := setupContextSaveTestEnv(t)
			defer env.cleanup()
			insertAgentDefWithSystemTemplate(t, env, "analyzer", "sonnet-5", "cli_interactive", "")

			pool := db.WrapAsPool(env.database)
			if err := pool.SetConfig("claude_system_prompt_override_enabled", map[bool]string{true: "true", false: "false"}[gateOn]); err != nil {
				t.Fatalf("SetConfig: %v", err)
			}

			sp := New(Config{
				DataPath: env.dbPath,
				Pool:     pool,
				ModelConfigs: map[string]ModelConfig{
					"sonnet-5": {Provider: "anthropic", CLIModel: "raw"},
				},
			})

			want := sp.systemPromptOverrideFor("sonnet-5", nil)
			got := sp.resolveSystemPromptOverride("analyzer", env.projectID, "feature", "sonnet-5", nil)
			if got != want {
				t.Errorf("resolveSystemPromptOverride = %q, want byte-identical systemPromptOverrideFor result %q", got, want)
			}
			if gateOn && got == "" {
				t.Error("gate on + empty system_template_id: want non-empty override (byte-identical to pre-feature behavior)")
			}
			if !gateOn && got != "" {
				t.Error("gate off + empty system_template_id: want empty override (byte-identical to pre-feature behavior)")
			}
		})
	}
}

// TestResolveSystemPromptOverride_NilDef_FallsBackToGate verifies a global
// workflow (no agent def row, loadAgentDefinition returns nil) still falls
// back to the gate rather than panicking.
func TestResolveSystemPromptOverride_NilDef_FallsBackToGate(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	pool := db.WrapAsPool(env.database)
	if err := pool.SetConfig("claude_system_prompt_override_enabled", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     pool,
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", CLIModel: "raw"},
		},
	})

	want := sp.systemPromptOverrideFor("sonnet-5", nil)
	got := sp.resolveSystemPromptOverride("no-such-agent", env.projectID, "feature", "sonnet-5", nil)
	if got != want {
		t.Errorf("resolveSystemPromptOverride(nil def) = %q, want %q", got, want)
	}
}

// ── noSystemPromptFilePrefix (pure helper) ───────────────────────────────────

func TestNoSystemPromptFilePrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		suffix   string
		override string
		adapter  CLIAdapter
		want     string
	}{
		{"claude ignores everything", "suffix", "override", &ClaudeAdapter{}, ""},
		{"codex both empty", "", "", &CodexAdapter{}, ""},
		{"codex suffix only", "suffix-body", "", &CodexAdapter{}, "suffix-body"},
		{"codex override only", "", "override-body", &CodexAdapter{}, "override-body"},
		{"codex both set: override then suffix", "suffix-body", "override-body", &CodexAdapter{}, "override-body\n\nsuffix-body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := noSystemPromptFilePrefix(tc.suffix, tc.override, tc.adapter)
			if got != tc.want {
				t.Errorf("noSystemPromptFilePrefix(%q, %q, %T) = %q, want %q", tc.suffix, tc.override, tc.adapter, got, tc.want)
			}
		})
	}
}
