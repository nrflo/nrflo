package repo

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/types"
)

func makePythonScriptTool(projectID, id, name string) *model.PythonScript {
	return &model.PythonScript{
		ID:              id,
		ProjectID:       projectID,
		Name:            name,
		Description:     "Tool description",
		Code:            "",
		Kind:            "tool",
		ToolDescription: "Does something useful",
		InputSchema:     `{"type":"object","properties":{"x":{"type":"string"}}}`,
		TimeoutSec:      30,
	}
}

func TestPythonScriptRepo_KindRoundTrip(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	tool := makePythonScriptTool(projectID, "ps-toolrt", "My Tool")
	if err := r.Create(tool); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := r.Get(projectID, "ps-toolrt")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.Kind != "tool" {
		t.Errorf("Kind = %q, want tool", got.Kind)
	}
	if got.ToolDescription != tool.ToolDescription {
		t.Errorf("ToolDescription = %q, want %q", got.ToolDescription, tool.ToolDescription)
	}
	if got.InputSchema != tool.InputSchema {
		t.Errorf("InputSchema = %q, want %q", got.InputSchema, tool.InputSchema)
	}
	if got.TimeoutSec != tool.TimeoutSec {
		t.Errorf("TimeoutSec = %d, want %d", got.TimeoutSec, tool.TimeoutSec)
	}
}

func TestPythonScriptRepo_AgentKindRoundTrip(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	agent := &model.PythonScript{
		ID:        "ps-agentrt",
		ProjectID: projectID,
		Name:      "My Agent",
		Kind:      "agent",
		Code:      "print(1)",
	}
	if err := r.Create(agent); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := r.Get(projectID, "ps-agentrt")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.Kind != "agent" {
		t.Errorf("Kind = %q, want agent", got.Kind)
	}
}

func TestPythonScriptRepo_ListByKind(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	scripts := []*model.PythonScript{
		{ID: "ps-a1", ProjectID: projectID, Name: "Agent One", Kind: "agent"},
		{ID: "ps-a2", ProjectID: projectID, Name: "Agent Two", Kind: "agent"},
		{ID: "ps-t1", ProjectID: projectID, Name: "Tool Alpha", Kind: "tool",
			ToolDescription: "does x", InputSchema: "{}", TimeoutSec: 30},
	}
	for _, s := range scripts {
		if err := r.Create(s); err != nil {
			t.Fatalf("Create(%s): %v", s.ID, err)
		}
	}

	agents, err := r.ListByKind(projectID, "agent")
	if err != nil {
		t.Fatalf("ListByKind(agent): %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("ListByKind(agent) = %d items, want 2", len(agents))
	}
	for _, s := range agents {
		if s.Kind != "agent" {
			t.Errorf("ListByKind(agent): got kind=%q, want agent", s.Kind)
		}
	}

	tools, err := r.ListByKind(projectID, "tool")
	if err != nil {
		t.Fatalf("ListByKind(tool): %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("ListByKind(tool) = %d items, want 1", len(tools))
	}
	if len(tools) > 0 && tools[0].Kind != "tool" {
		t.Errorf("ListByKind(tool): got kind=%q, want tool", tools[0].Kind)
	}

	all, err := r.List(projectID)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List() = %d items, want 3", len(all))
	}
}

func TestPythonScriptRepo_PartialUniqueIndex_ToolDuplicate(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-uix', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	r := NewPythonScriptRepo(pool, clock.Real())

	s1 := makePythonScriptTool("proj-uix", "ps-uidup1", "duplicated")
	s2 := makePythonScriptTool("proj-uix", "ps-uidup2", "duplicated")

	if err := r.Create(s1); err != nil {
		t.Fatalf("first Create(): %v", err)
	}
	err := r.Create(s2)
	if err == nil {
		t.Error("second Create() same name+project+tool: expected UNIQUE error, got nil")
	} else if !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("error = %v, want UNIQUE constraint", err)
	}
}

func TestPythonScriptRepo_PartialUniqueIndex_AgentAndToolSameName(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES ('proj-uixt', 'P', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	r := NewPythonScriptRepo(pool, clock.Real())

	agent := &model.PythonScript{
		ID: "ps-agname", ProjectID: "proj-uixt", Name: "shared", Kind: "agent", Code: "x=1",
	}
	tool := makePythonScriptTool("proj-uixt", "ps-toolname", "shared")

	if err := r.Create(agent); err != nil {
		t.Fatalf("Create(agent): %v", err)
	}
	if err := r.Create(tool); err != nil {
		t.Errorf("Create(tool) after agent same name+project: unexpected error: %v", err)
	}
}

func TestPythonScriptRepo_PartialUniqueIndex_DifferentProjects(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	for _, pid := range []string{"proj-xa", "proj-xb"} {
		if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
			VALUES (?, 'P', datetime('now'), datetime('now'))`, pid); err != nil {
			t.Fatalf("seed %s: %v", pid, err)
		}
	}
	r := NewPythonScriptRepo(pool, clock.Real())

	t1 := makePythonScriptTool("proj-xa", "ps-xat1", "sameName")
	t2 := makePythonScriptTool("proj-xb", "ps-xbt1", "sameName")

	if err := r.Create(t1); err != nil {
		t.Fatalf("Create(proj-xa): %v", err)
	}
	if err := r.Create(t2); err != nil {
		t.Errorf("Create(proj-xb) same tool name, different project: unexpected error: %v", err)
	}
}

func TestPythonScriptRepo_UpdateToolFields(t *testing.T) {
	t.Parallel()
	r, projectID := setupPythonScriptTestDB(t)

	tool := makePythonScriptTool(projectID, "ps-tupd", "Tool")
	if err := r.Create(tool); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	newDesc := "Updated tool description"
	newSchema := `{"type":"object","properties":{"y":{"type":"integer"}}}`
	newTimeout := 60
	req := &types.PythonScriptUpdateRequest{
		ToolDescription: &newDesc,
		InputSchema:     &newSchema,
		TimeoutSec:      &newTimeout,
	}
	if err := r.Update(projectID, "ps-tupd", req); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got, err := r.Get(projectID, "ps-tupd")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.ToolDescription != newDesc {
		t.Errorf("ToolDescription = %q, want %q", got.ToolDescription, newDesc)
	}
	if got.InputSchema != newSchema {
		t.Errorf("InputSchema = %q, want %q", got.InputSchema, newSchema)
	}
	if got.TimeoutSec != newTimeout {
		t.Errorf("TimeoutSec = %d, want %d", got.TimeoutSec, newTimeout)
	}
}
