package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func insertLogSessWithEffectiveMode(t *testing.T, f *logFixture, id, agentType, effectiveMode string, endedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := endedAt.Add(-5 * time.Second).UTC().Format(time.RFC3339Nano)
	ended := endedAt.UTC().Format(time.RFC3339Nano)
	if _, err := f.db.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, effective_mode, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'log-proj', '', ?, 'ph', ?, 'completed', ?, ?, ?, ?, ?)`,
		id, f.wfiID, agentType, effectiveMode, started, ended, now, now,
	); err != nil {
		t.Fatalf("insertLogSessWithEffectiveMode(%s): %v", id, err)
	}
}

func setupEffectiveModeFixture(t *testing.T) *logFixture {
	t.Helper()
	f := setupLogFixture(t)
	mustExecLog(t, f.db, `INSERT INTO agent_definitions
		(id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, created_at, updated_at)
		VALUES ('agent-api', 'log-proj', 'log-wf', 'sonnet', 20, '', 0, 'api', datetime('now'), datetime('now'))`)
	return f
}

func TestSetEffectiveMode_UpdatesColumn(t *testing.T) {
	t.Parallel()
	f := setupEffectiveModeFixture(t)
	r := f.repo
	base := time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)

	insertLogSessWithEffectiveMode(t, f, "sess-set-em", "agent-api", "cli", base)

	if err := r.SetEffectiveMode("sess-set-em", "cli_interactive"); err != nil {
		t.Fatalf("SetEffectiveMode: %v", err)
	}

	// Verify via Get round-trip.
	got, err := r.Get("sess-set-em")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.EffectiveMode.Valid {
		t.Errorf("EffectiveMode.Valid = false after SetEffectiveMode")
	} else if got.EffectiveMode.String != "cli_interactive" {
		t.Errorf("EffectiveMode = %q, want cli_interactive", got.EffectiveMode.String)
	}
}

func TestSetEffectiveMode_NotFound(t *testing.T) {
	t.Parallel()
	f := setupEffectiveModeFixture(t)
	err := f.repo.SetEffectiveMode("does-not-exist", "cli")
	if err == nil {
		t.Errorf("SetEffectiveMode(missing) returned nil, want error")
	}
}

func TestCreate_PersistsEffectiveMode(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	mustExecLog(t, database, `INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-em', 'P', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj-em', 'wf-em', '', 'ticket', datetime('now'), datetime('now'))`)
	mustExecLog(t, database, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		VALUES ('wfi-em', 'proj-em', 'TKT-1', 'wf-em', 'active', 'ticket', datetime('now'), datetime('now'))`)

	r := NewAgentSessionRepo(database, clock.Real())
	sess := &model.AgentSession{
		ID:                 "sess-create-em",
		ProjectID:          "proj-em",
		TicketID:           "TKT-1",
		WorkflowInstanceID: "wfi-em",
		Phase:              "p",
		AgentType:          "a",
		Status:             model.AgentSessionRunning,
	}
	sess.EffectiveMode.String = "script"
	sess.EffectiveMode.Valid = true

	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get("sess-create-em")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.EffectiveMode.Valid {
		t.Errorf("EffectiveMode.Valid = false after Create")
	} else if got.EffectiveMode.String != "script" {
		t.Errorf("EffectiveMode = %q, want script", got.EffectiveMode.String)
	}
}

func TestListFinished_EffectiveModeColumn(t *testing.T) {
	t.Parallel()
	f := setupEffectiveModeFixture(t)
	base := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC)

	// Session with effective_mode set — column dominates JOIN fallback.
	insertLogSessWithEffectiveMode(t, f, "sess-lf-mode", "agent-api", "cli_interactive", base.Add(2*time.Second))

	// Session without effective_mode (NULL) — falls back to agent_def execution_mode via JOIN.
	insertLogSess(t, f.db, "sess-lf-null", "log-proj", f.wfiID, "completed", base.Add(time.Second))

	rows, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Fatalf("rows count = %d, want 2", len(rows))
	}

	// Most recent first: sess-lf-mode (base+2s), sess-lf-null (base+1s).
	r0 := rows[0]
	if r0.SessionID != "sess-lf-mode" {
		t.Fatalf("rows[0].SessionID = %q, want sess-lf-mode", r0.SessionID)
	}
	if !r0.EffectiveMode.Valid || r0.EffectiveMode.String != "cli_interactive" {
		t.Errorf("rows[0].EffectiveMode = {Valid:%v, String:%q}, want {true, cli_interactive}",
			r0.EffectiveMode.Valid, r0.EffectiveMode.String)
	}

	r1 := rows[1]
	if r1.SessionID != "sess-lf-null" {
		t.Fatalf("rows[1].SessionID = %q, want sess-lf-null", r1.SessionID)
	}
	if r1.EffectiveMode.Valid {
		t.Errorf("rows[1].EffectiveMode.Valid = true, want false (legacy NULL row)")
	}
	// The JOIN provides agent_def.execution_mode for the legacy row.
	// sess-lf-null uses agent_type='ag' which has no matching agent_def → ExecutionMode is NULL too.
	if r1.ExecutionMode.Valid {
		t.Errorf("rows[1].ExecutionMode.Valid = true for unmatched agent_type 'ag', want false")
	}
}

func TestListFinished_EffectiveModeAllValues(t *testing.T) {
	t.Parallel()
	f := setupEffectiveModeFixture(t)
	base := time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC)

	modes := []string{"cli", "cli_interactive", "api", "script"}
	for i, mode := range modes {
		insertLogSessWithEffectiveMode(t, f, "sess-all-"+mode, "agent-api", mode, base.Add(time.Duration(i)*time.Second))
	}

	rows, total, err := f.repo.ListFinished(ListFinishedFilter{ProjectID: "log-proj"}, 1, 100)
	if err != nil {
		t.Fatalf("ListFinished: %v", err)
	}
	if total != len(modes) {
		t.Errorf("total = %d, want %d", total, len(modes))
	}
	if len(rows) != len(modes) {
		t.Fatalf("rows count = %d, want %d", len(rows), len(modes))
	}

	// DESC order: script(+3s), api(+2s), cli_interactive(+1s), cli(+0s).
	expectedOrder := []string{"script", "api", "cli_interactive", "cli"}
	for i, wantMode := range expectedOrder {
		wantID := "sess-all-" + wantMode
		if rows[i].SessionID != wantID {
			t.Errorf("rows[%d].SessionID = %q, want %q", i, rows[i].SessionID, wantID)
		}
		if !rows[i].EffectiveMode.Valid || rows[i].EffectiveMode.String != wantMode {
			t.Errorf("rows[%d].EffectiveMode = {Valid:%v, String:%q}, want {true, %q}",
				i, rows[i].EffectiveMode.Valid, rows[i].EffectiveMode.String, wantMode)
		}
	}
}
