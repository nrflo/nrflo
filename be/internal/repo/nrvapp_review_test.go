package repo

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

func setupNrvappReviewDB(t *testing.T) (*NrvappReviewRepo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewNrvappReviewRepo(database, clk), "proj-1"
}

func TestNrvappReviewRepo_InsertGet(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	item := &model.NrvappReviewItem{
		ProjectID: projectID,
		ToolName:  "write_file",
		Input:     `{"path":"foo.go"}`,
	}
	if err := r.Insert(item); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if item.ID == "" {
		t.Errorf("ID not set after Insert")
	}
	if item.Status != model.ReviewStatusPending {
		t.Errorf("Status = %q, want %q", item.Status, model.ReviewStatusPending)
	}
	if item.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != item.ID {
		t.Errorf("ID = %q, want %q", got.ID, item.ID)
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.ToolName != "write_file" {
		t.Errorf("ToolName = %q, want write_file", got.ToolName)
	}
	if got.Status != model.ReviewStatusPending {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	if got.Output != nil {
		t.Errorf("Output = %v, want nil", got.Output)
	}
	if got.Draft != nil {
		t.Errorf("Draft = %v, want nil", got.Draft)
	}
	if got.ApprovedAt != nil {
		t.Errorf("ApprovedAt = %v, want nil", got.ApprovedAt)
	}
}

func TestNrvappReviewRepo_GetNotFound(t *testing.T) {
	t.Parallel()
	r, _ := setupNrvappReviewDB(t)
	_, err := r.Get("no-such-id")
	if err == nil {
		t.Fatal("Get missing: expected error, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want wrapping sql.ErrNoRows", err)
	}
}

func TestNrvappReviewRepo_ListAllStatuses(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	for _, tool := range []string{"tool_a", "tool_b", "tool_c"} {
		item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: tool, Input: `{}`}
		if err := r.Insert(item); err != nil {
			t.Fatalf("Insert %s: %v", tool, err)
		}
	}

	got, err := r.List(projectID, "", 100, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("List count = %d, want 3", len(got))
	}
}

func TestNrvappReviewRepo_ListStatusFilter(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	pending := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "tp", Input: `{}`}
	r.Insert(pending)

	approved := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "ta", Input: `{}`}
	r.Insert(approved)
	r.Approve(approved.ID, projectID)

	rejected := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "tr", Input: `{}`}
	r.Insert(rejected)
	r.Reject(rejected.ID, projectID, "bad")

	tests := []struct {
		status string
		want   int
	}{
		{"", 3},
		{model.ReviewStatusPending, 1},
		{model.ReviewStatusApproved, 1},
		{model.ReviewStatusRejected, 1},
	}
	for _, tc := range tests {
		got, err := r.List(projectID, tc.status, 100, 0)
		if err != nil {
			t.Fatalf("List(%q): %v", tc.status, err)
		}
		if len(got) != tc.want {
			t.Errorf("List(%q) count = %d, want %d", tc.status, len(got), tc.want)
		}
	}
}

func TestNrvappReviewRepo_ListPagination(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	for i := 0; i < 5; i++ {
		item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "t", Input: `{}`}
		if err := r.Insert(item); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	page1, err := r.List(projectID, "", 2, 0)
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	page2, err := r.List(projectID, "", 2, 2)
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	page3, err := r.List(projectID, "", 2, 4)
	if err != nil {
		t.Fatalf("List page3: %v", err)
	}

	if len(page1) != 2 {
		t.Errorf("page1 len = %d, want 2", len(page1))
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	if len(page3) != 1 {
		t.Errorf("page3 len = %d, want 1", len(page3))
	}

	ids := map[string]bool{}
	for _, p := range [][]*model.NrvappReviewItem{page1, page2, page3} {
		for _, item := range p {
			if ids[item.ID] {
				t.Errorf("duplicate ID %q across pages", item.ID)
			}
			ids[item.ID] = true
		}
	}
}

func TestNrvappReviewRepo_UpdateDraft(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "t", Input: `{}`}
	r.Insert(item)

	if err := r.UpdateDraft(item.ID, projectID, "my draft"); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Draft == nil || *got.Draft != "my draft" {
		t.Errorf("Draft = %v, want 'my draft'", got.Draft)
	}
	if got.Status != model.ReviewStatusPending {
		t.Errorf("Status = %q after UpdateDraft, want pending", got.Status)
	}
}

func TestNrvappReviewRepo_UpdateDraftNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)
	if err := r.UpdateDraft("no-such-id", projectID, "x"); err == nil {
		t.Fatal("UpdateDraft non-existent: expected error, got nil")
	}
}

func TestNrvappReviewRepo_ApproveCopiesDraftToOutput(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "t", Input: `{}`}
	r.Insert(item)
	r.UpdateDraft(item.ID, projectID, "the draft")

	if err := r.Approve(item.ID, projectID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != model.ReviewStatusApproved {
		t.Errorf("Status = %q, want approved", got.Status)
	}
	if got.Output == nil || *got.Output != "the draft" {
		t.Errorf("Output = %v, want 'the draft' (copied from draft)", got.Output)
	}
	if got.ApprovedAt == nil {
		t.Errorf("ApprovedAt is nil, want non-nil")
	}
}

func TestNrvappReviewRepo_ApprovePreservesExistingOutput(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	existing := "original output"
	item := &model.NrvappReviewItem{
		ProjectID: projectID,
		ToolName:  "t",
		Input:     `{}`,
		Output:    &existing,
	}
	r.Insert(item)
	r.UpdateDraft(item.ID, projectID, "new draft")

	if err := r.Approve(item.ID, projectID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Output == nil || *got.Output != "original output" {
		t.Errorf("Output = %v, want 'original output' (preserved, not overwritten by draft)", got.Output)
	}
}

func TestNrvappReviewRepo_ApproveNotFound(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)
	if err := r.Approve("no-such-id", projectID); err == nil {
		t.Fatal("Approve non-existent: expected error, got nil")
	}
}

func TestNrvappReviewRepo_Reject(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "t", Input: `{}`}
	r.Insert(item)

	if err := r.Reject(item.ID, projectID, "dangerous command"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != model.ReviewStatusRejected {
		t.Errorf("Status = %q, want rejected", got.Status)
	}
	if got.RejectReason == nil || *got.RejectReason != "dangerous command" {
		t.Errorf("RejectReason = %v, want 'dangerous command'", got.RejectReason)
	}
}

func TestNrvappReviewRepo_RejectEmptyReason(t *testing.T) {
	t.Parallel()
	r, projectID := setupNrvappReviewDB(t)

	item := &model.NrvappReviewItem{ProjectID: projectID, ToolName: "t", Input: `{}`}
	r.Insert(item)

	if err := r.Reject(item.ID, projectID, ""); err != nil {
		t.Fatalf("Reject with empty reason: %v", err)
	}

	got, err := r.Get(item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != model.ReviewStatusRejected {
		t.Errorf("Status = %q, want rejected", got.Status)
	}
}
