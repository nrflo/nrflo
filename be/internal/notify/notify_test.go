package notify

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

func setupNotifyDB(t *testing.T) (db.Querier, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	_, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-n', 'Test', datetime('now'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	return database, "proj-n"
}

func insertNotifyChannel(t *testing.T, channelRepo *repo.NotificationChannelRepo, projectID, name string, enabled bool, eventTypes []string) *model.NotificationChannel {
	t.Helper()
	ch := &model.NotificationChannel{
		ProjectID:  projectID,
		Name:       name,
		Kind:       model.ChannelKindSlack,
		Enabled:    enabled,
		Config:     `{"webhook_url":"https://example.com/hook"}`,
		EventTypes: eventTypes,
	}
	if err := channelRepo.Insert(ch); err != nil {
		t.Fatalf("insert channel %q: %v", name, err)
	}
	return ch
}

func TestDispatcher_OnEvent_IgnoresNonWatchedEventType(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, "ch1", true, []string{"orchestration.completed"})

	// Non-watched event — should produce no deliveries
	d.OnEvent(ws.NewEvent("ticket.updated", projectID, "", "", nil))

	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("deliveries = %d, want 0 for non-watched event", len(results))
	}
}

func TestDispatcher_OnEvent_AgentCompleted_PassDoesNotNotify(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, "ch2", true, []string{ws.EventAgentCompleted})

	// result=pass: must NOT trigger delivery
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", "", map[string]interface{}{"result": "pass"}))
	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 0 {
		t.Errorf("deliveries on pass = %d, want 0", len(results))
	}

	// result=fail: MUST trigger delivery
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", "", map[string]interface{}{"result": "fail"}))
	results2, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results2) != 1 {
		t.Errorf("deliveries on fail = %d, want 1", len(results2))
	}
}

func TestDispatcher_OnEvent_WatchedEvent_InsertsDelivery(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, "slack-ch", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", "feature", map[string]interface{}{
		"workflow": "feature",
	}))

	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(results))
	}
	if results[0].Status != model.DeliveryStatusPending {
		t.Errorf("status = %q, want pending", results[0].Status)
	}
	if results[0].EventType != ws.EventOrchestrationCompleted {
		t.Errorf("EventType = %q, want %q", results[0].EventType, ws.EventOrchestrationCompleted)
	}
}

func TestDispatcher_OnEvent_WakeCh_NonBlockingWhenFull(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	// Pre-fill channel so it cannot accept another signal
	wakeCh := make(chan struct{}, 1)
	wakeCh <- struct{}{}

	insertNotifyChannel(t, channelRepo, projectID, "ch3", true, []string{ws.EventOrchestrationCompleted})
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	// Must not block or panic
	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", "", nil))
}

func TestDispatcher_OnEvent_EmptyProjectID_Ignored(t *testing.T) {
	database, _ := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	// Event with no project ID must not panic or produce deliveries
	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, "", "", "", nil))
}

func TestDispatcher_OnEvent_DisabledChannel_SkipsDelivery(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, "disabled", false, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", "", nil))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 0 {
		t.Errorf("disabled channel got %d deliveries, want 0", len(results))
	}
}

func TestDispatcher_WatchedEvents_AllFiveTypes(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 10)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	eventTypes := []string{
		ws.EventOrchestrationCompleted,
		ws.EventOrchestrationFailed,
		ws.EventAgentContextSaving,
		ws.EventAgentStallRestart,
	}
	ch := insertNotifyChannel(t, channelRepo, projectID, "all-events", true, eventTypes)

	for _, et := range eventTypes {
		d.OnEvent(ws.NewEvent(et, projectID, "", "", nil))
	}
	// Also agent.completed with fail
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", "", map[string]interface{}{"result": "fail"}))

	// Insert channel for agent.completed
	ch2 := insertNotifyChannel(t, channelRepo, projectID, "agent-ch", true, []string{ws.EventAgentCompleted})
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", "", map[string]interface{}{"result": "fail"}))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 20)
	if len(results) != 4 {
		t.Errorf("ch deliveries = %d, want 4 (one per watched non-agent-completed event)", len(results))
	}
	results2, _ := deliveryRepo.ListByChannel(ch2.ID, 20)
	if len(results2) != 1 {
		t.Errorf("ch2 deliveries = %d, want 1", len(results2))
	}
}

func TestDispatcher_OnEvent_WorkflowFinalResultPreservedInPayload(t *testing.T) {
	database, projectID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, "result-ch", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", "feature", map[string]interface{}{
		"workflow":              "feature",
		"instance_id":          "wfi-123",
		"workflow_final_result": "Build completed successfully",
	}))

	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(results))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(results[0].Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["workflow_final_result"] != "Build completed successfully" {
		t.Errorf("workflow_final_result = %q, want %q", payload["workflow_final_result"], "Build completed successfully")
	}
	if payload["workflow"] != "feature" {
		t.Errorf("workflow = %q, want %q", payload["workflow"], "feature")
	}
}
