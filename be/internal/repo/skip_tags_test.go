// Package repo contains tests for the skip-tags feature (migration 000030).
// This file covers workflow groups repo tests and shared test helpers.
package repo

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// newSkipTagsDB opens a fresh DB (for WorkflowRepo which requires *db.DB).
func newSkipTagsDB(t *testing.T) *db.DB {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// newSkipTagsPool opens a fresh Pool (for WorkflowInstanceRepo and AgentDefinitionRepo).
func newSkipTagsPool(t *testing.T) *db.Pool {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedProjectDB seeds a project into a *db.DB.
func seedProjectDB(t *testing.T, database *db.DB, projectID string) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

// seedProjectPool seeds a project into a Pool.
func seedProjectPool(t *testing.T, pool *db.Pool, projectID string) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Test Project", "/tmp/test")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
}

// seedWorkflowPool seeds a workflow into a Pool.
func seedWorkflowPool(t *testing.T, pool *db.Pool, projectID, workflowID string) {
	t.Helper()
	_, err := pool.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		workflowID, projectID, "Test Workflow", "ticket")
	if err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}

// --- Workflow Groups ---

func TestWorkflowGroupsCreateAndGet(t *testing.T) {
	database := newSkipTagsDB(t)
	seedProjectDB(t, database, "proj")
	repo := NewWorkflowRepo(database, clock.Real())

	cases := []struct {
		name   string
		wfID   string
		groups []string
	}{
		{"empty", "wf-grp-empty", []string{}},
		{"single", "wf-grp-single", []string{"be"}},
		{"multiple", "wf-grp-multi", []string{"be", "fe", "docs"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := &model.Workflow{ID: tc.wfID, ProjectID: "proj", ScopeType: "ticket"}
			wf.SetGroups(tc.groups)

			if err := repo.Create(wf); err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.Get("proj", wf.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			gotGroups := got.GetGroups()
			if len(gotGroups) != len(tc.groups) {
				t.Errorf("GetGroups() = %v, want %v", gotGroups, tc.groups)
				return
			}
			for i, g := range tc.groups {
				if gotGroups[i] != g {
					t.Errorf("GetGroups()[%d] = %q, want %q", i, gotGroups[i], g)
				}
			}
		})
	}
}

func TestWorkflowGroupsUpdate(t *testing.T) {
	database := newSkipTagsDB(t)
	seedProjectDB(t, database, "proj")
	repo := NewWorkflowRepo(database, clock.Real())

	wf := &model.Workflow{ID: "wf-grp-upd", ProjectID: "proj", ScopeType: "ticket"}
	wf.SetGroups([]string{"be"})
	if err := repo.Create(wf); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newGroupsJSON := `["be","fe"]`
	if err := repo.Update("proj", "wf-grp-upd", &WorkflowUpdateFields{Groups: &newGroupsJSON}); err != nil {
		t.Fatalf("Update groups: %v", err)
	}

	got, err := repo.Get("proj", "wf-grp-upd")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if groups := got.GetGroups(); len(groups) != 2 || groups[0] != "be" || groups[1] != "fe" {
		t.Errorf("GetGroups() = %v, want [be fe]", groups)
	}
}

func TestWorkflowGroupsDefaultEmpty(t *testing.T) {
	database := newSkipTagsDB(t)
	seedProjectDB(t, database, "proj")

	_, err := database.Exec(`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"wf-no-groups", "proj", "No groups", "ticket")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := NewWorkflowRepo(database, clock.Real()).Get("proj", "wf-no-groups")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if groups := got.GetGroups(); len(groups) != 0 {
		t.Errorf("groups default = %v, want empty", groups)
	}
}

func TestWorkflowGroupsList(t *testing.T) {
	database := newSkipTagsDB(t)
	seedProjectDB(t, database, "proj")
	repo := NewWorkflowRepo(database, clock.Real())

	wf1 := &model.Workflow{ID: "wf-list-1", ProjectID: "proj", ScopeType: "ticket"}
	wf1.SetGroups([]string{"be", "fe"})
	wf2 := &model.Workflow{ID: "wf-list-2", ProjectID: "proj", ScopeType: "ticket"}
	wf2.SetGroups([]string{"docs"})

	for _, wf := range []*model.Workflow{wf1, wf2} {
		if err := repo.Create(wf); err != nil {
			t.Fatalf("Create %s: %v", wf.ID, err)
		}
	}

	wfs, err := repo.List("proj")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("List() len = %d, want 2", len(wfs))
	}

	byID := make(map[string]*model.Workflow)
	for _, wf := range wfs {
		byID[wf.ID] = wf
	}
	if g := byID["wf-list-1"].GetGroups(); len(g) != 2 {
		t.Errorf("wf-list-1 groups = %v, want [be fe]", g)
	}
	if g := byID["wf-list-2"].GetGroups(); len(g) != 1 || g[0] != "docs" {
		t.Errorf("wf-list-2 groups = %v, want [docs]", g)
	}
}

// --- Workflow and AgentSession model unit tests ---

func TestWorkflowGetSetGroupsRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
	}{
		{"nil becomes empty", nil},
		{"empty slice", []string{}},
		{"single", []string{"be"}},
		{"multiple", []string{"be", "fe", "docs"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := &model.Workflow{}
			wf.SetGroups(tc.groups)
			got := wf.GetGroups()

			want := tc.groups
			if want == nil {
				want = []string{}
			}
			if len(got) != len(want) {
				t.Errorf("GetGroups() = %v, want %v", got, want)
				return
			}
			for i, g := range want {
				if got[i] != g {
					t.Errorf("GetGroups()[%d] = %q, want %q", i, got[i], g)
				}
			}
		})
	}
}

func TestWorkflowGetGroupsEmptyString(t *testing.T) {
	wf := &model.Workflow{Groups: ""}
	if got := wf.GetGroups(); len(got) != 0 {
		t.Errorf("GetGroups() on empty string = %v, want []", got)
	}
}

func TestAgentSessionSkippedConstant(t *testing.T) {
	if model.AgentSessionSkipped != "skipped" {
		t.Errorf("AgentSessionSkipped = %q, want %q", model.AgentSessionSkipped, "skipped")
	}
}
