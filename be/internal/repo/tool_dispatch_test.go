package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func setupDispatchDB(t *testing.T) (*DispatchRepo, string) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewDispatchRepo(database, clk), "proj-1"
}

func TestDispatchRepo_InsertSetsIDAndTimestamp(t *testing.T) {
	t.Parallel()
	r, projectID := setupDispatchDB(t)

	d := &model.ToolDispatch{
		ProjectID:  projectID,
		ToolName:   "write_file",
		Input:      `{}`,
		Status:     model.DispatchStatusSuccess,
		DurationMs: 100,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set after Insert")
	}
	if d.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero after Insert")
	}
}

func TestDispatchRepo_InsertErrorStatus(t *testing.T) {
	t.Parallel()
	r, projectID := setupDispatchDB(t)

	errMsg := "connection refused"
	d := &model.ToolDispatch{
		ProjectID:  projectID,
		ToolName:   "exec_cmd",
		Input:      `{}`,
		Status:     model.DispatchStatusError,
		ErrorMsg:   &errMsg,
		DurationMs: 50,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert error-status dispatch: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set after Insert")
	}
}

func TestDispatchRepo_InsertNilSessionID(t *testing.T) {
	t.Parallel()
	r, projectID := setupDispatchDB(t)

	d := &model.ToolDispatch{
		ProjectID:  projectID,
		ToolName:   "no_session_tool",
		Input:      `{"x":1}`,
		Status:     model.DispatchStatusSuccess,
		DurationMs: 5,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert with nil SessionID: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set")
	}
}

func TestDispatchRepo_InsertWithSessionID(t *testing.T) {
	t.Parallel()
	r, projectID := setupDispatchDB(t)

	sessionID := "sess-abc"
	output := `{"result":"ok"}`
	d := &model.ToolDispatch{
		ProjectID:  projectID,
		SessionID:  &sessionID,
		ToolName:   "lookup_sku",
		Input:      `{"sku":"X1"}`,
		Output:     &output,
		Status:     model.DispatchStatusSuccess,
		DurationMs: 20,
	}
	if err := r.Insert(d); err != nil {
		t.Fatalf("Insert with SessionID: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set")
	}
}
