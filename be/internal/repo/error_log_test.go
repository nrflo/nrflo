package repo

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupErrorLogDB(t *testing.T) (*ErrorLogRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewErrorLogRepo(database, clock.Real()), "proj-1"
}

func TestErrorLogRepo_Insert(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	e := &model.ErrorLog{
		ID:         "err-1",
		ProjectID:  projectID,
		ErrorType:  model.ErrorTypeAgent,
		InstanceID: "sess-abc",
		Message:    "implementor: timeout after 1800s",
		CreatedAt:  "2025-01-01T00:00:00Z",
	}

	if err := r.Insert(e); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := r.List(projectID, "", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("List count = %d, want 1", len(results))
	}
	got := results[0]
	if got.ID != e.ID {
		t.Errorf("ID = %q, want %q", got.ID, e.ID)
	}
	if got.ProjectID != e.ProjectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, e.ProjectID)
	}
	if got.ErrorType != e.ErrorType {
		t.Errorf("ErrorType = %q, want %q", got.ErrorType, e.ErrorType)
	}
	if got.InstanceID != e.InstanceID {
		t.Errorf("InstanceID = %q, want %q", got.InstanceID, e.InstanceID)
	}
	if got.Message != e.Message {
		t.Errorf("Message = %q, want %q", got.Message, e.Message)
	}
	if got.CreatedAt != e.CreatedAt {
		t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, e.CreatedAt)
	}
}

func TestErrorLogRepo_Insert_UsesClockWhenCreatedAtEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	r := NewErrorLogRepo(database, clk)

	e := &model.ErrorLog{
		ID:         "err-clock",
		ProjectID:  "p1",
		ErrorType:  model.ErrorTypeWorkflow,
		InstanceID: "wfi-123",
		Message:    "workflow failed",
		// CreatedAt intentionally empty — repo should use clock
	}
	if err := r.Insert(e); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	results, err := r.List("p1", "", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("count = %d, want 1", len(results))
	}
	want := fixedTime.UTC().Format(time.RFC3339Nano)
	if results[0].CreatedAt != want {
		t.Errorf("CreatedAt = %q, want %q", results[0].CreatedAt, want)
	}
}

func TestErrorLogRepo_List_Pagination(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		e := &model.ErrorLog{
			ID:         fmt.Sprintf("err-%d", i+1),
			ProjectID:  projectID,
			ErrorType:  model.ErrorTypeAgent,
			InstanceID: fmt.Sprintf("sess-%d", i+1),
			Message:    fmt.Sprintf("error %d", i+1),
			CreatedAt:  base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano),
		}
		if err := r.Insert(e); err != nil {
			t.Fatalf("Insert %d: %v", i+1, err)
		}
	}

	// Page 1 (newest 2 — ordered DESC by created_at)
	p1, err := r.List(projectID, "", 2, 0)
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page1 count = %d, want 2", len(p1))
	}
	if p1[0].ID != "err-5" {
		t.Errorf("page1[0].ID = %q, want err-5 (most recent)", p1[0].ID)
	}
	if p1[1].ID != "err-4" {
		t.Errorf("page1[1].ID = %q, want err-4", p1[1].ID)
	}

	// Page 2
	p2, err := r.List(projectID, "", 2, 2)
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(p2) != 2 {
		t.Fatalf("page2 count = %d, want 2", len(p2))
	}

	// Page 3 (last record)
	p3, err := r.List(projectID, "", 2, 4)
	if err != nil {
		t.Fatalf("List page3: %v", err)
	}
	if len(p3) != 1 {
		t.Fatalf("page3 count = %d, want 1", len(p3))
	}
	if p3[0].ID != "err-1" {
		t.Errorf("page3[0].ID = %q, want err-1 (oldest)", p3[0].ID)
	}
}

func TestErrorLogRepo_List_TypeFilter(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []struct {
		id  string
		typ model.ErrorType
	}{
		{"err-a1", model.ErrorTypeAgent},
		{"err-a2", model.ErrorTypeAgent},
		{"err-w1", model.ErrorTypeWorkflow},
		{"err-s1", model.ErrorTypeSystem},
	}
	for i, e := range entries {
		err := r.Insert(&model.ErrorLog{
			ID:         e.id,
			ProjectID:  projectID,
			ErrorType:  e.typ,
			InstanceID: fmt.Sprintf("inst-%d", i),
			Message:    "test error",
			CreatedAt:  base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			t.Fatalf("Insert %s: %v", e.id, err)
		}
	}

	tests := []struct {
		name      string
		errorType string
		wantCount int
	}{
		{"all types (empty filter)", "", 4},
		{"agent only", "agent", 2},
		{"workflow only", "workflow", 1},
		{"system only", "system", 1},
		{"no match", "unknown", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results, err := r.List(projectID, tc.errorType, 100, 0)
			if err != nil {
				t.Fatalf("List(%q): %v", tc.errorType, err)
			}
			if len(results) != tc.wantCount {
				t.Errorf("List(%q) count = %d, want %d", tc.errorType, len(results), tc.wantCount)
			}
		})
	}
}

func TestErrorLogRepo_Count(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, typ := range []model.ErrorType{model.ErrorTypeAgent, model.ErrorTypeAgent, model.ErrorTypeWorkflow} {
		err := r.Insert(&model.ErrorLog{
			ID:         fmt.Sprintf("err-%d", i),
			ProjectID:  projectID,
			ErrorType:  typ,
			InstanceID: fmt.Sprintf("inst-%d", i),
			Message:    "msg",
			CreatedAt:  base.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	tests := []struct {
		name      string
		errorType string
		want      int
	}{
		{"all", "", 3},
		{"agent", "agent", 2},
		{"workflow", "workflow", 1},
		{"system (none)", "system", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			count, err := r.Count(projectID, tc.errorType)
			if err != nil {
				t.Fatalf("Count(%q): %v", tc.errorType, err)
			}
			if count != tc.want {
				t.Errorf("Count(%q) = %d, want %d", tc.errorType, count, tc.want)
			}
		})
	}
}

func TestErrorLogRepo_List_EmptyResult(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	results, err := r.List(projectID, "", 10, 0)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("List count = %d, want 0", len(results))
	}
}

func TestErrorLogRepo_Count_EmptyResult(t *testing.T) {
	r, projectID := setupErrorLogDB(t)

	count, err := r.Count(projectID, "")
	if err != nil {
		t.Fatalf("Count empty: %v", err)
	}
	if count != 0 {
		t.Errorf("Count = %d, want 0", count)
	}
}
