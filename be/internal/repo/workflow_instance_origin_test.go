package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func TestCreate_Origin_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("console_origin_round_trips_with_session_id", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := pool.Exec(
			`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-origin-console", "Origin Console", t.TempDir(), now, now); err != nil {
			t.Fatalf("seed project: %v", err)
		}
		if _, err := pool.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-origin-console", "proj-origin-console", "", "project", now, now); err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		r := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:              "wfi-origin-console-1",
			ProjectID:       "proj-origin-console",
			WorkflowID:      "wf-origin-console",
			ScopeType:       "project",
			Status:          model.WorkflowInstanceActive,
			Origin:          model.RunOriginConsole,
			OriginSessionID: "sess-console-abc",
		}
		if err := r.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := r.Get("wfi-origin-console-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Origin != model.RunOriginConsole {
			t.Errorf("Origin = %q, want %q", got.Origin, model.RunOriginConsole)
		}
		if got.OriginSessionID != "sess-console-abc" {
			t.Errorf("OriginSessionID = %q, want %q", got.OriginSessionID, "sess-console-abc")
		}
	})

	t.Run("human_origin_has_empty_session_id", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := pool.Exec(
			`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-origin-human", "Origin Human", t.TempDir(), now, now); err != nil {
			t.Fatalf("seed project: %v", err)
		}
		if _, err := pool.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-origin-human", "proj-origin-human", "", "project", now, now); err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		r := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:         "wfi-origin-human-1",
			ProjectID:  "proj-origin-human",
			WorkflowID: "wf-origin-human",
			ScopeType:  "project",
			Status:     model.WorkflowInstanceActive,
			Origin:     model.RunOriginHuman,
		}
		if err := r.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := r.Get("wfi-origin-human-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Origin != model.RunOriginHuman {
			t.Errorf("Origin = %q, want %q", got.Origin, model.RunOriginHuman)
		}
		if got.OriginSessionID != "" {
			t.Errorf("OriginSessionID = %q, want empty", got.OriginSessionID)
		}
	})

	t.Run("empty_origin_is_preserved_as_unknown", func(t *testing.T) {
		t.Parallel()
		pool := newTestPool(t)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := pool.Exec(
			`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"proj-origin-unknown", "Origin Unknown", t.TempDir(), now, now); err != nil {
			t.Fatalf("seed project: %v", err)
		}
		if _, err := pool.Exec(
			`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"wf-origin-unknown", "proj-origin-unknown", "", "project", now, now); err != nil {
			t.Fatalf("seed workflow: %v", err)
		}

		r := NewWorkflowInstanceRepo(pool, clock.Real())
		wi := &model.WorkflowInstance{
			ID:         "wfi-origin-unknown-1",
			ProjectID:  "proj-origin-unknown",
			WorkflowID: "wf-origin-unknown",
			ScopeType:  "project",
			Status:     model.WorkflowInstanceActive,
		}
		if err := r.Create(wi); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := r.Get("wfi-origin-unknown-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Origin != "" {
			t.Errorf("Origin = %q, want empty (unknown)", got.Origin)
		}
		if got.OriginSessionID != "" {
			t.Errorf("OriginSessionID = %q, want empty", got.OriginSessionID)
		}
	})
}
