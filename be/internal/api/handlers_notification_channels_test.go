package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/service"
	"be/internal/ws"
)

func newNotificationServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "notify_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	notifyWakeCh := make(chan struct{}, 8)
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})
	return &Server{
		pool:        pool,
		clock:       clock.Real(),
		wsHub:       hub,
		notifyWaker: service.NewChanWaker(notifyWakeCh),
	}
}

func seedNotifyProjectAndWorkflow(t *testing.T, s *Server, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("seedNotifyProjectAndWorkflow project(%q): %v", projectID, err)
	}
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, '', ?, ?)`,
		workflowID, projectID, now, now); err != nil {
		t.Fatalf("seedNotifyProjectAndWorkflow workflow(%q): %v", workflowID, err)
	}
}

func doNotifyRequest(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, path, projectID, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if projectID != "" {
		ctx := context.WithValue(req.Context(), projectKey, projectID)
		req = req.WithContext(ctx)
	}
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func createNotifyChannel(t *testing.T, s *Server, projectID, workflowID string) *model.NotificationChannel {
	t.Helper()
	body := `{"name":"Test Channel","kind":"slack","config":{"webhook_url":"https://example.com/ABCDEFGH"},"event_types":["orchestration.completed"]}`
	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/workflows/"+workflowID+"/notification-channels", projectID, body,
		map[string]string{"wid": workflowID})
	if rr.Code != http.StatusCreated {
		t.Fatalf("createNotifyChannel: status = %d, body: %s", rr.Code, rr.Body.String())
	}
	var ch model.NotificationChannel
	if err := json.NewDecoder(rr.Body).Decode(&ch); err != nil {
		t.Fatalf("decode channel: %v", err)
	}
	return &ch
}

func extractWebhookURL(t *testing.T, configJSON string) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		t.Fatalf("extractWebhookURL: %v", err)
	}
	v, _ := m["webhook_url"].(string)
	return v
}

func TestHandleNotificationChannels_FullLifecycle(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-lifecycle"
	workflowID := "wf-lifecycle"
	seedNotifyProjectAndWorkflow(t, s, projectID, workflowID)

	ch := createNotifyChannel(t, s, projectID, workflowID)
	if ch.ID == "" {
		t.Errorf("Create: ID not set")
	}
	if ch.Name != "Test Channel" {
		t.Errorf("Create: Name = %q, want 'Test Channel'", ch.Name)
	}
	if !strings.Contains(ch.Config, "****") {
		t.Errorf("Create: Config not masked: %q", ch.Config)
	}

	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID, projectID, "",
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("Get: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != ch.ID {
		t.Errorf("Get: ID = %q, want %q", got.ID, ch.ID)
	}

	maskedURL := extractWebhookURL(t, ch.Config)
	patchBody := `{"config":{"webhook_url":"` + maskedURL + `"},"name":"Updated"}`
	rr = doNotifyRequest(t, s, s.handleUpdateNotificationChannel, http.MethodPatch,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID, projectID, patchBody,
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH masked: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var patched model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&patched)
	if patched.Name != "Updated" {
		t.Errorf("PATCH: Name = %q, want Updated", patched.Name)
	}

	freshPatch := `{"config":{"webhook_url":"https://hooks.slack.com/new-fresh-ZZZZ"}}`
	rr = doNotifyRequest(t, s, s.handleUpdateNotificationChannel, http.MethodPatch,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID, projectID, freshPatch,
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH fresh: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	rr = doNotifyRequest(t, s, s.handleListNotificationChannels, http.MethodGet,
		"/api/v1/workflows/"+workflowID+"/notification-channels", projectID, "",
		map[string]string{"wid": workflowID})
	if rr.Code != http.StatusOK {
		t.Errorf("List: status = %d, want 200", rr.Code)
	}
	var list []model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}

	rr = doNotifyRequest(t, s, s.handleDeleteNotificationChannel, http.MethodDelete,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID, projectID, "",
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusNoContent {
		t.Errorf("Delete: status = %d, want 204", rr.Code)
	}

	rr = doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID, projectID, "",
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusNotFound {
		t.Errorf("Get after Delete: status = %d, want 404", rr.Code)
	}
}

func TestHandleTestNotificationChannel_InsertsDelivery(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-test"
	workflowID := "wf-test-send"
	seedNotifyProjectAndWorkflow(t, s, projectID, workflowID)

	ch := createNotifyChannel(t, s, projectID, workflowID)

	rr := doNotifyRequest(t, s, s.handleTestNotificationChannel, http.MethodPost,
		"/api/v1/workflows/"+workflowID+"/notification-channels/"+ch.ID+"/test", projectID, "",
		map[string]string{"wid": workflowID, "id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("TestSend: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "queued" {
		t.Errorf("TestSend: status = %q, want queued", resp["status"])
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workflows/"+workflowID+"/notification-deliveries?channel_id="+ch.ID, nil)
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	req.SetPathValue("wid", workflowID)
	rr2 := httptest.NewRecorder()
	s.handleListNotificationDeliveries(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("ListDeliveries: status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	var deliveries []model.NotificationDelivery
	json.NewDecoder(rr2.Body).Decode(&deliveries)
	if len(deliveries) != 1 {
		t.Errorf("deliveries count = %d, want 1", len(deliveries))
	}
	if len(deliveries) > 0 && deliveries[0].EventType != "test" {
		t.Errorf("EventType = %q, want test", deliveries[0].EventType)
	}
}
