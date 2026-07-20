package db

import (
	"strings"
	"testing"
)

// 000190 seeds a readonly "tier-t0-bare" injectable: role framing only for
// the console t0-bare profile — delegation mechanics arrive separately via
// the delegation-guidance append (spawner.AppendDelegationGuidanceForTools).

// TestMigration190_TierT0BareSeeded verifies the readonly injectable row:
// type, readonly, template==default_template (migration058 invariant), and
// the anchor tokens the console profile/tests key off of.
func TestMigration190_TierT0BareSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate, typ string
	var readonly int
	err = pool.QueryRow(
		`SELECT template, default_template, readonly, type FROM default_templates WHERE id = 'tier-t0-bare'`,
	).Scan(&template, &defaultTemplate, &readonly, &typ)
	if err != nil {
		t.Fatalf("SELECT default_templates id=tier-t0-bare: %v", err)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	if typ != "injectable" {
		t.Errorf("type = %q, want %q", typ, "injectable")
	}
	if template != defaultTemplate {
		t.Errorf("template != default_template (readonly invariant violated):\ntemplate=%q\ndefault_template=%q", template, defaultTemplate)
	}
	for _, anchor := range []string{"T0 Bare", "refinery digest"} {
		if !strings.Contains(template, anchor) {
			t.Errorf("template missing anchor %q; got %q", anchor, template)
		}
	}
}

// TestMigration190_NoDelegationMechanics verifies tier-t0-bare's own body
// contains role framing only, not the delegation how-to now owned by the
// delegation-guidance injectable (000188) — the two compose via
// AppendDelegationGuidanceForTools, not duplication.
func TestMigration190_NoDelegationMechanics(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template string
	err = pool.QueryRow(
		`SELECT template FROM default_templates WHERE id = 'tier-t0-bare'`,
	).Scan(&template)
	if err != nil {
		t.Fatalf("SELECT default_templates id=tier-t0-bare: %v", err)
	}
	for _, phrase := range []string{"get_delegation", "cheap-tier workers"} {
		if strings.Contains(template, phrase) {
			t.Errorf("tier-t0-bare template unexpectedly contains delegation-guidance phrase %q; got %q", phrase, template)
		}
	}
}

// TestMigration190_ReadonlyInvariantHoldsRepoWide re-verifies migration058's
// acceptance criterion after 000190's insert: no readonly row has
// template != default_template.
func TestMigration190_ReadonlyInvariantHoldsRepoWide(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var mismatched int
	err = pool.QueryRow(
		"SELECT COUNT(*) FROM default_templates WHERE readonly = 1 AND template != default_template",
	).Scan(&mismatched)
	if err != nil {
		t.Fatalf("count mismatched rows: %v", err)
	}
	if mismatched != 0 {
		t.Errorf("readonly rows with template != default_template = %d, want 0", mismatched)
	}
}
