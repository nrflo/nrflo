package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// stubErrorRecorder records calls to RecordError.
type stubErrorRecorder struct {
	calls         int
	lastProjectID string
}

func (s *stubErrorRecorder) RecordError(projectID, errorType, instanceID, message string) error {
	s.calls++
	s.lastProjectID = projectID
	return nil
}

func setupQueueDB(t *testing.T) (db.Querier, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err = database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-q', 'Test', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err = database.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES ('wf-q', 'proj-q', '', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	return database, "proj-q", "wf-q"
}

func insertQueueChannel(t *testing.T, cr *repo.NotificationChannelRepo, projectID, workflowID, webhookURL string) *model.NotificationChannel {
	t.Helper()
	ch := &model.NotificationChannel{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Name:       "test-ch",
		Kind:       model.ChannelKindSlack,
		Enabled:    true,
		Config:     `{"webhook_url":"` + webhookURL + `"}`,
		EventTypes: []string{ws.EventOrchestrationCompleted},
	}
	if err := cr.Insert(ch); err != nil {
		t.Fatalf("insertQueueChannel: %v", err)
	}
	return ch
}

func insertPendingDelivery(t *testing.T, dr *repo.NotificationDeliveryRepo, channelID, projectID string) *model.NotificationDelivery {
	t.Helper()
	d := &model.NotificationDelivery{
		ChannelID: channelID,
		ProjectID: projectID,
		EventType: ws.EventOrchestrationCompleted,
		Payload:   `{"workflow":"feature","ticket_id":"T-1"}`,
		Status:    model.DeliveryStatusPending,
	}
	if err := dr.Insert(d); err != nil {
		t.Fatalf("insertPendingDelivery: %v", err)
	}
	return d
}

func TestWorker_HappyPath_StatusSent(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := insertQueueChannel(t, channelRepo, projectID, workflowID, server.URL)
	insertPendingDelivery(t, deliveryRepo, ch.ID, projectID)

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()

	errSvc := &stubErrorRecorder{}
	worker := NewWorker(deliveryRepo, channelRepo, hub, errSvc, clk, make(chan struct{}, 1))
	worker.Tick(context.Background())

	if !called {
		t.Errorf("transport not called")
	}
	results, err := deliveryRepo.ListByChannel(ch.ID, 10)
	if err != nil {
		t.Fatalf("ListByChannel: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(results))
	}
	if results[0].Status != model.DeliveryStatusSent {
		t.Errorf("status = %q, want sent", results[0].Status)
	}
	if results[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", results[0].Attempts)
	}
	if errSvc.calls != 0 {
		t.Errorf("RecordError called %d times, want 0", errSvc.calls)
	}
}

func TestWorker_FailingServer_ExponentialBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	ch := insertQueueChannel(t, channelRepo, projectID, workflowID, server.URL)
	d := insertPendingDelivery(t, deliveryRepo, ch.ID, projectID)

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()

	errSvc := &stubErrorRecorder{}
	worker := NewWorker(deliveryRepo, channelRepo, hub, errSvc, clk, make(chan struct{}, 1))

	// Tick 1: attempt=1, next_attempt=now+15s
	worker.Tick(context.Background())
	results, _ := deliveryRepo.ListByChannel(ch.ID, 10)
	if results[0].Attempts != 1 {
		t.Errorf("after tick1: attempts = %d, want 1", results[0].Attempts)
	}
	if results[0].Status != model.DeliveryStatusPending {
		t.Errorf("after tick1: status = %q, want pending", results[0].Status)
	}
	if results[0].NextAttemptAt == nil {
		t.Fatal("after tick1: NextAttemptAt is nil")
	}
	expected15s := fixedTime.Add(15 * time.Second)
	if !results[0].NextAttemptAt.Equal(expected15s) {
		t.Errorf("after tick1: NextAttemptAt = %v, want %v", results[0].NextAttemptAt, expected15s)
	}

	// Tick before advance — delivery not yet ready
	worker.Tick(context.Background())
	results, _ = deliveryRepo.ListByChannel(ch.ID, 10)
	if results[0].Attempts != 1 {
		t.Errorf("before advance: attempts should still be 1, got %d", results[0].Attempts)
	}

	// Advance 15s, tick 2: attempt=2, next_attempt=now+60s
	clk.Advance(15 * time.Second)
	worker.Tick(context.Background())
	results, _ = deliveryRepo.ListByChannel(ch.ID, 10)
	if results[0].Attempts != 2 {
		t.Errorf("after tick2: attempts = %d, want 2", results[0].Attempts)
	}
	expected60s := fixedTime.Add(15 * time.Second).Add(60 * time.Second)
	if results[0].NextAttemptAt == nil || !results[0].NextAttemptAt.Equal(expected60s) {
		t.Errorf("after tick2: NextAttemptAt = %v, want %v", results[0].NextAttemptAt, expected60s)
	}

	// Advance 60s, tick 3: attempts=3, giving_up, RecordError called
	clk.Advance(60 * time.Second)
	worker.Tick(context.Background())
	results, _ = deliveryRepo.ListByChannel(ch.ID, 10)
	if results[0].Status != model.DeliveryStatusGivingUp {
		t.Errorf("after tick3: status = %q, want giving_up", results[0].Status)
	}
	if results[0].Attempts != 3 {
		t.Errorf("after tick3: attempts = %d, want 3", results[0].Attempts)
	}
	if errSvc.calls != 1 {
		t.Errorf("RecordError calls = %d, want 1", errSvc.calls)
	}
	if errSvc.lastProjectID != projectID {
		t.Errorf("RecordError projectID = %q, want %q", errSvc.lastProjectID, projectID)
	}
	_ = d
}

func TestWorker_NoPendingDeliveries_NoOp(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(fixedTime)
	database, projectID, workflowID := setupQueueDB(t)
	channelRepo := repo.NewNotificationChannelRepo(database, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(database, clk)

	// Channel with a sent delivery — worker should do nothing
	ch := insertQueueChannel(t, channelRepo, projectID, workflowID, "http://ignored")
	d := insertPendingDelivery(t, deliveryRepo, ch.ID, projectID)
	deliveryRepo.UpdateStatus(d.ID, model.DeliveryStatusSent, 1, "", nil)

	errSvc := &stubErrorRecorder{}
	worker := NewWorker(deliveryRepo, channelRepo, nil, errSvc, clk, make(chan struct{}, 1))
	worker.Tick(context.Background())

	// No error — nothing to retry
	if errSvc.calls != 0 {
		t.Errorf("RecordError calls = %d, want 0 (no pending deliveries)", errSvc.calls)
	}
}
