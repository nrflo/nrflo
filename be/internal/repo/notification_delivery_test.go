package repo

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func setupNotificationDeliveryDB(t *testing.T) (*NotificationDeliveryRepo, string, string) {
	t.Helper()
	database := newTestDB(t)
	clk := clock.Real()
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-d', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-d', 'proj-d', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	channelRepo := NewNotificationChannelRepo(database, clk)
	ch := makeNotifyChannel("proj-d", "wf-d", "ch", model.ChannelKindSlack, true, []string{"orchestration.completed"})
	if err := channelRepo.Insert(ch); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	dr := NewNotificationDeliveryRepo(database, clk)
	return dr, "proj-d", ch.ID
}

func makeDelivery(channelID, projectID, eventType string) *model.NotificationDelivery {
	return &model.NotificationDelivery{
		ChannelID: channelID,
		ProjectID: projectID,
		EventType: eventType,
		Payload:   `{"test":true}`,
		Status:    model.DeliveryStatusPending,
	}
}

func TestNotificationDeliveryRepo_Insert(t *testing.T) {
	t.Parallel()
	dr, projectID, channelID := setupNotificationDeliveryDB(t)
	d := makeDelivery(channelID, projectID, "orchestration.completed")
	if err := dr.Insert(d); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if d.ID == "" {
		t.Errorf("ID not set after Insert")
	}
	if d.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
}

func TestNotificationDeliveryRepo_ListPending_ExcludesSentFailedGivingUp(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p1', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-p1', 'p1', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	channelRepo := NewNotificationChannelRepo(database, clk)
	ch := makeNotifyChannel("p1", "wf-p1", "ch", model.ChannelKindSlack, true, nil)
	channelRepo.Insert(ch)

	dr := NewNotificationDeliveryRepo(database, clk)

	d1 := makeDelivery(ch.ID, "p1", "e1-pending")
	dr.Insert(d1)
	d2 := makeDelivery(ch.ID, "p1", "e2-sent")
	d2.Status = model.DeliveryStatusSent
	dr.Insert(d2)
	d3 := makeDelivery(ch.ID, "p1", "e3-failed")
	d3.Status = model.DeliveryStatusFailed
	dr.Insert(d3)
	d4 := makeDelivery(ch.ID, "p1", "e4-givingup")
	d4.Status = model.DeliveryStatusGivingUp
	dr.Insert(d4)

	results, err := dr.ListPending(clk.Now(), 100)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("ListPending count = %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].EventType != "e1-pending" {
		t.Errorf("EventType = %q, want e1-pending", results[0].EventType)
	}
}

func TestNotificationDeliveryRepo_ListPending_ExcludesFutureNextAttempt(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p2', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-p2', 'p2', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	channelRepo := NewNotificationChannelRepo(database, clk)
	ch := makeNotifyChannel("p2", "wf-p2", "ch", model.ChannelKindSlack, true, nil)
	channelRepo.Insert(ch)
	dr := NewNotificationDeliveryRepo(database, clk)

	future := clk.Now().Add(60 * time.Second)
	d1 := makeDelivery(ch.ID, "p2", "future-event")
	dr.Insert(d1)
	dr.UpdateStatus(d1.ID, model.DeliveryStatusPending, 1, "err", &future)

	d2 := makeDelivery(ch.ID, "p2", "ready-event")
	dr.Insert(d2)

	results, err := dr.ListPending(clk.Now(), 100)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("count = %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].EventType != "ready-event" {
		t.Errorf("EventType = %q, want ready-event", results[0].EventType)
	}

	clk.Advance(61 * time.Second)
	results2, err := dr.ListPending(clk.Now(), 100)
	if err != nil {
		t.Fatalf("ListPending after advance: %v", err)
	}
	if len(results2) != 2 {
		t.Errorf("count after advance = %d, want 2", len(results2))
	}
}

func TestNotificationDeliveryRepo_ListByChannel_NewestFirst(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)

	database := newTestDB(t)
	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('p3', 'T', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-p3', 'p3', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	channelRepo := NewNotificationChannelRepo(database, clk)
	ch := makeNotifyChannel("p3", "wf-p3", "ch", model.ChannelKindSlack, true, nil)
	channelRepo.Insert(ch)
	dr := NewNotificationDeliveryRepo(database, clk)

	d1 := makeDelivery(ch.ID, "p3", "first")
	dr.Insert(d1)
	clk.Advance(time.Second)
	d2 := makeDelivery(ch.ID, "p3", "second")
	dr.Insert(d2)
	clk.Advance(time.Second)
	d3 := makeDelivery(ch.ID, "p3", "third")
	dr.Insert(d3)

	results, err := dr.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("count = %d, want 3", len(results))
	}
	if results[0].EventType != "third" {
		t.Errorf("results[0].EventType = %q, want third (newest first)", results[0].EventType)
	}
	if results[2].EventType != "first" {
		t.Errorf("results[2].EventType = %q, want first (oldest last)", results[2].EventType)
	}
}

func TestNotificationDeliveryRepo_UpdateStatus_NotFound(t *testing.T) {
	t.Parallel()
	dr, _, _ := setupNotificationDeliveryDB(t)
	if err := dr.UpdateStatus("no-such", model.DeliveryStatusSent, 1, "", nil); err == nil {
		t.Errorf("UpdateStatus missing: expected error, got nil")
	}
}

func TestNotificationDeliveryRepo_ListByChannel_Limit(t *testing.T) {
	t.Parallel()
	dr, projectID, channelID := setupNotificationDeliveryDB(t)
	for i := 0; i < 5; i++ {
		dr.Insert(makeDelivery(channelID, projectID, "e"))
	}
	results, err := dr.ListByChannel(channelID, 3)
	if err != nil {
		t.Fatalf("ListByChannel limit=3: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("count = %d, want 3 (limit applied)", len(results))
	}
}
