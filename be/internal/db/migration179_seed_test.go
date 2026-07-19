package db

import (
	"strings"
	"testing"
)

// TestMigration179_RefinerySystemAgentSeeded verifies the single `_refinery`
// system_agent_definitions row: role/model/execution_mode plus the api-mode
// fields fold.go depends on (api_max_tokens, no tools).
func TestMigration179_RefinerySystemAgentSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var role, model, executionMode, tools string
	var apiMaxTokens int
	err = pool.QueryRow(
		`SELECT role, model, execution_mode, tools, api_max_tokens FROM system_agent_definitions WHERE id = '_refinery'`,
	).Scan(&role, &model, &executionMode, &tools, &apiMaxTokens)
	if err != nil {
		t.Fatalf("SELECT system_agent_definitions id=_refinery: %v", err)
	}
	if role != "refinery" {
		t.Errorf("role = %q, want %q", role, "refinery")
	}
	if model != "haiku-4-5" {
		t.Errorf("model = %q, want %q", model, "haiku-4-5")
	}
	if executionMode != "api" {
		t.Errorf("execution_mode = %q, want %q", executionMode, "api")
	}
	if tools != "" {
		t.Errorf("tools = %q, want empty (refinery never spawns a session)", tools)
	}
	if apiMaxTokens != 1500 {
		t.Errorf("api_max_tokens = %d, want 1500", apiMaxTokens)
	}
}

// TestMigration179_WorkingSetInjectableIsDigestWrapper verifies the
// `working-set` default_templates row was repointed at ${DIGEST} and, per the
// migration058 readonly invariant, that template == default_template.
func TestMigration179_WorkingSetInjectableIsDigestWrapper(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate string
	var readonly int
	err = pool.QueryRow(
		`SELECT template, default_template, readonly FROM default_templates WHERE id = 'working-set'`,
	).Scan(&template, &defaultTemplate, &readonly)
	if err != nil {
		t.Fatalf("SELECT default_templates id=working-set: %v", err)
	}
	if template != defaultTemplate {
		t.Errorf("template != default_template (readonly=%d invariant violated):\ntemplate=%q\ndefault_template=%q", readonly, template, defaultTemplate)
	}
	if !strings.Contains(template, "${DIGEST}") {
		t.Errorf("template = %q, want it to contain ${DIGEST}", template)
	}
}

// TestMigration179_RefineryDigestsTableSchema verifies the refinery_digests
// head table's columns and single-row-per-session primary key.
func TestMigration179_RefineryDigestsTableSchema(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(`PRAGMA table_info(refinery_digests)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	wantCols := map[string]bool{
		"console_session_id": false,
		"project_id":         false,
		"version":            false,
		"content":            false,
		"fold_count":         false,
		"created_at":         false,
		"updated_at":         false,
	}
	var pkCol string
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		if _, ok := wantCols[name]; ok {
			wantCols[name] = true
		}
		if pk == 1 {
			pkCol = name
		}
	}
	for col, seen := range wantCols {
		if !seen {
			t.Errorf("refinery_digests missing expected column %q", col)
		}
	}
	if pkCol != "console_session_id" {
		t.Errorf("refinery_digests PRIMARY KEY = %q, want %q", pkCol, "console_session_id")
	}
}
