package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// setupNotificationChannelDB seeds a project + workflow and returns (repo, projectID, workflowID).
func setupNotificationChannelDB(t *testing.T) (*NotificationChannelRepo, string, string) {
	t.Helper()
	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-1', 'proj-1', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	return NewNotificationChannelRepo(database, clock.Real()), "proj-1", "wf-1"
}

func makeNotifyChannel(projectID, workflowID, name string, kind model.ChannelKind, enabled bool, eventTypes []string) *model.NotificationChannel {
	return &model.NotificationChannel{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Name:       name,
		Kind:       kind,
		Enabled:    enabled,
		Config:     `{"webhook_url":"https://example.com/hook"}`,
		EventTypes: eventTypes,
	}
}

func TestNotificationChannelRepo_Insert_Get(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	ch := makeNotifyChannel(projectID, workflowID, "slack-alerts", model.ChannelKindSlack, true,
		[]string{"orchestration.completed", "agent.completed"})
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if ch.ID == "" {
		t.Errorf("ID not set after Insert")
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projectID)
	}
	if got.WorkflowID != workflowID {
		t.Errorf("WorkflowID = %q, want %q", got.WorkflowID, workflowID)
	}
	if got.Name != "slack-alerts" {
		t.Errorf("Name = %q, want slack-alerts", got.Name)
	}
	if got.Kind != model.ChannelKindSlack {
		t.Errorf("Kind = %q, want slack", got.Kind)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if len(got.EventTypes) != 2 {
		t.Fatalf("EventTypes len = %d, want 2", len(got.EventTypes))
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero")
	}
}

func TestNotificationChannelRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	r, _, _ := setupNotificationChannelDB(t)
	if _, err := r.Get("no-such-id"); err == nil {
		t.Fatalf("Get missing: expected error, got nil")
	}
}

func TestNotificationChannelRepo_Update_MutatesFields(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-upd', 'p1', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	r := NewNotificationChannelRepo(database, clk)

	ch := makeNotifyChannel("p1", "wf-upd", "old-name", model.ChannelKindSlack, true, []string{"a"})
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	originalUpdatedAt := ch.UpdatedAt

	clk.Advance(time.Second)
	ch.Name = "new-name"
	ch.Enabled = false
	ch.EventTypes = []string{"b", "c"}
	if err := r.Update(ch); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.Get(ch.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Name != "new-name" {
		t.Errorf("Name = %q, want new-name", got.Name)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if len(got.EventTypes) != 2 {
		t.Errorf("EventTypes len = %d, want 2", len(got.EventTypes))
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt %v not after original %v", got.UpdatedAt, originalUpdatedAt)
	}
}

func TestNotificationChannelRepo_Update_NotFound(t *testing.T) {
	t.Parallel()
	r, _, _ := setupNotificationChannelDB(t)
	ch := &model.NotificationChannel{ID: "no-such", Name: "x", Kind: model.ChannelKindSlack}
	if err := r.Update(ch); err == nil {
		t.Fatalf("Update non-existent: expected error, got nil")
	}
}

func TestNotificationChannelRepo_Delete(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)
	ch := makeNotifyChannel(projectID, workflowID, "to-delete", model.ChannelKindSlack, true, nil)
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := r.Delete(ch.ID); err != nil {
		t.Fatalf("Delete first: %v", err)
	}
	if err := r.Delete(ch.ID); err == nil {
		t.Fatalf("Delete second: expected error, got nil")
	}
}

func TestNotificationChannelRepo_ListByWorkflow_FiltersWorkflow(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	for _, proj := range []string{"pa", "pb"} {
		if _, err := database.Exec(
			`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, proj); err != nil {
			t.Fatalf("insert project %s: %v", proj, err)
		}
	}
	for _, row := range [][2]string{{"wf-1", "pa"}, {"wf-2", "pa"}, {"wf-1", "pb"}} {
		if _, err := database.Exec(
			`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, '', datetime('now'), datetime('now'))`, row[0], row[1]); err != nil {
			t.Fatalf("insert workflow %s/%s: %v", row[1], row[0], err)
		}
	}
	r := NewNotificationChannelRepo(database, clock.Real())

	r.Insert(makeNotifyChannel("pa", "wf-1", "c1", model.ChannelKindSlack, true, nil))
	r.Insert(makeNotifyChannel("pa", "wf-1", "c2", model.ChannelKindTelegram, true, nil))
	r.Insert(makeNotifyChannel("pa", "wf-2", "c3", model.ChannelKindSlack, true, nil))
	r.Insert(makeNotifyChannel("pb", "wf-1", "c4", model.ChannelKindSlack, true, nil))

	if list, err := r.ListByWorkflow("pa", "wf-1"); err != nil {
		t.Fatalf("ListByWorkflow pa/wf-1: %v", err)
	} else if len(list) != 2 {
		t.Errorf("pa/wf-1 count = %d, want 2", len(list))
	}
	if list, err := r.ListByWorkflow("pa", "wf-2"); err != nil {
		t.Fatalf("ListByWorkflow pa/wf-2: %v", err)
	} else if len(list) != 1 {
		t.Errorf("pa/wf-2 count = %d, want 1", len(list))
	}
	if list, err := r.ListByWorkflow("pb", "wf-1"); err != nil {
		t.Fatalf("ListByWorkflow pb/wf-1: %v", err)
	} else if len(list) != 1 {
		t.Errorf("pb/wf-1 count = %d, want 1", len(list))
	}
	if list, _ := r.ListByWorkflow("pa", "none"); len(list) != 0 {
		t.Errorf("pa/none count = %d, want 0", len(list))
	}
}

func TestNotificationChannelRepo_ListEnabledForEvent(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	ch1 := makeNotifyChannel(projectID, workflowID, "ch1", model.ChannelKindSlack, true,
		[]string{"orchestration.completed", "agent.completed"})
	r.Insert(ch1)
	ch2 := makeNotifyChannel(projectID, workflowID, "ch2", model.ChannelKindSlack, false,
		[]string{"orchestration.completed"}) // disabled
	r.Insert(ch2)
	ch3 := makeNotifyChannel(projectID, workflowID, "ch3", model.ChannelKindSlack, true,
		[]string{"agent.started"}) // wrong event
	r.Insert(ch3)
	ch4 := makeNotifyChannel(projectID, workflowID, "ch4", model.ChannelKindTelegram, true,
		[]string{"orchestration.completed"})
	r.Insert(ch4)

	results, err := r.ListEnabledForEvent(projectID, workflowID, "orchestration.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("count = %d, want 2 (ch1, ch4)", len(results))
	}
	results2, err := r.ListEnabledForEvent(projectID, workflowID, "agent.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent agent.completed: %v", err)
	}
	if len(results2) != 1 {
		t.Errorf("count = %d, want 1 (ch1 only)", len(results2))
	}
	results3, err := r.ListEnabledForEvent(projectID, workflowID, "agent.started")
	if err != nil {
		t.Fatalf("ListEnabledForEvent agent.started: %v", err)
	}
	if len(results3) != 1 {
		t.Errorf("count = %d, want 1 (ch3 only)", len(results3))
	}
}

func TestNotificationChannelRepo_ListEnabledForEvent_WorkflowIsolation(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-iso', 'Iso', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	for _, wfID := range []string{"wf-a", "wf-b"} {
		if _, err := database.Exec(
			`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, 'proj-iso', '', datetime('now'), datetime('now'))`, wfID); err != nil {
			t.Fatalf("insert workflow %s: %v", wfID, err)
		}
	}
	r := NewNotificationChannelRepo(database, clock.Real())
	r.Insert(makeNotifyChannel("proj-iso", "wf-a", "ch-a", model.ChannelKindSlack, true, []string{"orchestration.completed"}))
	r.Insert(makeNotifyChannel("proj-iso", "wf-b", "ch-b", model.ChannelKindSlack, true, []string{"orchestration.completed"}))

	listA, err := r.ListEnabledForEvent("proj-iso", "wf-a", "orchestration.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent wf-a: %v", err)
	}
	if len(listA) != 1 || listA[0].Name != "ch-a" {
		t.Errorf("wf-a: got %d channels (want 1 named ch-a)", len(listA))
	}

	listB, err := r.ListEnabledForEvent("proj-iso", "wf-b", "orchestration.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent wf-b: %v", err)
	}
	if len(listB) != 1 || listB[0].Name != "ch-b" {
		t.Errorf("wf-b: got %d channels (want 1 named ch-b)", len(listB))
	}
}

func TestNotificationChannelRepo_WorkflowDelete_CascadesChannels(t *testing.T) {
	t.Parallel()
	r, projectID, workflowID := setupNotificationChannelDB(t)

	ch := makeNotifyChannel(projectID, workflowID, "cascade-ch", model.ChannelKindSlack, true, nil)
	if err := r.Insert(ch); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := r.db.Exec(`DELETE FROM workflows WHERE id = ? AND project_id = ?`, workflowID, projectID); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	if _, err := r.Get(ch.ID); err == nil {
		t.Errorf("Get after cascade delete: expected not-found error, got nil")
	}
	if list, _ := r.ListByWorkflow(projectID, workflowID); len(list) != 0 {
		t.Errorf("ListByWorkflow after cascade: got %d channels, want 0", len(list))
	}
}
