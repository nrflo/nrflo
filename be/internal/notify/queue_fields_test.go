package notify

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// recordingTransport captures the last Notification sent through it.
type recordingTransport struct {
	last *Notification
}

func (r *recordingTransport) Kind() string { return "script" }
func (r *recordingTransport) Send(n *Notification) error {
	r.last = n
	return nil
}

// insertScriptChannel inserts a notification_channels row with kind='script'.
func insertScriptChannel(t *testing.T, querier db.Querier, projectID, workflowID string) string {
	t.Helper()
	id := "ch-script-" + projectID
	_, err := querier.Exec(
		`INSERT INTO notification_channels (id, project_id, workflow_id, name, kind, enabled, config, message_template, event_types, created_at, updated_at)
		 VALUES (?, ?, ?, 'script-ch', 'script', 1, '{"script_code":"pass"}', '', '["orchestration.completed"]', datetime('now'), datetime('now'))`,
		id, projectID, workflowID,
	)
	if err != nil {
		t.Fatalf("insertScriptChannel: %v", err)
	}
	return id
}

func TestWorker_DispatchPopulatesNotificationFields(t *testing.T) {
	rec := &recordingTransport{}
	old := registry["script"]
	registry["script"] = rec
	t.Cleanup(func() {
		if old != nil {
			registry["script"] = old
		} else {
			delete(registry, "script")
		}
	})

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)

	channelID := insertScriptChannel(t, database, projectID, workflowID)

	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)

	d := &model.NotificationDelivery{
		ChannelID: channelID,
		ProjectID: projectID,
		EventType: "orchestration.completed",
		Payload:   `{"workflow":"foo","instance_id":"wfi-x","ticket_id":"T-1"}`,
		Status:    model.DeliveryStatusPending,
	}
	if err := deliveryRepo.Insert(d); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}

	errSvc := &stubErrorRecorder{}
	worker := NewWorker(deliveryRepo, channelRepo, nil, errSvc, clk, make(chan struct{}, 1))
	worker.Tick(context.Background())

	if rec.last == nil {
		t.Fatal("transport.Send not called")
	}
	n := rec.last
	if n.WorkflowID != "foo" {
		t.Errorf("WorkflowID = %q, want foo", n.WorkflowID)
	}
	if n.InstanceID != "wfi-x" {
		t.Errorf("InstanceID = %q, want wfi-x", n.InstanceID)
	}
	if n.TicketID != "T-1" {
		t.Errorf("TicketID = %q, want T-1", n.TicketID)
	}
	if n.EventType != "orchestration.completed" {
		t.Errorf("EventType = %q, want orchestration.completed", n.EventType)
	}
	if n.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", n.ProjectID, projectID)
	}
	if n.Payload == nil {
		t.Error("Payload is nil, want populated map")
	}
	if errSvc.calls != 0 {
		t.Errorf("RecordError called %d times, want 0", errSvc.calls)
	}
}
