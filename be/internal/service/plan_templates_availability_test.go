package service

import (
	"os"
	"strings"
	"testing"

	"be/internal/clock"
)

// stubLookPath replaces the package-level lookPath var (used by CLIAvailable)
// and clears its memoization cache so CLIAvailable re-probes with the stub.
// Restores both on cleanup. Tests using this MUST NOT call t.Parallel(): the
// lookPath var and cliAvailabilityCache are process-global, so this relies on
// Go's test dispatch running non-parallel siblings to completion (including
// t.Cleanup) before any t.Parallel() sibling's body actually executes.
func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := lookPath
	lookPath = fn
	cliAvailabilityCache.Range(func(k, _ any) bool { cliAvailabilityCache.Delete(k); return true })
	t.Cleanup(func() {
		lookPath = orig
		cliAvailabilityCache.Range(func(k, _ any) bool { cliAvailabilityCache.Delete(k); return true })
	})
}

// TestEnabledTemplates_DynamicWorkflow_PathProbeDropsCodexEvenWhenRowsEnabled
// is acceptance case #5: every seeded codex_* cli_models row is read_only=1
// (migration 000156) and can never be disabled via the `enabled` flag (see
// cli_model.go), so on an install without the codex binary the only way to
// hide codex-backed templates is CLIAvailable's PATH probe. Stubs lookPath to
// fail for "codex" (simulating a Claude-only install) while the codex rows
// stay enabled=1, and verifies EnabledTemplates drops the codex twins while
// ValidateTemplatesEnabled rejects a manifest referencing one.
func TestEnabledTemplates_DynamicWorkflow_PathProbeDropsCodexEvenWhenRowsEnabled(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) {
		if name == "codex" {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + name, nil
	})
	pool := seedDynamicWorkflowDB(t, "path_probe.db")

	var codexEnabled int
	if err := pool.QueryRow(`SELECT enabled FROM cli_models WHERE id = 'codex_gpt56_terra_high'`).Scan(&codexEnabled); err != nil {
		t.Fatalf("select codex row enabled: %v", err)
	}
	if codexEnabled != 1 {
		t.Fatalf("codex_gpt56_terra_high.enabled = %d, want 1 (read_only rows stay enabled)", codexEnabled)
	}

	all, err := AllowedTemplates(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	enabled := EnabledTemplates(pool, clock.Real(), all)
	if len(enabled) != len(all)-2 {
		t.Fatalf("EnabledTemplates count = %d, want %d (10 - 2 codex twins hidden by PATH probe)", len(enabled), len(all)-2)
	}
	for _, tpl := range enabled {
		if tpl.ID == "module-reviewer-codex" || tpl.ID == "finding-verifier-codex" {
			t.Errorf("EnabledTemplates kept %q despite lookPath(codex) failing", tpl.ID)
		}
	}

	if err := ValidateTemplatesEnabled(pool, clock.Real(), []PlanTemplate{{ID: "module-reviewer-codex", Model: "codex_gpt56_terra_high", ExecutionMode: "cli_interactive"}}); err == nil {
		t.Fatal("ValidateTemplatesEnabled: expected error for a codex template on a codex-less install, got nil")
	} else if !strings.Contains(err.Error(), "module-reviewer-codex") {
		t.Errorf("ValidateTemplatesEnabled error = %q, want it to name the offending template", err.Error())
	}
}

// TestEnabledTemplates_APIModeDisabled_DropsAPITemplates verifies an
// api-mode fanout_template is excluded from EnabledTemplates while
// api_mode_enabled is off, and included once it is turned on.
func TestEnabledTemplates_APIModeDisabled_DropsAPITemplates(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)
	insertFanoutTemplate(t, pool, projectID, workflowID, "api-worker", "opus_4_8", "api")

	all, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}

	offEnabled := EnabledTemplates(pool, clock.Real(), all)
	for _, tpl := range offEnabled {
		if tpl.ID == "api-worker" {
			t.Error("EnabledTemplates included an api-mode template while api_mode_enabled is off")
		}
	}

	settingsSvc := NewGlobalSettingsService(pool, clock.Real())
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	onEnabled := EnabledTemplates(pool, clock.Real(), all)
	found := false
	for _, tpl := range onEnabled {
		if tpl.ID == "api-worker" {
			found = true
		}
	}
	if !found {
		t.Error("EnabledTemplates dropped an api-mode template even after api_mode_enabled was turned on")
	}
}

// TestEnabledTemplates_EffectiveReasoningEffort verifies the effective effort
// filled in by EnabledTemplates is the def-level override when one is set,
// and the model row's own reasoning_effort otherwise.
func TestEnabledTemplates_EffectiveReasoningEffort(t *testing.T) {
	t.Parallel()
	pool, projectID, workflowID := setupPlanValidateEnv(t)

	// "worker" (seeded by setupPlanValidateEnv) is bound to model "sonnet",
	// whose seeded cli_models row has reasoning_effort = "" (no row-level
	// default) — the effective effort should be "" with no override.
	insertFanoutTemplate(t, pool, projectID, workflowID, "override-worker", "opus_4_8", "cli_interactive")
	if _, err := pool.Exec(`UPDATE agent_definitions SET reasoning_effort = 'xhigh' WHERE project_id = ? AND workflow_id = ? AND id = 'override-worker'`, projectID, workflowID); err != nil {
		t.Fatalf("set reasoning_effort override: %v", err)
	}

	all, err := AllowedTemplates(pool, projectID, workflowID)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	enabled := EnabledTemplates(pool, clock.Real(), all)

	var worker, overrideWorker *PlanTemplate
	for i := range enabled {
		switch enabled[i].ID {
		case "worker":
			worker = &enabled[i]
		case "override-worker":
			overrideWorker = &enabled[i]
		}
	}
	if worker == nil || overrideWorker == nil {
		t.Fatalf("EnabledTemplates missing expected templates: got %+v", enabled)
	}
	if worker.ReasoningEffort != "" {
		t.Errorf("worker (no override) ReasoningEffort = %q, want empty (model row default)", worker.ReasoningEffort)
	}
	if overrideWorker.ReasoningEffort != "xhigh" {
		t.Errorf("override-worker ReasoningEffort = %q, want %q (def override wins over the opus_4_8 row's own effort)", overrideWorker.ReasoningEffort, "xhigh")
	}
	if worker.CLIType != "claude" || overrideWorker.CLIType != "claude" {
		t.Errorf("CLIType = %q/%q, want claude/claude for cli_interactive templates", worker.CLIType, overrideWorker.CLIType)
	}
}
