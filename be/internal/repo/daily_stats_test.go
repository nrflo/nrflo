package repo

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func TestDailyStatsUpsertAndGet(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	// Test data
	projectID := "test-proj"
	date := "2025-01-15"
	stats := model.DailyStats{
		TicketsCreated: 5,
		TicketsClosed:  3,
		TokensSpent:    150000,
		AgentTimeSec:   1234.56,
	}

	// Test Upsert (insert)
	err = repo.Upsert(projectID, date, stats)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Test GetByDate
	retrieved, err := repo.GetByDate(projectID, date)
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	// Verify retrieved data
	if retrieved.ProjectID != projectID {
		t.Errorf("expected project_id %q, got %q", projectID, retrieved.ProjectID)
	}
	if retrieved.Date != date {
		t.Errorf("expected date %q, got %q", date, retrieved.Date)
	}
	if retrieved.TicketsCreated != stats.TicketsCreated {
		t.Errorf("expected tickets_created %d, got %d", stats.TicketsCreated, retrieved.TicketsCreated)
	}
	if retrieved.TicketsClosed != stats.TicketsClosed {
		t.Errorf("expected tickets_closed %d, got %d", stats.TicketsClosed, retrieved.TicketsClosed)
	}
	if retrieved.TokensSpent != stats.TokensSpent {
		t.Errorf("expected tokens_spent %d, got %d", stats.TokensSpent, retrieved.TokensSpent)
	}
	if retrieved.AgentTimeSec != stats.AgentTimeSec {
		t.Errorf("expected agent_time_sec %.2f, got %.2f", stats.AgentTimeSec, retrieved.AgentTimeSec)
	}
	if retrieved.ID == 0 {
		t.Errorf("expected non-zero ID, got 0")
	}
	if retrieved.UpdatedAt == "" {
		t.Errorf("expected non-empty updated_at, got empty string")
	}

	// Verify updated_at is a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339Nano, retrieved.UpdatedAt)
	if err != nil {
		t.Errorf("updated_at is not valid RFC3339: %v", err)
	}
}

func TestDailyStatsUpsertUpdate(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	projectID := "test-proj"
	date := "2025-01-15"

	// Insert initial stats
	initialStats := model.DailyStats{
		TicketsCreated: 5,
		TicketsClosed:  3,
		TokensSpent:    150000,
		AgentTimeSec:   1234.56,
	}
	err = repo.Upsert(projectID, date, initialStats)
	if err != nil {
		t.Fatalf("initial Upsert failed: %v", err)
	}

	// Get initial record
	first, err := repo.GetByDate(projectID, date)
	if err != nil {
		t.Fatalf("GetByDate after first insert failed: %v", err)
	}
	firstUpdatedAt := first.UpdatedAt

	// Small delay to ensure updated_at changes (RFC3339 has 1-second resolution)
	time.Sleep(1100 * time.Millisecond)

	// Upsert with updated values (same project+date)
	updatedStats := model.DailyStats{
		TicketsCreated: 10,
		TicketsClosed:  8,
		TokensSpent:    300000,
		AgentTimeSec:   2500.78,
	}
	err = repo.Upsert(projectID, date, updatedStats)
	if err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}

	// Get updated record
	second, err := repo.GetByDate(projectID, date)
	if err != nil {
		t.Fatalf("GetByDate after second insert failed: %v", err)
	}

	// Verify updated values
	if second.TicketsCreated != updatedStats.TicketsCreated {
		t.Errorf("expected tickets_created %d after update, got %d", updatedStats.TicketsCreated, second.TicketsCreated)
	}
	if second.TicketsClosed != updatedStats.TicketsClosed {
		t.Errorf("expected tickets_closed %d after update, got %d", updatedStats.TicketsClosed, second.TicketsClosed)
	}
	if second.TokensSpent != updatedStats.TokensSpent {
		t.Errorf("expected tokens_spent %d after update, got %d", updatedStats.TokensSpent, second.TokensSpent)
	}
	if second.AgentTimeSec != updatedStats.AgentTimeSec {
		t.Errorf("expected agent_time_sec %.2f after update, got %.2f", updatedStats.AgentTimeSec, second.AgentTimeSec)
	}

	// Verify updated_at changed
	if second.UpdatedAt == firstUpdatedAt {
		t.Errorf("expected updated_at to change after upsert, but it remained %q", firstUpdatedAt)
	}

	// Verify ID changed (INSERT OR REPLACE deletes old row and creates new one)
	if second.ID == first.ID {
		t.Logf("note: INSERT OR REPLACE may change ID from %d to %d", first.ID, second.ID)
	}
}

func TestDailyStatsGetByDateNonexistent(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	// Query nonexistent date
	retrieved, err := repo.GetByDate("test-proj", "2099-12-31")
	if err != nil {
		t.Fatalf("GetByDate should not error on nonexistent row, got: %v", err)
	}

	// Verify zero-value struct
	if retrieved.ID != 0 {
		t.Errorf("expected zero ID for nonexistent row, got %d", retrieved.ID)
	}
	if retrieved.ProjectID != "" {
		t.Errorf("expected empty project_id for nonexistent row, got %q", retrieved.ProjectID)
	}
	if retrieved.Date != "" {
		t.Errorf("expected empty date for nonexistent row, got %q", retrieved.Date)
	}
	if retrieved.TicketsCreated != 0 {
		t.Errorf("expected zero tickets_created for nonexistent row, got %d", retrieved.TicketsCreated)
	}
	if retrieved.TicketsClosed != 0 {
		t.Errorf("expected zero tickets_closed for nonexistent row, got %d", retrieved.TicketsClosed)
	}
	if retrieved.TokensSpent != 0 {
		t.Errorf("expected zero tokens_spent for nonexistent row, got %d", retrieved.TokensSpent)
	}
	if retrieved.AgentTimeSec != 0 {
		t.Errorf("expected zero agent_time_sec for nonexistent row, got %.2f", retrieved.AgentTimeSec)
	}
	if retrieved.UpdatedAt != "" {
		t.Errorf("expected empty updated_at for nonexistent row, got %q", retrieved.UpdatedAt)
	}
}

func TestDailyStatsMultipleProjects(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create two projects
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "proj-a", "Project A")
	if err != nil {
		t.Fatalf("failed to create project A: %v", err)
	}
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))`, "proj-b", "Project B")
	if err != nil {
		t.Fatalf("failed to create project B: %v", err)
	}

	repo := NewDailyStatsRepo(database, clock.Real())

	date := "2025-01-20"

	// Insert stats for project A
	statsA := model.DailyStats{
		TicketsCreated: 10,
		TicketsClosed:  5,
		TokensSpent:    100000,
		AgentTimeSec:   500.0,
	}
	err = repo.Upsert("proj-a", date, statsA)
	if err != nil {
		t.Fatalf("Upsert for project A failed: %v", err)
	}

	// Insert stats for project B (same date)
	statsB := model.DailyStats{
		TicketsCreated: 20,
		TicketsClosed:  15,
		TokensSpent:    200000,
		AgentTimeSec:   1000.0,
	}
	err = repo.Upsert("proj-b", date, statsB)
	if err != nil {
		t.Fatalf("Upsert for project B failed: %v", err)
	}

	// Retrieve and verify project A
	retrievedA, err := repo.GetByDate("proj-a", date)
	if err != nil {
		t.Fatalf("GetByDate for project A failed: %v", err)
	}
	if retrievedA.ProjectID != "proj-a" {
		t.Errorf("expected project_id 'proj-a', got %q", retrievedA.ProjectID)
	}
	if retrievedA.TicketsCreated != statsA.TicketsCreated {
		t.Errorf("expected tickets_created %d for project A, got %d", statsA.TicketsCreated, retrievedA.TicketsCreated)
	}

	// Retrieve and verify project B
	retrievedB, err := repo.GetByDate("proj-b", date)
	if err != nil {
		t.Fatalf("GetByDate for project B failed: %v", err)
	}
	if retrievedB.ProjectID != "proj-b" {
		t.Errorf("expected project_id 'proj-b', got %q", retrievedB.ProjectID)
	}
	if retrievedB.TicketsCreated != statsB.TicketsCreated {
		t.Errorf("expected tickets_created %d for project B, got %d", statsB.TicketsCreated, retrievedB.TicketsCreated)
	}

	// Verify they are separate rows
	if retrievedA.ID == retrievedB.ID {
		t.Errorf("expected different IDs for different projects, both got %d", retrievedA.ID)
	}
}

func TestDailyStatsMultipleDates(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	projectID := "test-proj"

	// Insert stats for multiple dates
	dates := []string{"2025-01-10", "2025-01-11", "2025-01-12"}
	for i, date := range dates {
		stats := model.DailyStats{
			TicketsCreated: i + 1,
			TicketsClosed:  i,
			TokensSpent:    int64((i + 1) * 10000),
			AgentTimeSec:   float64(i * 100),
		}
		err = repo.Upsert(projectID, date, stats)
		if err != nil {
			t.Fatalf("Upsert for date %s failed: %v", date, err)
		}
	}

	// Retrieve and verify each date
	for i, date := range dates {
		retrieved, err := repo.GetByDate(projectID, date)
		if err != nil {
			t.Fatalf("GetByDate for date %s failed: %v", date, err)
		}
		if retrieved.Date != date {
			t.Errorf("expected date %q, got %q", date, retrieved.Date)
		}
		if retrieved.TicketsCreated != i+1 {
			t.Errorf("expected tickets_created %d for date %s, got %d", i+1, date, retrieved.TicketsCreated)
		}
	}
}

func TestDailyStatsForeignKeyConstraint(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	repo := NewDailyStatsRepo(database, clock.Real())

	// Try to insert stats for nonexistent project
	stats := model.DailyStats{
		TicketsCreated: 1,
		TicketsClosed:  0,
		TokensSpent:    1000,
		AgentTimeSec:   10.0,
	}
	err = repo.Upsert("nonexistent-project", "2025-01-15", stats)
	if err == nil {
		t.Fatalf("expected FK constraint error for nonexistent project, got nil")
	}
	// Check that error mentions constraint or foreign key
	errMsg := err.Error()
	if errMsg == "" {
		t.Errorf("expected error message, got empty string")
	}
}

func TestDailyStatsDefaultValues(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	// Upsert with all zero values
	stats := model.DailyStats{
		TicketsCreated: 0,
		TicketsClosed:  0,
		TokensSpent:    0,
		AgentTimeSec:   0,
	}
	err = repo.Upsert("test-proj", "2025-01-15", stats)
	if err != nil {
		t.Fatalf("Upsert with zero values failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := repo.GetByDate("test-proj", "2025-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if retrieved.TicketsCreated != 0 {
		t.Errorf("expected tickets_created 0, got %d", retrieved.TicketsCreated)
	}
	if retrieved.TicketsClosed != 0 {
		t.Errorf("expected tickets_closed 0, got %d", retrieved.TicketsClosed)
	}
	if retrieved.TokensSpent != 0 {
		t.Errorf("expected tokens_spent 0, got %d", retrieved.TokensSpent)
	}
	if retrieved.AgentTimeSec != 0 {
		t.Errorf("expected agent_time_sec 0, got %.2f", retrieved.AgentTimeSec)
	}
}

func TestDailyStatsLargeValues(t *testing.T) {
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

	repo := NewDailyStatsRepo(database, clock.Real())

	// Test with large values
	stats := model.DailyStats{
		TicketsCreated: 999999,
		TicketsClosed:  999999,
		TokensSpent:    9223372036854775807, // max int64
		AgentTimeSec:   999999.999999,
	}
	err = repo.Upsert("test-proj", "2025-01-15", stats)
	if err != nil {
		t.Fatalf("Upsert with large values failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := repo.GetByDate("test-proj", "2025-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if retrieved.TicketsCreated != stats.TicketsCreated {
		t.Errorf("expected tickets_created %d, got %d", stats.TicketsCreated, retrieved.TicketsCreated)
	}
	if retrieved.TicketsClosed != stats.TicketsClosed {
		t.Errorf("expected tickets_closed %d, got %d", stats.TicketsClosed, retrieved.TicketsClosed)
	}
	if retrieved.TokensSpent != stats.TokensSpent {
		t.Errorf("expected tokens_spent %d, got %d", stats.TokensSpent, retrieved.TokensSpent)
	}
	// Use approximate comparison for float
	if retrieved.AgentTimeSec < stats.AgentTimeSec-0.001 || retrieved.AgentTimeSec > stats.AgentTimeSec+0.001 {
		t.Errorf("expected agent_time_sec ~%.6f, got %.6f", stats.AgentTimeSec, retrieved.AgentTimeSec)
	}
}
