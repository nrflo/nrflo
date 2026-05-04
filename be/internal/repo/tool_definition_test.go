package repo

import (
	"be/internal/db"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func setupToolDefRepo(t *testing.T) (*ToolDefinitionRepo, *db.Pool) {
	t.Helper()
	pool := newTestPool(t)
	return NewToolDefinitionRepo(pool, clock.Real()), pool
}

func mkTool(id, name string) *model.ToolDefinition {
	return &model.ToolDefinition{
		ID:          id,
		Name:        name,
		Description: "desc " + name,
		InputSchema: []byte(`{"type":"object"}`),
		Endpoint:    "http://example/" + name,
	}
}

// TestToolDefinitionRepo_CreateGet round-trips via the repo.
func TestToolDefinitionRepo_CreateGet(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)

	tool := mkTool("t1", "echo")
	if err := r.Create(tool); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tool.AuthMethod != "none" {
		t.Errorf("default AuthMethod = %q, want none", tool.AuthMethod)
	}
	if tool.TimeoutSec != 30 {
		t.Errorf("default TimeoutSec = %d, want 30", tool.TimeoutSec)
	}

	got, err := r.Get("t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "echo" {
		t.Errorf("Name = %q, want echo", got.Name)
	}
	if string(got.InputSchema) != `{"type":"object"}` {
		t.Errorf("InputSchema = %q, want JSON", string(got.InputSchema))
	}
	if got.AuthMethod != "none" {
		t.Errorf("AuthMethod = %q, want none", got.AuthMethod)
	}
	if got.TimeoutSec != 30 {
		t.Errorf("TimeoutSec = %d, want 30", got.TimeoutSec)
	}
}

func TestToolDefinitionRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)
	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("Get(missing): want error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want contains 'not found'", err)
	}
}

// TestToolDefinitionRepo_ListByProject_NullsIncluded — both per-project rows and
// global (NULL project_id) rows are returned for a given project_id filter.
func TestToolDefinitionRepo_ListByProject_NullsIncluded(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)

	a := mkTool("ta", "alpha")
	b := mkTool("tb", "beta")
	c := mkTool("tc", "gamma")
	pa := "p1"
	pb := "p2"
	a.ProjectID = &pa
	b.ProjectID = &pb
	// c is global (ProjectID nil)

	for _, tool := range []*model.ToolDefinition{a, b, c} {
		if err := r.Create(tool); err != nil {
			t.Fatalf("Create %s: %v", tool.ID, err)
		}
	}

	got, err := r.ListByProject("p1")
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	// Should include p1 and global; not p2.
	names := map[string]bool{}
	for _, x := range got {
		names[x.Name] = true
	}
	if !names["alpha"] || !names["gamma"] {
		t.Errorf("ListByProject(p1) names = %v, want alpha+gamma", names)
	}
	if names["beta"] {
		t.Errorf("ListByProject(p1) includes beta (p2)")
	}
}

func TestToolDefinitionRepo_ListByWorkflow(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)

	a := mkTool("ta", "alpha")
	b := mkTool("tb", "beta")
	wfa := "wf-1"
	wfb := "wf-2"
	a.WorkflowID = &wfa
	b.WorkflowID = &wfb
	for _, x := range []*model.ToolDefinition{a, b} {
		if err := r.Create(x); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	got, err := r.ListByWorkflow("wf-1")
	if err != nil {
		t.Fatalf("ListByWorkflow: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("ListByWorkflow(wf-1) = %v, want [alpha]", got)
	}
}

func TestToolDefinitionRepo_Update(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)
	if err := r.Create(mkTool("t1", "echo")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newDesc := "new description"
	newSchema := `{"type":"object","required":["x"]}`
	if err := r.Update("t1", &ToolDefUpdateFields{Description: &newDesc, InputSchema: &newSchema}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := r.Get("t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != newDesc {
		t.Errorf("Description = %q, want %q", got.Description, newDesc)
	}
	if string(got.InputSchema) != newSchema {
		t.Errorf("InputSchema = %q, want %q", string(got.InputSchema), newSchema)
	}
}

func TestToolDefinitionRepo_Update_NotFound(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)
	v := "x"
	err := r.Update("nope", &ToolDefUpdateFields{Description: &v})
	if err == nil {
		t.Fatal("Update(nope): want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

func TestToolDefinitionRepo_Delete(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)
	if err := r.Create(mkTool("t1", "echo")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Delete("t1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get("t1"); err == nil {
		t.Fatal("expected not found after Delete")
	}
	if err := r.Delete("t1"); err == nil {
		t.Fatal("Delete twice: want error second time")
	}
}

func TestToolDefinitionRepo_UniqueName(t *testing.T) {
	t.Parallel()
	r, _ := setupToolDefRepo(t)
	if err := r.Create(mkTool("t1", "echo")); err != nil {
		t.Fatalf("Create t1: %v", err)
	}
	err := r.Create(mkTool("t2", "echo"))
	if err == nil {
		t.Fatal("Create with duplicate name: want UNIQUE error")
	}
	if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("error = %v, want UNIQUE", err)
	}
}
