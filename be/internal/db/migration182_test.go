package db

import (
	"strings"
	"testing"
)

// TestMigration182_ExtractorSeed verifies the _t2_extractor tier definition:
// haiku model, low effort, api execution mode, no delegate tool.
func TestMigration182_ExtractorSeed(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var model, executionMode, tools string
	var reasoningEffort *string
	if err := pool.QueryRow(
		`SELECT model, execution_mode, tools, reasoning_effort FROM system_agent_definitions WHERE id = '_t2_extractor'`,
	).Scan(&model, &executionMode, &tools, &reasoningEffort); err != nil {
		t.Fatalf("query _t2_extractor: %v", err)
	}
	if model != "haiku-4-5" {
		t.Errorf("model = %q, want haiku-4-5", model)
	}
	if executionMode != "api" {
		t.Errorf("execution_mode = %q, want api", executionMode)
	}
	if reasoningEffort == nil || *reasoningEffort != "low" {
		t.Errorf("reasoning_effort = %v, want \"low\"", reasoningEffort)
	}
	if strings.Contains(tools, "delegate") {
		t.Errorf("tools = %q, must not include delegate (T2 has no further delegation)", tools)
	}
	for _, want := range []string{"findings_add", "agent_finished", "agent_fail"} {
		if !strings.Contains(tools, want) {
			t.Errorf("tools = %q, want contains %q", tools, want)
		}
	}
}

// TestMigration182_ExecutorSeed verifies the _t1_executor tier definition:
// sonnet model, medium effort, api execution mode, tools include delegate.
func TestMigration182_ExecutorSeed(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var model, executionMode, tools string
	var reasoningEffort *string
	if err := pool.QueryRow(
		`SELECT model, execution_mode, tools, reasoning_effort FROM system_agent_definitions WHERE id = '_t1_executor'`,
	).Scan(&model, &executionMode, &tools, &reasoningEffort); err != nil {
		t.Fatalf("query _t1_executor: %v", err)
	}
	if model != "sonnet-5" {
		t.Errorf("model = %q, want sonnet-5", model)
	}
	if executionMode != "api" {
		t.Errorf("execution_mode = %q, want api", executionMode)
	}
	if reasoningEffort == nil || *reasoningEffort != "medium" {
		t.Errorf("reasoning_effort = %v, want \"medium\"", reasoningEffort)
	}
	if tools != "*" {
		t.Errorf("tools = %q, want \"*\" (full execution set, includes delegate)", tools)
	}
}

// TestMigration182_ReadonlyTemplateInvariant re-verifies the migration-058
// invariant still holds after 182: no readonly default_templates row may
// diverge from its default_template baseline. 182 itself seeds no
// default_templates rows, but this guards against any future regression in
// the same migration file.
func TestMigration182_ReadonlyTemplateInvariant(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var mismatched int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM default_templates WHERE readonly = 1 AND template != default_template`,
	).Scan(&mismatched); err != nil {
		t.Fatalf("count mismatched rows: %v", err)
	}
	if mismatched != 0 {
		t.Errorf("readonly rows with template != default_template = %d, want 0", mismatched)
	}
}

// TestMigration182_ModelsExistForBothTiers verifies the models referenced by
// the two tier seeds are themselves registered and api-mode-enabled — a
// dangling model reference would make the delegate builtin fail at spawn
// time with "model not found in models".
func TestMigration182_ModelsExistForBothTiers(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"haiku-4-5", "sonnet-5"} {
		var apiModel string
		if err := pool.QueryRow(`SELECT api_model FROM models WHERE id = ?`, id).Scan(&apiModel); err != nil {
			t.Fatalf("query model %s: %v", id, err)
		}
		if apiModel == "" {
			t.Errorf("model %s: api_model is empty, want api mode enabled", id)
		}
	}
}
