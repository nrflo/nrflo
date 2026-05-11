package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

func TestWorker_RendersTemplate_BodyContainsRenderedOutput(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		json.NewDecoder(r.Body).Decode(&p)
		capturedBody = p["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := &model.NotificationChannel{
		ProjectID:       projectID,
		WorkflowID:      workflowID,
		Name:            "render-test",
		Kind:            model.ChannelKindSlack,
		Enabled:         true,
		Config:          `{"webhook_url":"` + server.URL + `"}`,
		MessageTemplate: "event: ${event_type} | wf: ${workflow}",
		EventTypes:      []string{ws.EventOrchestrationCompleted},
	}
	if err := channelRepo.Insert(ch); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	d := &model.NotificationDelivery{
		ChannelID: ch.ID,
		ProjectID: projectID,
		EventType: ws.EventOrchestrationCompleted,
		Payload:   `{"event_type":"orchestration.completed","workflow":"feature"}`,
		Status:    model.DeliveryStatusPending,
	}
	if err := deliveryRepo.Insert(d); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}

	worker := NewWorker(deliveryRepo, channelRepo, nil, &stubErrorRecorder{}, clk, make(chan struct{}, 1))
	worker.Tick(context.Background())

	want := "event: orchestration.completed | wf: feature"
	if capturedBody != want {
		t.Errorf("rendered body = %q, want %q", capturedBody, want)
	}

	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if len(results) != 1 || results[0].Status != model.DeliveryStatusSent {
		t.Errorf("delivery status after render = %v, want sent", results[0].Status)
	}
}

func TestWorker_DefaultTemplate_Rendered(t *testing.T) {
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		json.NewDecoder(r.Body).Decode(&p)
		capturedBody = p["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := &model.NotificationChannel{
		ProjectID:       projectID,
		WorkflowID:      workflowID,
		Name:            "default-tpl",
		Kind:            model.ChannelKindSlack,
		Enabled:         true,
		Config:          `{"webhook_url":"` + server.URL + `"}`,
		MessageTemplate: DefaultTemplate(model.ChannelKindSlack),
		EventTypes:      []string{ws.EventOrchestrationCompleted},
	}
	if err := channelRepo.Insert(ch); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	d := &model.NotificationDelivery{
		ChannelID: ch.ID,
		ProjectID: projectID,
		EventType: ws.EventOrchestrationCompleted,
		Payload:   `{"event_type":"orchestration.completed","workflow":"bugfix","instance_id":"wfi-001"}`,
		Status:    model.DeliveryStatusPending,
	}
	if err := deliveryRepo.Insert(d); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}

	worker := NewWorker(deliveryRepo, channelRepo, nil, &stubErrorRecorder{}, clk, make(chan struct{}, 1))
	worker.Tick(context.Background())

	if capturedBody == "" {
		t.Error("captured body is empty; expected rendered default template")
	}
	for _, s := range []string{"orchestration.completed", "bugfix"} {
		if !strings.Contains(capturedBody, s) {
			t.Errorf("rendered body missing %q: %q", s, capturedBody)
		}
	}
}
