package repo

import (
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestGetBlockersWithTitles(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)
	depRepo := NewDependencyRepo(database)

	// Create blocker ticket
	blocker := &model.Ticket{
		ID:        "BLOCKER-001",
		ProjectID: "test-proj",
		Title:     "Blocker Ticket",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Create blocked ticket
	blocked := &model.Ticket{
		ID:        "BLOCKED-001",
		ProjectID: "test-proj",
		Title:     "Blocked Ticket",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocked); err != nil {
		t.Fatalf("failed to create blocked ticket: %v", err)
	}

	// Create dependency
	dep := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "blocked-001",
		DependsOnID: "blocker-001",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep); err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}

	// Get blockers and verify title is populated
	blockers, err := depRepo.GetBlockers("test-proj", "blocked-001")
	if err != nil {
		t.Fatalf("GetBlockers failed: %v", err)
	}

	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blockers))
	}

	if blockers[0].DependsOnTitle != "Blocker Ticket" {
		t.Errorf("expected depends_on_title 'Blocker Ticket', got %q", blockers[0].DependsOnTitle)
	}
	if blockers[0].DependsOnID != "blocker-001" {
		t.Errorf("expected depends_on_id 'blocker-001', got %q", blockers[0].DependsOnID)
	}
}

func TestGetBlockedWithTitles(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)
	depRepo := NewDependencyRepo(database)

	// Create blocker ticket
	blocker := &model.Ticket{
		ID:        "BLOCKER-002",
		ProjectID: "test-proj",
		Title:     "Blocker Ticket 2",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Create blocked ticket
	blocked := &model.Ticket{
		ID:        "BLOCKED-002",
		ProjectID: "test-proj",
		Title:     "Blocked Ticket 2",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocked); err != nil {
		t.Fatalf("failed to create blocked ticket: %v", err)
	}

	// Create dependency (BLOCKED-002 depends on BLOCKER-002)
	dep := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "blocked-002",
		DependsOnID: "blocker-002",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep); err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}

	// Get blocked tickets for blocker and verify issue_title is populated
	blockedList, err := depRepo.GetBlocked("test-proj", "blocker-002")
	if err != nil {
		t.Fatalf("GetBlocked failed: %v", err)
	}

	if len(blockedList) != 1 {
		t.Fatalf("expected 1 blocked ticket, got %d", len(blockedList))
	}

	if blockedList[0].IssueTitle != "Blocked Ticket 2" {
		t.Errorf("expected issue_title 'Blocked Ticket 2', got %q", blockedList[0].IssueTitle)
	}
	if blockedList[0].IssueID != "blocked-002" {
		t.Errorf("expected issue_id 'blocked-002', got %q", blockedList[0].IssueID)
	}
}

func TestGetBlockersDeletedTicket(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)
	depRepo := NewDependencyRepo(database)

	// Create blocker ticket
	blocker := &model.Ticket{
		ID:        "BLOCKER-DEL",
		ProjectID: "test-proj",
		Title:     "Blocker to Delete",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Create blocked ticket
	blocked := &model.Ticket{
		ID:        "BLOCKED-DEL",
		ProjectID: "test-proj",
		Title:     "Blocked Ticket",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocked); err != nil {
		t.Fatalf("failed to create blocked ticket: %v", err)
	}

	// Create dependency
	dep := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "blocked-del",
		DependsOnID: "blocker-del",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep); err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}

	// Delete the blocker ticket directly from DB
	_, err = database.Exec(`DELETE FROM tickets WHERE LOWER(id) = LOWER(?) AND LOWER(project_id) = LOWER(?)`, "blocker-del", "test-proj")
	if err != nil {
		t.Fatalf("failed to delete blocker: %v", err)
	}

	// Get blockers - should handle deleted ticket gracefully
	blockers, err := depRepo.GetBlockers("test-proj", "blocked-del")
	if err != nil {
		t.Fatalf("GetBlockers failed: %v", err)
	}

	if len(blockers) != 1 {
		t.Fatalf("expected 1 blocker (dependency still exists), got %d", len(blockers))
	}

	// COALESCE should return empty string for deleted ticket
	if blockers[0].DependsOnTitle != "" {
		t.Errorf("expected empty depends_on_title for deleted ticket, got %q", blockers[0].DependsOnTitle)
	}
	if blockers[0].DependsOnID != "blocker-del" {
		t.Errorf("expected depends_on_id 'blocker-del', got %q", blockers[0].DependsOnID)
	}
}

func TestGetBlockedCaseInsensitive(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)
	depRepo := NewDependencyRepo(database)

	// Create blocker with mixed case
	blocker := &model.Ticket{
		ID:        "MixedCase-Blocker",
		ProjectID: "test-proj",
		Title:     "Mixed Case Blocker",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Create blocked ticket
	blocked := &model.Ticket{
		ID:        "Blocked-Case",
		ProjectID: "test-proj",
		Title:     "Blocked Case Test",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocked); err != nil {
		t.Fatalf("failed to create blocked ticket: %v", err)
	}

	// Create dependency with different case
	dep := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "BLOCKED-CASE",
		DependsOnID: "MIXEDCASE-BLOCKER",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep); err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}

	// Query with different case
	blockedList, err := depRepo.GetBlocked("test-proj", "mixedcase-blocker")
	if err != nil {
		t.Fatalf("GetBlocked failed: %v", err)
	}

	if len(blockedList) != 1 {
		t.Fatalf("expected 1 blocked ticket (case-insensitive), got %d", len(blockedList))
	}

	if blockedList[0].IssueTitle != "Blocked Case Test" {
		t.Errorf("expected issue_title 'Blocked Case Test', got %q", blockedList[0].IssueTitle)
	}
}

func TestGetBlockersMultipleBlockers(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "test-proj", "Test Project")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	ticketRepo := NewTicketRepo(database)
	depRepo := NewDependencyRepo(database)

	// Create multiple blocker tickets
	blocker1 := &model.Ticket{
		ID:        "BLOCKER-1",
		ProjectID: "test-proj",
		Title:     "First Blocker",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker1); err != nil {
		t.Fatalf("failed to create blocker1: %v", err)
	}

	blocker2 := &model.Ticket{
		ID:        "BLOCKER-2",
		ProjectID: "test-proj",
		Title:     "Second Blocker",
		Status:    model.StatusOpen,
		Priority:  1,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocker2); err != nil {
		t.Fatalf("failed to create blocker2: %v", err)
	}

	// Create blocked ticket
	blocked := &model.Ticket{
		ID:        "BLOCKED-MULTI",
		ProjectID: "test-proj",
		Title:     "Blocked by Multiple",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.IssueTypeTask,
		CreatedBy: "tester",
	}
	if err := ticketRepo.Create(blocked); err != nil {
		t.Fatalf("failed to create blocked ticket: %v", err)
	}

	// Create dependencies
	dep1 := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "blocked-multi",
		DependsOnID: "blocker-1",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep1); err != nil {
		t.Fatalf("failed to create dependency 1: %v", err)
	}

	dep2 := &model.Dependency{
		ProjectID:   "test-proj",
		IssueID:     "blocked-multi",
		DependsOnID: "blocker-2",
		Type:        "blocks",
		CreatedBy:   "tester",
	}
	if err := depRepo.Create(dep2); err != nil {
		t.Fatalf("failed to create dependency 2: %v", err)
	}

	// Get blockers and verify all are returned with titles
	blockers, err := depRepo.GetBlockers("test-proj", "blocked-multi")
	if err != nil {
		t.Fatalf("GetBlockers failed: %v", err)
	}

	if len(blockers) != 2 {
		t.Fatalf("expected 2 blockers, got %d", len(blockers))
	}

	// Verify both blockers have titles
	titleMap := make(map[string]string)
	for _, b := range blockers {
		titleMap[b.DependsOnID] = b.DependsOnTitle
	}

	if titleMap["blocker-1"] != "First Blocker" {
		t.Errorf("expected 'First Blocker' for blocker-1, got %q", titleMap["blocker-1"])
	}
	if titleMap["blocker-2"] != "Second Blocker" {
		t.Errorf("expected 'Second Blocker' for blocker-2, got %q", titleMap["blocker-2"])
	}
}
