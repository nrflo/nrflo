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

// setupNotifyDB seeds a project + workflow and returns (db, projectID, workflowID).
func setupNotifyDB(t *testing.T) (db.Querier, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-n', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err = database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-n', 'proj-n', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	return database, "proj-n", "wf-n"
}

func insertNotifyChannel(t *testing.T, channelRepo *repo.NotificationChannelRepo, projectID, workflowID, name string, enabled bool, eventTypes []string) *model.NotificationChannel {
	t.Helper()
	ch := &model.NotificationChannel{
		ProjectID:  projectID,
		WorkflowID: workflowID,
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
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "ch1", true, []string{"orchestration.completed"})

	d.OnEvent(ws.NewEvent("ticket.updated", projectID, "", workflowID, nil))

	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("deliveries = %d, want 0 for non-watched event", len(results))
	}
}

func TestDispatcher_OnEvent_AgentCompleted_PassDoesNotNotify(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "ch2", true, []string{ws.EventAgentCompleted})

	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", workflowID, map[string]interface{}{"result": "pass"}))
	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 0 {
		t.Errorf("deliveries on pass = %d, want 0", len(results))
	}

	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", workflowID, map[string]interface{}{"result": "fail"}))
	results2, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results2) != 1 {
		t.Errorf("deliveries on fail = %d, want 1", len(results2))
	}
}

func TestDispatcher_OnEvent_WatchedEvent_InsertsDelivery(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "slack-ch", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, map[string]interface{}{
		"workflow": workflowID,
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

// TestDispatcher_OnEvent_ScriptKind_InsertsDelivery pins down that the
// dispatcher allow-list accepts ChannelKindScript alongside slack/telegram.
// Regression guard for the bug where the script kind was added to the model,
// migration, transport, runtime, and validator but missed in notify.go's
// per-channel kind check — silently dropping every script delivery.
func TestDispatcher_OnEvent_ScriptKind_InsertsDelivery(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := &model.NotificationChannel{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Name:       "script-ch",
		Kind:       model.ChannelKindScript,
		Enabled:    true,
		Config:     `{"script_code":"pass"}`,
		EventTypes: []string{ws.EventOrchestrationCompleted},
	}
	if err := channelRepo.Insert(ch); err != nil {
		t.Fatalf("insert script channel: %v", err)
	}

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, nil))

	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("script deliveries = %d, want 1 (dispatcher must allow ChannelKindScript)", len(results))
	}
	if results[0].Status != model.DeliveryStatusPending {
		t.Errorf("status = %q, want pending", results[0].Status)
	}
}

func TestDispatcher_OnEvent_WakeCh_NonBlockingWhenFull(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	wakeCh := make(chan struct{}, 1)
	wakeCh <- struct{}{}

	insertNotifyChannel(t, channelRepo, projectID, workflowID, "ch3", true, []string{ws.EventOrchestrationCompleted})
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, nil))
}

func TestDispatcher_OnEvent_EmptyProjectID_Ignored(t *testing.T) {
	database, _, _ := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, "", "", "", nil))
}

func TestDispatcher_OnEvent_EmptyWorkflowID_Ignored(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "ch-wfempty", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", "", nil))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 0 {
		t.Errorf("empty workflow: deliveries = %d, want 0", len(results))
	}
}

func TestDispatcher_OnEvent_DisabledChannel_SkipsDelivery(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "disabled", false, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, nil))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 0 {
		t.Errorf("disabled channel got %d deliveries, want 0", len(results))
	}
}

func TestDispatcher_WatchedEvents_AllFiveTypes(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 10)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	eventTypes := []string{
		ws.EventOrchestrationCompleted,
		ws.EventOrchestrationFailed,
		ws.EventAgentContextSaving,
		ws.EventAgentStallRestart,
	}
	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "all-events", true, eventTypes)

	for _, et := range eventTypes {
		d.OnEvent(ws.NewEvent(et, projectID, "", workflowID, nil))
	}
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", workflowID, map[string]interface{}{"result": "fail"}))

	ch2 := insertNotifyChannel(t, channelRepo, projectID, workflowID, "agent-ch", true, []string{ws.EventAgentCompleted})
	d.OnEvent(ws.NewEvent(ws.EventAgentCompleted, projectID, "", workflowID, map[string]interface{}{"result": "fail"}))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 20)
	if len(results) != 4 {
		t.Errorf("ch deliveries = %d, want 4", len(results))
	}
	results2, _ := deliveryRepo.ListByChannel(ch2.ID, 20)
	if len(results2) != 1 {
		t.Errorf("ch2 deliveries = %d, want 1", len(results2))
	}
}

func TestDispatcher_OnEvent_WorkflowFinalResultPreservedInPayload(t *testing.T) {
	database, projectID, workflowID := setupNotifyDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "result-ch", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, map[string]interface{}{
		"workflow":              workflowID,
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
	if payload["workflow"] != workflowID {
		t.Errorf("workflow = %q, want %q", payload["workflow"], workflowID)
	}
}

// TestDispatcher_OnEvent_WorkflowIsolation verifies that events for wf-a only
// enqueue deliveries for wf-a channels, not for wf-b channels watching the same event.
func TestDispatcher_OnEvent_WorkflowIsolation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "isolation.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-iso', 'Iso', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	for _, wfID := range []string{"wf-iso-a", "wf-iso-b"} {
		if _, err = database.Exec(
			`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, 'proj-iso', '', datetime('now'), datetime('now'))`, wfID); err != nil {
			t.Fatalf("insert workflow %s: %v", wfID, err)
		}
	}

	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)
	wakeCh := make(chan struct{}, 10)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, nil, wakeCh)

	chA := insertNotifyChannel(t, channelRepo, "proj-iso", "wf-iso-a", "ch-a", true, []string{ws.EventOrchestrationCompleted})
	chB := insertNotifyChannel(t, channelRepo, "proj-iso", "wf-iso-b", "ch-b", true, []string{ws.EventOrchestrationCompleted})

	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, "proj-iso", "", "wf-iso-a", nil))

	resultsA, err := deliveryRepo.ListByChannel(chA.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel chA: %v", err)
	}
	if len(resultsA) != 1 {
		t.Errorf("chA deliveries = %d, want 1", len(resultsA))
	}
	resultsB, err := deliveryRepo.ListByChannel(chB.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel chB: %v", err)
	}
	if len(resultsB) != 0 {
		t.Errorf("chB deliveries = %d, want 0 (wf-iso-b not targeted)", len(resultsB))
	}
}
