package db

import (
	"strings"
	"testing"
)

// 000188 seeds a readonly "delegation-guidance" injectable and trims the
// delegation how-to it now owns out of tier-t0-decider/tier-t1-executor
// (seeded readonly by 000178), keeping their "## Role:" headers.

// TestMigration188_DelegationGuidanceSeeded verifies the readonly injectable
// row: type, readonly, template==default_template (migration058 invariant),
// and the anchor tokens the spawner-side gate/tests key off of.
func TestMigration188_DelegationGuidanceSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate, typ string
	var readonly int
	err = pool.QueryRow(
		`SELECT template, default_template, readonly, type FROM default_templates WHERE id = 'delegation-guidance'`,
	).Scan(&template, &defaultTemplate, &readonly, &typ)
	if err != nil {
		t.Fatalf("SELECT default_templates id=delegation-guidance: %v", err)
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
	for _, anchor := range []string{"delegate", "get_delegation", "dynamic", "extractor"} {
		if !strings.Contains(template, anchor) {
			t.Errorf("template missing anchor %q; got %q", anchor, template)
		}
	}
}

// TestMigration188_ReadonlyInvariantHoldsRepoWide re-verifies migration058's
// acceptance criterion after 000188's inserts/updates: no readonly row has
// template != default_template.
func TestMigration188_ReadonlyInvariantHoldsRepoWide(t *testing.T) {
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

// TestMigration188_TierTemplatesTrimmed verifies tier-t0-decider and
// tier-t1-executor keep their "## Role:" header + template==default_template,
// but no longer contain the delegation how-to phrases now owned by the
// delegation-guidance injectable.
func TestMigration188_TierTemplatesTrimmed(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct {
		id           string
		wantHeader   string
		movedPhrases []string // delegation how-to now owned by delegation-guidance; must be gone
	}{
		{
			id:           "tier-t0-decider",
			wantHeader:   "## Role: T0 Decider",
			movedPhrases: []string{"cheap-tier workers", "raw tool output"},
		},
		{
			id:           "tier-t1-executor",
			wantHeader:   "## Role: T1 Executor",
			movedPhrases: []string{"T2 extractor", "raw transcripts"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var template, defaultTemplate string
			err := pool.QueryRow(
				`SELECT template, default_template FROM default_templates WHERE id = ?`, tc.id,
			).Scan(&template, &defaultTemplate)
			if err != nil {
				t.Fatalf("SELECT default_templates id=%q: %v", tc.id, err)
			}
			if template != defaultTemplate {
				t.Errorf("template != default_template (readonly invariant violated) for %q:\ntemplate=%q\ndefault_template=%q", tc.id, template, defaultTemplate)
			}
			if !strings.Contains(template, tc.wantHeader) {
				t.Errorf("%s template missing header %q; got %q", tc.id, tc.wantHeader, template)
			}
			for _, phrase := range tc.movedPhrases {
				if strings.Contains(template, phrase) {
					t.Errorf("%s template still contains moved delegation phrase %q; got %q", tc.id, phrase, template)
				}
			}
		})
	}
}
