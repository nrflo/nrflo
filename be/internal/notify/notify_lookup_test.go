package notify

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/ws"
)

func setupLookupDB(t *testing.T) (db.Querier, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "lookup.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-lk', 'LK', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err = database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-lk', 'proj-lk', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	return database, "proj-lk", "wf-lk"
}

func TestDispatcher_OnEvent_ProjectLookup_EnrichesPayload(t *testing.T) {
	database, projectID, workflowID := setupLookupDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	insertNotifyChannel(t, channelRepo, projectID, workflowID, "lk-ch", true, []string{ws.EventOrchestrationCompleted})

	projectLookup := ProjectLookupFunc(func(id string) (string, bool, error) {
		return "Enriched Project Name", true, nil
	})

	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, projectLookup, nil, wakeCh)
	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, nil))

	chModel := insertNotifyChannel(t, channelRepo, projectID, workflowID, "dummy", false, nil)
	all, _ := deliveryRepo.ListByChannel(chModel.ID, 10)
	_ = all

	channels, err := channelRepo.ListByWorkflow(projectID, workflowID)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	var deliveryPayload string
	for _, ch := range channels {
		if ch.Name != "lk-ch" {
			continue
		}
		results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
		if len(results) != 1 {
			t.Fatalf("deliveries = %d, want 1", len(results))
		}
		deliveryPayload = results[0].Payload
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(deliveryPayload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["project_name"] != "Enriched Project Name" {
		t.Errorf("project_name = %v, want %q", payload["project_name"], "Enriched Project Name")
	}
}

func TestDispatcher_OnEvent_TicketLookup_EnrichesPayload(t *testing.T) {
	database, projectID, workflowID := setupLookupDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "tk-ch", true, []string{ws.EventOrchestrationCompleted})

	ticketLookup := TicketLookupFunc(func(pid, tid string) (string, bool, error) {
		return "My Ticket Title", true, nil
	})

	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, ticketLookup, wakeCh)

	// Fire event with ticket_id set
	evt := ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "T-99", workflowID, nil)
	d.OnEvent(evt)

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(results))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(results[0].Payload), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["ticket_name"] != "My Ticket Title" {
		t.Errorf("ticket_name = %v, want %q", payload["ticket_name"], "My Ticket Title")
	}
	if payload["ticket_id"] != "T-99" {
		t.Errorf("ticket_id = %v, want T-99", payload["ticket_id"])
	}
}

func TestDispatcher_OnEvent_TicketLookup_NotCalledWithoutTicketID(t *testing.T) {
	database, projectID, workflowID := setupLookupDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "notk-ch", true, []string{ws.EventOrchestrationCompleted})

	var lookupCalled bool
	ticketLookup := TicketLookupFunc(func(pid, tid string) (string, bool, error) {
		lookupCalled = true
		return "Should Not Appear", true, nil
	})

	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, nil, ticketLookup, wakeCh)
	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "", workflowID, nil))

	if lookupCalled {
		t.Error("ticket lookup was called despite no ticket_id in event")
	}

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(results))
	}

	var payload map[string]interface{}
	json.Unmarshal([]byte(results[0].Payload), &payload)
	if _, ok := payload["ticket_name"]; ok {
		t.Errorf("ticket_name present in payload despite no ticket_id: %v", payload["ticket_name"])
	}
}

func TestDispatcher_OnEvent_LookupError_DispatchContinues(t *testing.T) {
	database, projectID, workflowID := setupLookupDB(t)
	clk := clock.Real()
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := insertNotifyChannel(t, channelRepo, projectID, workflowID, "err-ch", true, []string{ws.EventOrchestrationCompleted})

	errProjectLookup := ProjectLookupFunc(func(id string) (string, bool, error) {
		return "", false, errors.New("lookup failed")
	})
	errTicketLookup := TicketLookupFunc(func(pid, tid string) (string, bool, error) {
		return "", false, errors.New("lookup failed")
	})

	wakeCh := make(chan struct{}, 1)
	d := NewDispatcher(channelRepo, deliveryRepo, errProjectLookup, errTicketLookup, wakeCh)
	d.OnEvent(ws.NewEvent(ws.EventOrchestrationCompleted, projectID, "T-1", workflowID, nil))

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1 (lookup error must not block dispatch)", len(results))
	}

	var payload map[string]interface{}
	json.Unmarshal([]byte(results[0].Payload), &payload)
	if v, ok := payload["project_name"]; ok && v != "" {
		t.Errorf("project_name = %v, want empty/absent after lookup error", v)
	}
}
