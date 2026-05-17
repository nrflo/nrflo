package repo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

type artifactTestEnv struct {
	repo      *ArtifactRepo
	pool      *db.Pool
	clk       *clock.TestClock
	projectID string
	wfID      string
	wfiID     string
}

func newArtifactTestEnv(t *testing.T) *artifactTestEnv {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC))
	const (
		projectID = "proj-art"
		wfID      = "wf-art"
		wfiID     = "wfi-art"
	)
	seedArtifactParents(t, pool, projectID, wfID, wfiID)
	return &artifactTestEnv{
		repo:      NewArtifactRepo(pool, clk),
		pool:      pool,
		clk:       clk,
		projectID: projectID,
		wfID:      wfID,
		wfiID:     wfiID,
	}
}

func seedArtifactParents(t *testing.T, pool *db.Pool, projectID, wfID, wfiID string) {
	t.Helper()
	findings, _ := json.Marshal(map[string]any{})
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		projectID, "Art Project", "/tmp/art",
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		wfID, projectID, "Workflow", "ticket",
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		wfiID, projectID, "TKT-1", wfID, model.WorkflowInstanceCompleted, "ticket", string(findings),
	); err != nil {
		t.Fatalf("seed workflow_instance: %v", err)
	}
}

func makeArtifact(id, name, projectID, wfiID string) *model.Artifact {
	return &model.Artifact{
		ID:                 id,
		ProjectID:          projectID,
		WorkflowInstanceID: wfiID,
		Name:               name,
		Type:               model.ArtifactTypeInternal,
		PathKey:            wfiID + "/" + id + "__" + name,
		SizeBytes:          100,
		Source:             model.ArtifactSourceAgent,
	}
}

func TestArtifactRepo_CreateGet(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)

	a := makeArtifact("art-cg1", "report.txt", env.projectID, env.wfiID)
	a.ContentType = "text/plain"
	a.CreatedBySession = "sess-1"

	if err := env.repo.Create(a); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	got, err := env.repo.Get(a.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.ID != a.ID {
		t.Errorf("ID = %q, want %q", got.ID, a.ID)
	}
	if got.Name != a.Name {
		t.Errorf("Name = %q, want %q", got.Name, a.Name)
	}
	if got.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", got.ContentType)
	}
	if got.CreatedBySession != "sess-1" {
		t.Errorf("CreatedBySession = %q, want sess-1", got.CreatedBySession)
	}
	if got.ProjectID != env.projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, env.projectID)
	}
	if got.WorkflowInstanceID != env.wfiID {
		t.Errorf("WorkflowInstanceID = %q, want %q", got.WorkflowInstanceID, env.wfiID)
	}
	if got.SizeBytes != 100 {
		t.Errorf("SizeBytes = %d, want 100", got.SizeBytes)
	}
	if got.Type != model.ArtifactTypeInternal {
		t.Errorf("Type = %q, want %q", got.Type, model.ArtifactTypeInternal)
	}
	if got.Source != model.ArtifactSourceAgent {
		t.Errorf("Source = %q, want %q", got.Source, model.ArtifactSourceAgent)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestArtifactRepo_GetMissing(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)
	got, err := env.repo.Get("no-such-id")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != nil {
		t.Errorf("Get() = %+v, want nil for missing id", got)
	}
}

func TestArtifactRepo_UniqueConstraint(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)

	a := makeArtifact("art-uc1", "dup.txt", env.projectID, env.wfiID)
	if err := env.repo.Create(a); err != nil {
		t.Fatalf("Create() first: %v", err)
	}
	a2 := makeArtifact("art-uc2", "dup.txt", env.projectID, env.wfiID)
	if err := env.repo.Create(a2); err == nil {
		t.Fatal("Create() duplicate (wfi_id, name) expected UNIQUE error, got nil")
	}
}

func TestArtifactRepo_ExistsByNameForInstance(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)

	ok, err := env.repo.ExistsByNameForInstance(env.wfiID, "check.txt")
	if err != nil {
		t.Fatalf("ExistsByNameForInstance() before insert: %v", err)
	}
	if ok {
		t.Error("ExistsByNameForInstance() = true before insert, want false")
	}

	a := makeArtifact("art-ex1", "check.txt", env.projectID, env.wfiID)
	if err := env.repo.Create(a); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	ok, err = env.repo.ExistsByNameForInstance(env.wfiID, "check.txt")
	if err != nil {
		t.Fatalf("ExistsByNameForInstance() after insert: %v", err)
	}
	if !ok {
		t.Error("ExistsByNameForInstance() = false after insert, want true")
	}
}

func TestArtifactRepo_Delete(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)

	a := makeArtifact("art-del1", "todelete.txt", env.projectID, env.wfiID)
	if err := env.repo.Create(a); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if err := env.repo.Delete(a.ID); err != nil {
		t.Fatalf("Delete(): %v", err)
	}
	got, err := env.repo.Get(a.ID)
	if err != nil {
		t.Fatalf("Get() after Delete: %v", err)
	}
	if got != nil {
		t.Error("Get() after Delete returned non-nil, want nil")
	}
}

func TestArtifactRepo_DeleteNotFound(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)
	err := env.repo.Delete("no-such-id")
	if err == nil {
		t.Fatal("Delete() missing artifact expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Delete() error = %q, want 'not found'", err.Error())
	}
}

func TestArtifactRepo_List(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)
	findings, _ := json.Marshal(map[string]any{})

	// Seed a second wfi; its artifact must not appear in List(env.wfiID).
	if _, err := env.pool.Exec(
		`INSERT INTO workflow_instances (id,project_id,ticket_id,workflow_id,status,scope_type,findings,created_at,updated_at) VALUES (?,?,?,?,?,?,?,datetime('now'),datetime('now'))`,
		"wfi-other", env.projectID, "TKT-2", env.wfID, model.WorkflowInstanceCompleted, "ticket", string(findings),
	); err != nil {
		t.Fatalf("seed wfi-other: %v", err)
	}
	other := makeArtifact("art-other", "other.txt", env.projectID, "wfi-other")
	if err := env.repo.Create(other); err != nil {
		t.Fatalf("Create(other): %v", err)
	}

	names := []string{"alpha.txt", "beta.txt", "gamma.txt"}
	for i, name := range names {
		a := makeArtifact("art-l"+string(rune('1'+i)), name, env.projectID, env.wfiID)
		if err := env.repo.Create(a); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		env.clk.Advance(time.Second)
	}

	list, err := env.repo.List(env.wfiID)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3 (only env.wfiID)", len(list))
	}
	for i, want := range names {
		if list[i].Name != want {
			t.Errorf("list[%d].Name = %q, want %q (created_at ASC)", i, list[i].Name, want)
		}
	}
}

func TestArtifactRepo_ListByProject(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewArtifactRepo(pool, clk)
	findings, _ := json.Marshal(map[string]any{})

	for _, pid := range []string{"lbp-a", "lbp-b"} {
		if _, err := pool.Exec(`INSERT INTO projects (id,name,root_path,created_at,updated_at) VALUES (?,?,?,datetime('now'),datetime('now'))`, pid, pid, "/tmp"); err != nil {
			t.Fatalf("seed project %s: %v", pid, err)
		}
		if _, err := pool.Exec(`INSERT INTO workflows (id,project_id,description,scope_type,created_at,updated_at) VALUES (?,?,?,?,datetime('now'),datetime('now'))`, "wf-"+pid, pid, "W", "ticket"); err != nil {
			t.Fatalf("seed wf %s: %v", pid, err)
		}
		if _, err := pool.Exec(
			`INSERT INTO workflow_instances (id,project_id,ticket_id,workflow_id,status,scope_type,findings,created_at,updated_at) VALUES (?,?,?,?,?,?,?,datetime('now'),datetime('now'))`,
			"wfi-"+pid, pid, "TKT", "wf-"+pid, model.WorkflowInstanceCompleted, "ticket", string(findings),
		); err != nil {
			t.Fatalf("seed wfi %s: %v", pid, err)
		}
	}

	for _, a := range []*model.Artifact{
		makeArtifact("art-a1", "a1.txt", "lbp-a", "wfi-lbp-a"),
		makeArtifact("art-a2", "a2.txt", "lbp-a", "wfi-lbp-a"),
		makeArtifact("art-b1", "b1.txt", "lbp-b", "wfi-lbp-b"),
	} {
		if err := r.Create(a); err != nil {
			t.Fatalf("Create(%s): %v", a.ID, err)
		}
		clk.Advance(time.Second)
	}

	listA, err := r.ListByProject("lbp-a")
	if err != nil {
		t.Fatalf("ListByProject(lbp-a): %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("ListByProject(lbp-a) = %d items, want 2", len(listA))
	}
	listB, err := r.ListByProject("lbp-b")
	if err != nil {
		t.Fatalf("ListByProject(lbp-b): %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("ListByProject(lbp-b) = %d items, want 1", len(listB))
	}
}

func TestArtifactRepo_CascadeOnWFIDelete(t *testing.T) {
	t.Parallel()
	env := newArtifactTestEnv(t)

	a := makeArtifact("art-cas1", "cascade.txt", env.projectID, env.wfiID)
	if err := env.repo.Create(a); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, err := env.pool.Exec(`DELETE FROM workflow_instances WHERE id = ?`, env.wfiID); err != nil {
		t.Fatalf("delete wfi: %v", err)
	}
	list, err := env.repo.List(env.wfiID)
	if err != nil {
		t.Fatalf("List() after cascade delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() after wfi delete = %d items, want 0 (cascade)", len(list))
	}
}
