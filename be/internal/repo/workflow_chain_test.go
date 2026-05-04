package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func setupChainDB(t *testing.T) (*WorkflowChainRepo, *WorkflowChainStepRepo, string) {
	t.Helper()
	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewWorkflowChainRepo(database, clock.Real()),
		NewWorkflowChainStepRepo(database, clock.Real()),
		"proj-1"
}

func makeChain(id, projectID, name string) *model.WorkflowChain {
	return &model.WorkflowChain{
		ID:          id,
		ProjectID:   projectID,
		Name:        name,
		Description: "test chain description",
	}
}

func makeStep(id, projectID, chainID string, position int) *model.WorkflowChainStep {
	return &model.WorkflowChainStep{
		ID:                   id,
		ProjectID:            projectID,
		ChainID:              chainID,
		Position:             position,
		WorkflowName:         "feature",
		ScopeType:            "ticket",
		BaseInstructions:     "do the thing",
		RequireTicketHandoff: false,
	}
}

func TestWorkflowChainRepo_Create_Get(t *testing.T) {
	t.Parallel()
	r, _, projectID := setupChainDB(t)

	c := makeChain("chain-1", projectID, "My Chain")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	got, err := r.GetChain(projectID, "chain-1")
	if err != nil {
		t.Fatalf("GetChain: %v", err)
	}
	if got.ID != "chain-1" {
		t.Errorf("ID = %q, want chain-1", got.ID)
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.Name != "My Chain" {
		t.Errorf("Name = %q, want My Chain", got.Name)
	}
	if got.Description != "test chain description" {
		t.Errorf("Description = %q, want test chain description", got.Description)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero")
	}
}

func TestWorkflowChainRepo_Create_Get_CaseInsensitive(t *testing.T) {
	t.Parallel()
	r, _, projectID := setupChainDB(t)

	c := makeChain("CHAIN-UPPER", projectID, "Upper Chain")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}

	got, err := r.GetChain(projectID, "chain-upper")
	if err != nil {
		t.Fatalf("GetChain lower: %v", err)
	}
	if got.ID != "chain-upper" {
		t.Errorf("ID = %q, want chain-upper", got.ID)
	}

	got2, err := r.GetChain(projectID, "CHAIN-UPPER")
	if err != nil {
		t.Fatalf("GetChain upper: %v", err)
	}
	if got2.ID != "chain-upper" {
		t.Errorf("ID = %q, want chain-upper", got2.ID)
	}

	got3, err := r.GetChain("PROJ-1", "chain-upper")
	if err != nil {
		t.Fatalf("GetChain project upper: %v", err)
	}
	if got3.ID != "chain-upper" {
		t.Errorf("ID = %q, want chain-upper", got3.ID)
	}
}

func TestWorkflowChainRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	r, _, projectID := setupChainDB(t)

	_, err := r.GetChain(projectID, "no-such-chain")
	if err == nil {
		t.Fatalf("GetChain missing: expected error, got nil")
	}
}

func TestWorkflowChainRepo_ListChains_FiltersByProject(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	for _, id := range []string{"proj-a", "proj-b"} {
		_, err := database.Exec(
			`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, id)
		if err != nil {
			t.Fatalf("insert project %s: %v", id, err)
		}
	}
	r := NewWorkflowChainRepo(database, clock.Real())

	if err := r.CreateChain(makeChain("c-a1", "proj-a", "A1")); err != nil {
		t.Fatalf("CreateChain a1: %v", err)
	}
	if err := r.CreateChain(makeChain("c-a2", "proj-a", "A2")); err != nil {
		t.Fatalf("CreateChain a2: %v", err)
	}
	if err := r.CreateChain(makeChain("c-b1", "proj-b", "B1")); err != nil {
		t.Fatalf("CreateChain b1: %v", err)
	}

	listA, err := r.ListChains("proj-a")
	if err != nil {
		t.Fatalf("ListChains proj-a: %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("ListChains proj-a count = %d, want 2", len(listA))
	}

	listB, err := r.ListChains("proj-b")
	if err != nil {
		t.Fatalf("ListChains proj-b: %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("ListChains proj-b count = %d, want 1", len(listB))
	}

	listNone, err := r.ListChains("proj-none")
	if err != nil {
		t.Fatalf("ListChains proj-none: %v", err)
	}
	if len(listNone) != 0 {
		t.Errorf("ListChains proj-none count = %d, want 0", len(listNone))
	}
}

func TestWorkflowChainRepo_Update_MutatesFields(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	_, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	r := NewWorkflowChainRepo(database, clk)

	c := makeChain("chain-upd", "p1", "Original Name")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	originalUpdatedAt := c.UpdatedAt

	clk.Advance(time.Second)

	c.Name = "Updated Name"
	c.Description = "Updated description"
	if err := r.UpdateChain(c); err != nil {
		t.Fatalf("UpdateChain: %v", err)
	}

	got, err := r.GetChain("p1", "chain-upd")
	if err != nil {
		t.Fatalf("GetChain after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name = %q, want Updated Name", got.Name)
	}
	if got.Description != "Updated description" {
		t.Errorf("Description = %q, want Updated description", got.Description)
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt %v not after original %v", got.UpdatedAt, originalUpdatedAt)
	}
}

func TestWorkflowChainRepo_Update_NotFound(t *testing.T) {
	t.Parallel()
	r, _, projectID := setupChainDB(t)

	c := makeChain("no-such", projectID, "X")
	if err := r.UpdateChain(c); err == nil {
		t.Fatalf("UpdateChain non-existent: expected error, got nil")
	}
}

func TestWorkflowChainRepo_DeleteChain(t *testing.T) {
	t.Parallel()
	r, _, projectID := setupChainDB(t)

	c := makeChain("chain-del", projectID, "Del")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	if err := r.DeleteChain(projectID, "chain-del"); err != nil {
		t.Fatalf("DeleteChain first: %v", err)
	}
	if err := r.DeleteChain(projectID, "chain-del"); err == nil {
		t.Fatalf("DeleteChain second: expected error, got nil")
	}
}

func TestWorkflowChainRepo_DeleteChain_CascadesToSteps(t *testing.T) {
	t.Parallel()
	r, sr, projectID := setupChainDB(t)

	c := makeChain("chain-cascade", projectID, "Cascade")
	if err := r.CreateChain(c); err != nil {
		t.Fatalf("CreateChain: %v", err)
	}
	for i, sid := range []string{"s1", "s2", "s3"} {
		if err := sr.UpsertStep(makeStep(sid, projectID, "chain-cascade", i)); err != nil {
			t.Fatalf("UpsertStep %s: %v", sid, err)
		}
	}

	stepsBeforeDelete, err := sr.ListSteps("chain-cascade")
	if err != nil {
		t.Fatalf("ListSteps before delete: %v", err)
	}
	if len(stepsBeforeDelete) != 3 {
		t.Errorf("ListSteps before delete count = %d, want 3", len(stepsBeforeDelete))
	}

	if err := r.DeleteChain(projectID, "chain-cascade"); err != nil {
		t.Fatalf("DeleteChain: %v", err)
	}

	stepsAfterDelete, err := sr.ListSteps("chain-cascade")
	if err != nil {
		t.Fatalf("ListSteps after delete: %v", err)
	}
	if len(stepsAfterDelete) != 0 {
		t.Errorf("ListSteps after delete count = %d, want 0 (cascade expected)", len(stepsAfterDelete))
	}
}
