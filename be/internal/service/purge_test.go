package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

func purgeCount(t *testing.T, pool *db.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func TestPurgeInstanceTraceIfEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "purge.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	home := t.TempDir()
	t.Setenv("NRFLO_HOME", home)
	clk := clock.Real()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ctx := context.Background()

	for _, stmt := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1','P','` + now + `','` + now + `')`,
		`INSERT INTO workflows (project_id, id, description, scope_type, purge_on_completion, created_at, updated_at)
		 VALUES ('p1','wf1','','project',1,'` + now + `','` + now + `')`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, skip_tags, external_id, external_context, purge_on_completion, created_at, updated_at)
		 VALUES ('wfi-1','p1','','wf1','project','completed','["secret-tag"]','ext-123','sensitive context',1,'` + now + `','` + now + `')`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, external_id, purge_on_completion, created_at, updated_at)
		 VALUES ('wfi-keep','p1','','wf1','project','completed','keep-ext',0,'` + now + `','` + now + `')`,
	} {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("seed: %v\n%s", err, stmt)
		}
	}

	for _, sid := range []string{"sess-1", "sess-2"} {
		if _, err := pool.Exec(`
			INSERT INTO agent_sessions
				(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, result,
				 result_reason, spawn_command, prompt, system_prompt, config, spawn_token, started_at, created_at, updated_at)
			VALUES (?, 'p1','','wfi-1','impl','implementor','completed','pass',
				'failure detail','claude --secret-flag','SENSITIVE PROMPT','SYSTEM SUFFIX','{"hook":1}',?,?,?,?)`,
			sid, sid+"-token", now, now, now); err != nil {
			t.Fatalf("seed session %s: %v", sid, err)
		}
		for seq := 1; seq <= 3; seq++ {
			if _, err := pool.Exec(`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES (?,?,?,?)`,
				sid, seq, "secret message body", now); err != nil {
				t.Fatalf("seed message: %v", err)
			}
		}
	}

	// Findings: workflow_instance scope (input) + session scope (the final result).
	fr := repo.NewFindingRepo(pool, clk)
	if err := fr.Upsert("workflow_instance", "wfi-1", "user_instructions", json.RawMessage(`"do the secret thing"`),
		repo.Denorm{ProjectID: "p1", WorkflowInstanceID: "wfi-1"}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("seed wfi finding: %v", err)
	}
	if err := fr.Upsert("session", "sess-1", "workflow_final_result", json.RawMessage(`"secret result"`),
		repo.Denorm{ProjectID: "p1", WorkflowInstanceID: "wfi-1"}, repo.Actor{Source: "agent"}); err != nil {
		t.Fatalf("seed session finding: %v", err)
	}

	if err := repo.NewErrorLogRepo(pool, clk).Insert(&model.ErrorLog{
		ID: "err-1", ProjectID: "p1", ErrorType: "workflow", InstanceID: "wfi-1", Message: "secret failure detail",
	}); err != nil {
		t.Fatalf("seed error: %v", err)
	}

	// Artifact row + a real file at the internal-storage path.
	const pathKey = "wfi-1/art1__file.txt"
	if err := repo.NewArtifactRepo(pool, clk).Create(&model.Artifact{
		ID: "art1", ProjectID: "p1", WorkflowInstanceID: "wfi-1", Name: "file.txt",
		Type: "internal", PathKey: pathKey, SizeBytes: 6, Source: "input",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	artFile := filepath.Join(home, "projects", "p1", "artifacts", pathKey)
	if err := os.MkdirAll(filepath.Dir(artFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- Purge ---
	svc := NewPurgeService(pool, clk, nil, dbPath)
	counts, err := svc.PurgeInstanceTraceIfEnabled(ctx, "wfi-1")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if counts == nil {
		t.Fatal("expected counts, got nil (flag should be on)")
	}
	if counts.MessagesDeleted != 6 || counts.FilesDeleted != 1 {
		t.Errorf("counts = %+v; want 6 messages, 1 file", counts)
	}

	// Sessions are kept (2 rows) but every sensitive column is blanked.
	if got := purgeCount(t, pool, `SELECT COUNT(*) FROM agent_sessions WHERE workflow_instance_id='wfi-1'`); got != 2 {
		t.Fatalf("sessions kept = %d, want 2", got)
	}
	rows, err := pool.Query(`SELECT prompt, system_prompt, spawn_command, result_reason, spawn_token, config, agent_type, status FROM agent_sessions WHERE workflow_instance_id='wfi-1'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var prompt, sysPrompt, spawnCmd, reason, token sql.NullString
		var config, agentType, status string
		if err := rows.Scan(&prompt, &sysPrompt, &spawnCmd, &reason, &token, &config, &agentType, &status); err != nil {
			t.Fatal(err)
		}
		if prompt.Valid || sysPrompt.Valid || spawnCmd.Valid || reason.Valid || token.Valid {
			t.Errorf("sensitive column not blanked: prompt=%v sys=%v cmd=%v reason=%v token=%v", prompt, sysPrompt, spawnCmd, reason, token)
		}
		if config != "" {
			t.Errorf("config = %q, want '' (NOT NULL column)", config)
		}
		if agentType != "implementor" || status != "completed" {
			t.Errorf("operational columns altered: agent_type=%q status=%q", agentType, status)
		}
	}

	// Everything else for the instance is deleted.
	for _, c := range []struct {
		name, query string
	}{
		{"messages", `SELECT COUNT(*) FROM agent_messages WHERE session_id IN ('sess-1','sess-2')`},
		{"findings", `SELECT COUNT(*) FROM findings WHERE workflow_instance_id='wfi-1'`},
		{"findings_history", `SELECT COUNT(*) FROM findings_history WHERE (scope='workflow_instance' AND scope_id='wfi-1') OR (scope='session' AND scope_id IN ('sess-1','sess-2'))`},
		{"errors", `SELECT COUNT(*) FROM errors WHERE instance_id='wfi-1'`},
		{"artifacts", `SELECT COUNT(*) FROM artifacts WHERE workflow_instance_id='wfi-1'`},
	} {
		if got := purgeCount(t, pool, c.query); got != 0 {
			t.Errorf("%s remaining = %d, want 0", c.name, got)
		}
	}
	if _, err := os.Stat(artFile); !os.IsNotExist(err) {
		t.Errorf("artifact file should be deleted, stat err = %v", err)
	}

	// Instance caller fields cleared; identity/status kept.
	var extID, extCtx sql.NullString
	var skipTags, status string
	if err := pool.QueryRow(`SELECT external_id, external_context, skip_tags, status FROM workflow_instances WHERE id='wfi-1'`).
		Scan(&extID, &extCtx, &skipTags, &status); err != nil {
		t.Fatal(err)
	}
	if extID.Valid || extCtx.Valid {
		t.Errorf("caller fields not cleared: external_id=%v external_context=%v", extID, extCtx)
	}
	if skipTags != "[]" {
		t.Errorf("skip_tags = %q, want '[]'", skipTags)
	}
	if status != "completed" {
		t.Errorf("status = %q, want completed (identity preserved)", status)
	}

	// Audit row written with NULL user_id (exercises the FK fix).
	items, total, err := repo.NewAuditRepo(pool, clk).List(model.AuditFilter{Action: "workflow.purged"}, 1, 10)
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("audit rows = %d, want 1", total)
	}
	if items[0].ResourceID != "wfi-1" || items[0].UserID != "" {
		t.Errorf("audit row = %+v; want resource wfi-1 and empty (NULL) user_id", items[0])
	}

	// Idempotent: a second purge does not error.
	if _, err := svc.PurgeInstanceTraceIfEnabled(ctx, "wfi-1"); err != nil {
		t.Fatalf("second purge: %v", err)
	}

	// Flag off: the other instance is a no-op and stays intact.
	c2, err := svc.PurgeInstanceTraceIfEnabled(ctx, "wfi-keep")
	if err != nil {
		t.Fatalf("purge keep: %v", err)
	}
	if c2 != nil {
		t.Errorf("flag-off purge returned counts %+v, want nil no-op", c2)
	}
	var keepExt sql.NullString
	if err := pool.QueryRow(`SELECT external_id FROM workflow_instances WHERE id='wfi-keep'`).Scan(&keepExt); err != nil {
		t.Fatal(err)
	}
	if !keepExt.Valid || keepExt.String != "keep-ext" {
		t.Errorf("untouched instance external_id = %v, want keep-ext", keepExt)
	}
}
