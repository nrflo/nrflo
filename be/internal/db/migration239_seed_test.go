package db

import (
	"strings"
	"testing"
)

// 000239 adds the third delegate tier: _t3_verifier (tier-2 chain, one-shot,
// no delegate in the CSV, refute-by-default prompt) and points the executor
// prompt, delegation-guidance injectable, and t0 verification-pass guidance
// at it.

func TestMigration239_VerifierTierSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var mdl, mode, tools, prompt string
	var tier, timeout, isolate int
	err = pool.QueryRow(
		`SELECT model, execution_mode, tools, prompt, tier, timeout, isolate_worktree
		 FROM system_agent_definitions WHERE id = '_t3_verifier'`,
	).Scan(&mdl, &mode, &tools, &prompt, &tier, &timeout, &isolate)
	if err != nil {
		t.Fatalf("SELECT _t3_verifier: %v", err)
	}
	if mdl != "" {
		t.Errorf("model = %q, want empty (tier-resolved)", mdl)
	}
	if tier != 2 {
		t.Errorf("tier = %d, want 2", tier)
	}
	if mode != "api" {
		t.Errorf("execution_mode = %q, want api", mode)
	}
	if timeout != 10 {
		t.Errorf("timeout = %d, want 10 (minutes)", timeout)
	}
	if isolate != 0 {
		t.Errorf("isolate_worktree = %d, want 0 (read-only role stays in place)", isolate)
	}
	if strings.Contains(tools, "delegate") {
		t.Errorf("tools CSV must not grant delegate (no recursion): %s", tools)
	}
	for _, tool := range []string{"read_file", "bash", "findings_add", "agent_finished"} {
		if !strings.Contains(tools, tool) {
			t.Errorf("tools CSV missing %q: %s", tool, tools)
		}
	}
	for _, anchor := range []string{"adversarially verify", "verdict", "_delegate_findings", "${DELEGATE_BRIEF}"} {
		if !strings.Contains(prompt, anchor) {
			t.Errorf("prompt missing anchor %q", anchor)
		}
	}
}

func TestMigration239_GuidanceRoutesToVerifier(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var executorPrompt string
	if err := pool.QueryRow(
		`SELECT prompt FROM system_agent_definitions WHERE id = '_t1_executor'`,
	).Scan(&executorPrompt); err != nil {
		t.Fatalf("SELECT _t1_executor: %v", err)
	}
	if !strings.Contains(executorPrompt, `tier="verifier"`) {
		t.Error("_t1_executor prompt does not route re-checks to the verifier tier")
	}

	for _, id := range []string{"delegation-guidance", "tier-t0-decider", "tier-t0-bare"} {
		var tpl, def string
		if err := pool.QueryRow(
			`SELECT template, default_template FROM default_templates WHERE id = ?`, id,
		).Scan(&tpl, &def); err != nil {
			t.Fatalf("SELECT %s: %v", id, err)
		}
		if !strings.Contains(tpl, "verifier") || !strings.Contains(def, "verifier") {
			t.Errorf("%s template/default_template does not mention the verifier tier", id)
		}
	}
	for _, id := range []string{"tier-t0-decider", "tier-t0-bare"} {
		var tpl string
		if err := pool.QueryRow(
			`SELECT template FROM default_templates WHERE id = ?`, id,
		).Scan(&tpl); err != nil {
			t.Fatalf("SELECT %s: %v", id, err)
		}
		if strings.Contains(tpl, "a fresh extractor per claim") {
			t.Errorf("%s still tells deciders to re-check with an extractor", id)
		}
		if !strings.Contains(tpl, "Extractor and verifier delegations return their findings inline") {
			t.Errorf("%s inline-return contract does not cover verifier", id)
		}
	}
}
