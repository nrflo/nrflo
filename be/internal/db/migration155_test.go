package db

import (
	"testing"
)

// TestMigration155_ToolsCSVRewrite verifies existing agent definitions granting
// the deleted web_deep_research builtin are rewritten to the replacement tools
// (a CSV token matching no tool hard-fails the spawn).
func TestMigration155_ToolsCSVRewrite(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed rows the way a pre-155 install would look, then re-run the rewrite
	// statement to assert its transformation (migrations already ran on the
	// empty template, so apply the statement directly to the seeded rows).
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p155', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
		 VALUES ('wf155', 'p155', '', 'project', '[]', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	cases := map[string]string{
		"a155-only":   "web_deep_research",
		"a155-middle": "web_search,web_deep_research,emit_findings",
		"a155-spaced": "web_search, web_deep_research, emit_findings",
		"a155-none":   "web_search,emit_findings",
	}
	for id, tools := range cases {
		if _, err := pool.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, tools, created_at, updated_at)
			 VALUES (?, 'p155', 'wf155', 'p', 0, ?, datetime('now'), datetime('now'))`, id, tools); err != nil {
			t.Fatalf("seed agent %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(
		`UPDATE agent_definitions SET tools =
		   TRIM(REPLACE(REPLACE(',' || tools || ',', ',web_deep_research,', ',run_subworkflow,get_subworkflow,'), ', web_deep_research,', ',run_subworkflow,get_subworkflow,'), ',')
		 WHERE ',' || REPLACE(tools, ' ', '') || ',' LIKE '%,web_deep_research,%'`); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	want := map[string]string{
		"a155-only":   "run_subworkflow,get_subworkflow",
		"a155-middle": "web_search,run_subworkflow,get_subworkflow,emit_findings",
		"a155-spaced": "web_search,run_subworkflow,get_subworkflow, emit_findings",
		"a155-none":   "web_search,emit_findings",
	}
	for id, w := range want {
		var got string
		if err := pool.QueryRow(`SELECT tools FROM agent_definitions WHERE id=?`, id).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if got != w {
			t.Errorf("%s: tools = %q, want %q", id, got, w)
		}
	}

	// New columns exist with defaults.
	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		 VALUES ('wfi155', 'p155', '', 'wf155', 'active', 'project', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	var parent string
	var swDepth, swStarts int
	if err := pool.QueryRow(
		`SELECT parent_instance_id, subworkflow_depth, subworkflow_starts FROM workflow_instances WHERE id='wfi155'`,
	).Scan(&parent, &swDepth, &swStarts); err != nil {
		t.Fatalf("query new columns: %v", err)
	}
	if parent != "" || swDepth != 0 || swStarts != 0 {
		t.Errorf("defaults = (%q, %d, %d), want (\"\", 0, 0)", parent, swDepth, swStarts)
	}
}
