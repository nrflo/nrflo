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

func seedNotifyProject(t *testing.T, s *Server, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now)
	if err != nil {
		t.Fatalf("seedNotifyProject(%q): %v", projectID, err)
	}
}

func doNotifyRequest(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, path, projectID, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	bodyReader := strings.NewReader(body)
	req := httptest.NewRequest(method, path, bodyReader)
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

func createNotifyChannel(t *testing.T, s *Server, projectID string) *model.NotificationChannel {
	t.Helper()
	body := `{"name":"Test Channel","kind":"slack","config":{"webhook_url":"https://example.com/ABCDEFGH"},"event_types":["orchestration.completed"]}`
	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/notification-channels", projectID, body, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createNotifyChannel: status = %d, body: %s", rr.Code, rr.Body.String())
	}
	var ch model.NotificationChannel
	if err := json.NewDecoder(rr.Body).Decode(&ch); err != nil {
		t.Fatalf("decode channel: %v", err)
	}
	return &ch
}

// --- Missing X-Project header ---

func TestHandleListNotificationChannels_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleListNotificationChannels, http.MethodGet,
		"/api/v1/notification-channels", "", "", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleCreateNotificationChannel_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/notification-channels", "", `{"name":"x","kind":"slack"}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleGetNotificationChannel_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/notification-channels/x", "", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Full lifecycle ---

func TestHandleNotificationChannels_FullLifecycle(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-lifecycle"
	seedNotifyProject(t, s, projectID)

	// Create
	ch := createNotifyChannel(t, s, projectID)
	if ch.ID == "" {
		t.Errorf("Create: ID not set")
	}
	if ch.Name != "Test Channel" {
		t.Errorf("Create: Name = %q, want 'Test Channel'", ch.Name)
	}
	// Config masked in Create response
	if !strings.Contains(ch.Config, "****") {
		t.Errorf("Create: Config not masked: %q", ch.Config)
	}

	// Get
	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/notification-channels/"+ch.ID, projectID, "", map[string]string{"id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("Get: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != ch.ID {
		t.Errorf("Get: ID = %q, want %q", got.ID, ch.ID)
	}

	// PATCH — masked webhook_url echoed back → secret preserved
	maskedURL := extractWebhookURL(t, ch.Config)
	patchBody := `{"config":{"webhook_url":"` + maskedURL + `"},"name":"Updated"}`
	rr = doNotifyRequest(t, s, s.handleUpdateNotificationChannel, http.MethodPatch,
		"/api/v1/notification-channels/"+ch.ID, projectID, patchBody, map[string]string{"id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH masked: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var patched model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&patched)
	if patched.Name != "Updated" {
		t.Errorf("PATCH: Name = %q, want Updated", patched.Name)
	}

	// PATCH — fresh webhook_url → rotates
	freshPatch := `{"config":{"webhook_url":"https://hooks.slack.com/new-fresh-ZZZZ"}}`
	rr = doNotifyRequest(t, s, s.handleUpdateNotificationChannel, http.MethodPatch,
		"/api/v1/notification-channels/"+ch.ID, projectID, freshPatch, map[string]string{"id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH fresh: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// List — channel appears
	rr = doNotifyRequest(t, s, s.handleListNotificationChannels, http.MethodGet,
		"/api/v1/notification-channels", projectID, "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("List: status = %d, want 200", rr.Code)
	}
	var list []model.NotificationChannel
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}

	// Delete
	rr = doNotifyRequest(t, s, s.handleDeleteNotificationChannel, http.MethodDelete,
		"/api/v1/notification-channels/"+ch.ID, projectID, "", map[string]string{"id": ch.ID})
	if rr.Code != http.StatusNoContent {
		t.Errorf("Delete: status = %d, want 204", rr.Code)
	}

	// Get after delete — 404
	rr = doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/notification-channels/"+ch.ID, projectID, "", map[string]string{"id": ch.ID})
	if rr.Code != http.StatusNotFound {
		t.Errorf("Get after Delete: status = %d, want 404", rr.Code)
	}
}

func TestHandleTestNotificationChannel_InsertsDelivery(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-test"
	seedNotifyProject(t, s, projectID)

	ch := createNotifyChannel(t, s, projectID)

	rr := doNotifyRequest(t, s, s.handleTestNotificationChannel, http.MethodPost,
		"/api/v1/notification-channels/"+ch.ID+"/test", projectID, "", map[string]string{"id": ch.ID})
	if rr.Code != http.StatusOK {
		t.Errorf("TestSend: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "queued" {
		t.Errorf("TestSend: status = %q, want queued", resp["status"])
	}

	// List deliveries — must have exactly one row
	rr = doNotifyRequest(t, s, s.handleListNotificationDeliveries, http.MethodGet,
		"/api/v1/notification-deliveries?channel_id="+ch.ID, projectID, "",
		map[string]string{})
	rr.Result().Request, _ = http.NewRequest(http.MethodGet,
		"/api/v1/notification-deliveries?channel_id="+ch.ID, nil)
	// Use URL query for channel_id
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/notification-deliveries?channel_id="+ch.ID, nil)
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
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

func TestHandleListNotificationDeliveries_MissingChannelID(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-ndel-miss"
	seedNotifyProject(t, s, projectID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notification-deliveries", nil)
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleListNotificationDeliveries(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateNotificationChannel_InvalidKind(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-badkind"
	seedNotifyProject(t, s, projectID)

	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/notification-channels", projectID, `{"name":"x","kind":"sms"}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetNotificationChannel_NotFound(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-nf"
	seedNotifyProject(t, s, projectID)

	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/notification-channels/no-such", projectID, "", map[string]string{"id": "no-such"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// extractWebhookURL extracts the webhook_url from a masked config JSON string.
func extractWebhookURL(t *testing.T, configJSON string) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		t.Fatalf("extractWebhookURL: %v", err)
	}
	v, _ := m["webhook_url"].(string)
	return v
}
