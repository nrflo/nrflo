package repo

import (
	"database/sql"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestListByParent(t *testing.T) {
	// Create temporary database
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project first (FK constraint)
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)

	// Create an epic ticket
	epic := &model.Ticket{
		ID:        "EPIC-001",
		ProjectID: "test-proj",
		Title:     "Epic Ticket",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeEpic,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(epic); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create child tickets with different priorities
	child1 := &model.Ticket{
		ID:             "CHILD-001",
		ProjectID:      "test-proj",
		Title:          "Child 1",
		Status:         model.StatusOpen,
		Priority:       3,
		IssueType:      model.IssueTypeTask,
		ParentTicketID: sql.NullString{String: "epic-001", Valid: true},
		CreatedBy:      "tester",
	}
	if err := ticketRepo.Create(child1); err != nil {
		t.Fatalf("failed to create child 1: %v", err)
	}

	child2 := &model.Ticket{
		ID:             "CHILD-002",
		ProjectID:      "test-proj",
		Title:          "Child 2",
		Status:         model.StatusOpen,
		Priority:       1,
		IssueType:      model.IssueTypeTask,
		ParentTicketID: sql.NullString{String: "epic-001", Valid: true},
		CreatedBy:      "tester",
	}
	if err := ticketRepo.Create(child2); err != nil {
		t.Fatalf("failed to create child 2: %v", err)
	}

	// Test ListByParent
	children, err := ticketRepo.ListByParent("test-proj", "epic-001")
	if err != nil {
		t.Fatalf("ListByParent failed: %v", err)
	}

	// Verify count
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Verify ordering: priority ASC, so child2 (priority 1) should come first
	if children[0].ID != "child-002" {
		t.Errorf("expected first child to be child-002, got %q", children[0].ID)
	}
	if children[1].ID != "child-001" {
		t.Errorf("expected second child to be child-001, got %q", children[1].ID)
	}

	// Verify child details
	if children[0].Priority != 1 {
		t.Errorf("expected first child priority 1, got %d", children[0].Priority)
	}
	if children[1].Priority != 3 {
		t.Errorf("expected second child priority 3, got %d", children[1].Priority)
	}
}

func TestListByParentNoChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project first (FK constraint)
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)

	// Create an epic with no children
	epic := &model.Ticket{
		ID:        "LONELY-EPIC",
		ProjectID: "test-proj",
		Title:     "Lonely Epic",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeEpic,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(epic); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Test ListByParent
	children, err := ticketRepo.ListByParent("test-proj", "lonely-epic")
	if err != nil {
		t.Fatalf("ListByParent failed: %v", err)
	}

	// Repo returns nil for no results (converted to [] by handler)
	if len(children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(children))
	}
}

func TestListByParentCaseInsensitive(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project first (FK constraint)
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)

	// Create epic with mixed case
	epic := &model.Ticket{
		ID:        "MixedCase-Epic",
		ProjectID: "test-proj",
		Title:     "Case Test",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeEpic,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(epic); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create child with different case in parent_ticket_id
	child := &model.Ticket{
		ID:             "CHILD",
		ProjectID:      "test-proj",
		Title:          "Child",
		Status:         model.StatusOpen,
		Priority:       1,
		IssueType:      model.IssueTypeTask,
		ParentTicketID: sql.NullString{String: "MIXEDCASE-EPIC", Valid: true},
		CreatedBy:      "tester",
	}
	if err := ticketRepo.Create(child); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Query with different case
	children, err := ticketRepo.ListByParent("test-proj", "mixedcase-epic")
	if err != nil {
		t.Fatalf("ListByParent failed: %v", err)
	}

	if len(children) != 1 {
		t.Fatalf("expected 1 child (case-insensitive), got %d", len(children))
	}
	if children[0].ID != "child" {
		t.Errorf("expected child ID 'child', got %q", children[0].ID)
	}
}
