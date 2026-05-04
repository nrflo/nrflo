package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func setupNotificationChannelDB(t *testing.T) (*NotificationChannelRepo, string) {
	t.Helper()
	database := newTestDB(t)
	var err error
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-1', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return NewNotificationChannelRepo(database, clock.Real()), "proj-1"
}

func makeNotifyChannel(projectID, name string, kind model.ChannelKind, enabled bool, eventTypes []string) *model.NotificationChannel {
	return &model.NotificationChannel{
		ProjectID:  projectID,
		Name:       name,
		Kind:       kind,
		Enabled:    enabled,
		Config:     `{"webhook_url":"https://example.com/hook"}`,
		EventTypes: eventTypes,
	}
}

func TestNotificationChannelRepo_Insert_Get(t *testing.T) {
	t.Parallel()
	r, projectID := setupNotificationChannelDB(t)

	ch := makeNotifyChannel(projectID, "slack-alerts", model.ChannelKindSlack, true,
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
	r, _ := setupNotificationChannelDB(t)
	_, err := r.Get("no-such-id")
	if err == nil {
		t.Fatalf("Get missing: expected error, got nil")
	}
}

func TestNotificationChannelRepo_Update_MutatesFields(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	var err error
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	r := NewNotificationChannelRepo(database, clk)

	ch := makeNotifyChannel("p1", "old-name", model.ChannelKindSlack, true, []string{"a"})
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
	r, _ := setupNotificationChannelDB(t)
	ch := &model.NotificationChannel{ID: "no-such", Name: "x", Kind: model.ChannelKindSlack}
	if err := r.Update(ch); err == nil {
		t.Fatalf("Update non-existent: expected error, got nil")
	}
}

func TestNotificationChannelRepo_Delete(t *testing.T) {
	t.Parallel()
	r, projectID := setupNotificationChannelDB(t)
	ch := makeNotifyChannel(projectID, "to-delete", model.ChannelKindSlack, true, nil)
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

func TestNotificationChannelRepo_ListByProject_FiltersProject(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	var err error
	for _, id := range []string{"pa", "pb"} {
		if _, err := database.Exec(
			`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'P', datetime('now'), datetime('now'))`, id); err != nil {
			t.Fatalf("insert project %s: %v", id, err)
		}
	}
	r := NewNotificationChannelRepo(database, clock.Real())

	r.Insert(makeNotifyChannel("pa", "c1", model.ChannelKindSlack, true, nil))
	r.Insert(makeNotifyChannel("pa", "c2", model.ChannelKindTelegram, true, nil))
	r.Insert(makeNotifyChannel("pb", "c3", model.ChannelKindSlack, true, nil))

	listA, err := r.ListByProject("pa")
	if err != nil {
		t.Fatalf("ListByProject pa: %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("ListByProject pa count = %d, want 2", len(listA))
	}
	listB, err := r.ListByProject("pb")
	if err != nil {
		t.Fatalf("ListByProject pb: %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("ListByProject pb count = %d, want 1", len(listB))
	}
	listC, err := r.ListByProject("none")
	if err != nil {
		t.Fatalf("ListByProject none: %v", err)
	}
	if len(listC) != 0 {
		t.Errorf("ListByProject none count = %d, want 0", len(listC))
	}
}

func TestNotificationChannelRepo_ListEnabledForEvent(t *testing.T) {
	t.Parallel()
	r, projectID := setupNotificationChannelDB(t)

	// enabled + subscribes to target event
	ch1 := makeNotifyChannel(projectID, "ch1", model.ChannelKindSlack, true,
		[]string{"orchestration.completed", "agent.completed"})
	r.Insert(ch1)

	// disabled — must be excluded
	ch2 := makeNotifyChannel(projectID, "ch2", model.ChannelKindSlack, false,
		[]string{"orchestration.completed"})
	r.Insert(ch2)

	// enabled but wrong events
	ch3 := makeNotifyChannel(projectID, "ch3", model.ChannelKindSlack, true,
		[]string{"agent.started"})
	r.Insert(ch3)

	// enabled telegram, watches target event
	ch4 := makeNotifyChannel(projectID, "ch4", model.ChannelKindTelegram, true,
		[]string{"orchestration.completed"})
	r.Insert(ch4)

	results, err := r.ListEnabledForEvent(projectID, "orchestration.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("count = %d, want 2 (ch1, ch4)", len(results))
	}

	results2, err := r.ListEnabledForEvent(projectID, "agent.completed")
	if err != nil {
		t.Fatalf("ListEnabledForEvent agent.completed: %v", err)
	}
	if len(results2) != 1 {
		t.Errorf("count = %d, want 1 (ch1 only)", len(results2))
	}

	results3, err := r.ListEnabledForEvent(projectID, "agent.started")
	if err != nil {
		t.Fatalf("ListEnabledForEvent agent.started: %v", err)
	}
	if len(results3) != 1 {
		t.Errorf("count = %d, want 1 (ch3 only)", len(results3))
	}
}
