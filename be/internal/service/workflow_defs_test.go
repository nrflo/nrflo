package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupWorkflowDefsTestEnv creates an isolated DB for workflow def tests.
func setupWorkflowDefsTestEnv(t *testing.T) (*db.Pool, *WorkflowService) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wf_defs_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj1", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	svc := NewWorkflowService(pool, clock.Real())
	return pool, svc
}

// makePhases returns a valid single-phase JSON for use in tests.
func makePhases(t *testing.T) json.RawMessage {
	t.Helper()
	b, _ := json.Marshal([]map[string]interface{}{{"agent": "analyzer", "layer": 0}})
	return b
}

// --- ValidateGroups unit tests ---

func TestValidateGroups(t *testing.T) {
	cases := []struct {
		name    string
		groups  []string
		wantErr bool
		errMsg  string
	}{
		{"empty slice", []string{}, false, ""},
		{"nil slice", nil, false, ""},
		{"valid single", []string{"be"}, false, ""},
		{"valid multiple", []string{"be", "fe", "docs"}, false, ""},
		{"empty string", []string{""}, true, "empty strings"},
		{"whitespace only", []string{"  "}, true, "empty strings"},
		{"duplicate", []string{"be", "be"}, true, "duplicate"},
		{"duplicate in set", []string{"be", "fe", "be"}, true, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGroups(tc.groups)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateGroups(%v) = nil, want error containing %q", tc.groups, tc.errMsg)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateGroups(%v) = %v, want nil", tc.groups, err)
			}
		})
	}
}

// --- CreateWorkflowDef with groups ---

func TestCreateWorkflowDef_WithGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	wf, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf1",
		Phases: phases,
		Groups: []string{"be", "fe", "docs"},
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef with groups: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow")
	}
}

func TestCreateWorkflowDef_NoGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-nogroup",
		Phases: phases,
		// Groups omitted (nil)
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef without groups: %v", err)
	}
}

func TestCreateWorkflowDef_EmptyGroupEntry(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-badgroup",
		Phases: phases,
		Groups: []string{"be", ""},
	})
	if err == nil {
		t.Fatal("expected error for empty group string, got nil")
	}
}

func TestCreateWorkflowDef_DuplicateGroup(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-dupgroup",
		Phases: phases,
		Groups: []string{"be", "be"},
	})
	if err == nil {
		t.Fatal("expected error for duplicate group, got nil")
	}
}

// --- GetWorkflowDef returns groups ---

func TestGetWorkflowDef_ReturnsGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)
	groups := []string{"be", "fe"}

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-get",
		Phases: phases,
		Groups: groups,
	})
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-get")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if len(def.Groups) != 2 {
		t.Errorf("GetWorkflowDef groups: got %v, want [be fe]", def.Groups)
	}
	if def.Groups[0] != "be" || def.Groups[1] != "fe" {
		t.Errorf("GetWorkflowDef groups values: got %v", def.Groups)
	}
}

func TestGetWorkflowDef_EmptyGroupsReturnsSlice(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-nogroup2",
		Phases: phases,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-nogroup2")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if def.Groups == nil {
		t.Error("Groups should be non-nil empty slice, got nil")
	}
	if len(def.Groups) != 0 {
		t.Errorf("expected empty groups, got %v", def.Groups)
	}
}

// --- ListWorkflowDefs returns groups ---

func TestListWorkflowDefs_ReturnsGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-list",
		Phases: phases,
		Groups: []string{"be", "fe"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	def, ok := defs["wf-list"]
	if !ok {
		t.Fatal("wf-list not in result")
	}
	if len(def.Groups) != 2 || def.Groups[0] != "be" {
		t.Errorf("ListWorkflowDefs groups: got %v", def.Groups)
	}
}

// --- UpdateWorkflowDef groups ---

func TestUpdateWorkflowDef_UpdatesGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-upd",
		Phases: phases,
		Groups: []string{"be"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	newGroups := []string{"be", "fe", "docs"}
	if err := svc.UpdateWorkflowDef("proj1", "wf-upd", &types.WorkflowDefUpdateRequest{
		Groups: &newGroups,
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	def, err := svc.GetWorkflowDef("proj1", "wf-upd")
	if err != nil {
		t.Fatalf("GetWorkflowDef after update: %v", err)
	}
	if len(def.Groups) != 3 {
		t.Errorf("expected 3 groups after update, got %v", def.Groups)
	}
}

func TestUpdateWorkflowDef_InvalidGroups(t *testing.T) {
	_, svc := setupWorkflowDefsTestEnv(t)
	phases := makePhases(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:     "wf-inv",
		Phases: phases,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	badGroups := []string{"be", "be"}
	if err := svc.UpdateWorkflowDef("proj1", "wf-inv", &types.WorkflowDefUpdateRequest{
		Groups: &badGroups,
	}); err == nil {
		t.Fatal("expected error for duplicate groups in update, got nil")
	}
}
