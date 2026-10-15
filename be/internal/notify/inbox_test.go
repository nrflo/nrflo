package notify

import (
	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
	"testing"
)

func TestDispatcherInboxAlwaysReceivesOneCopy(t *testing.T) {
	database, project, workflow := setupNotifyDB(t)
	clk := clock.Real()
	channels := repo.NewNotificationChannelRepo(database, clk)
	deliveries := repo.NewNotificationDeliveryRepo(database, clk)
	inbox := repo.NewNotificationInboxRepo(database, clk)
	dispatcher := NewDispatcher(channels, deliveries, nil, nil, make(chan struct{}, 1), inbox, nil)
	event := ws.NewEvent(ws.EventOrchestrationCompleted, project, "T-1", workflow, map[string]any{"workflow_final_result": "**Done**"})
	dispatcher.OnEvent(event)
	result, err := inbox.List(&project, 50, 0)
	if err != nil || result.Total != 1 || result.Unread != 1 {
		t.Fatalf("inbox without channels: %+v, %v", result, err)
	}
	for _, name := range []string{"one", "two"} {
		insertNotifyChannel(t, channels, project, workflow, name, true, []string{event.Type})
	}
	dispatcher.OnEvent(event)
	result, err = inbox.List(&project, 50, 0)
	if err != nil || result.Total != 2 {
		t.Fatalf("inbox with two channels: %+v, %v", result, err)
	}
	pending, err := deliveries.ListPending(clk.Now(), 50)
	if err != nil || len(pending) != 2 {
		t.Fatalf("external deliveries: %+v, %v", pending, err)
	}
	for _, delivery := range pending {
		if err := deliveries.UpdateStatus(delivery.ID, model.DeliveryStatusGivingUp, 3, "offline", nil); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := inbox.CountUnread(); err != nil || count != 2 {
		t.Fatalf("failed external sends changed inbox: %d, %v", count, err)
	}
}

func TestDispatcherInboxGeneralAndNonWorkflowEvents(t *testing.T) {
	database, project, _ := setupNotifyDB(t)
	clk := clock.Real()
	inbox := repo.NewNotificationInboxRepo(database, clk)
	dispatcher := NewDispatcher(repo.NewNotificationChannelRepo(database, clk), repo.NewNotificationDeliveryRepo(database, clk), nil, nil, make(chan struct{}, 1), inbox, nil)
	dispatcher.OnEvent(ws.NewEvent(ws.EventOrchestrationFailed, "", "", "", map[string]any{"reason": "general"}))
	dispatcher.OnEvent(ws.NewEvent(ws.EventAgentStallRestart, project, "", "", nil))
	dispatcher.OnEvent(ws.NewEvent(ws.EventAgentCompleted, project, "", "", map[string]any{"result": "pass"}))
	result, err := inbox.List(nil, 50, 0)
	if err != nil || result.Total != 2 {
		t.Fatalf("combined inbox: %+v, %v", result, err)
	}
	result, err = inbox.List(&project, 50, 0)
	if err != nil || result.Total != 1 {
		t.Fatalf("project inbox: %+v, %v", result, err)
	}
}
