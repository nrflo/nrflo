package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleListNotificationChannels_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleListNotificationChannels, http.MethodGet,
		"/api/v1/workflows/wf-x/notification-channels", "", "", map[string]string{"wid": "wf-x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleCreateNotificationChannel_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/workflows/wf-x/notification-channels", "", `{"name":"x","kind":"slack"}`,
		map[string]string{"wid": "wf-x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleGetNotificationChannel_MissingProject(t *testing.T) {
	s := newNotificationServer(t)
	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/workflows/wf-x/notification-channels/x", "", "",
		map[string]string{"wid": "wf-x", "id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleListNotificationDeliveries_MissingChannelID(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-ndel-miss"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'T', '/tmp', ?, ?)`, projectID, now, now)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/wf-x/notification-deliveries", nil)
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	req.SetPathValue("wid", "wf-x")
	rr := httptest.NewRecorder()
	s.handleListNotificationDeliveries(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateNotificationChannel_InvalidKind(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-badkind"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'T', '/tmp', ?, ?)`, projectID, now, now)

	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/workflows/wf-bad/notification-channels", projectID, `{"name":"x","kind":"sms"}`,
		map[string]string{"wid": "wf-bad"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetNotificationChannel_NotFound(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-notify-nf"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'T', '/tmp', ?, ?)`, projectID, now, now)

	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/workflows/wf-nf/notification-channels/no-such", projectID, "",
		map[string]string{"wid": "wf-nf", "id": "no-such"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleGetNotificationChannel_CrossWorkflow_NotFound verifies that a channel
// created under wf-a is not accessible via the wf-b path.
func TestHandleGetNotificationChannel_CrossWorkflow_NotFound(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-xwf"
	workflowA := "wf-cross-a"
	workflowB := "wf-cross-b"
	seedNotifyProjectAndWorkflow(t, s, projectID, workflowA)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, '', ?, ?)`,
		workflowB, projectID, now, now); err != nil {
		t.Fatalf("seed wf-b: %v", err)
	}

	ch := createNotifyChannel(t, s, projectID, workflowA)

	rr := doNotifyRequest(t, s, s.handleGetNotificationChannel, http.MethodGet,
		"/api/v1/workflows/"+workflowB+"/notification-channels/"+ch.ID, projectID, "",
		map[string]string{"wid": workflowB, "id": ch.ID})
	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-workflow GET: status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateNotificationChannel_WorkflowNotFound(t *testing.T) {
	s := newNotificationServer(t)
	projectID := "proj-wf-notfound"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'T', '/tmp', ?, ?)`, projectID, now, now)

	rr := doNotifyRequest(t, s, s.handleCreateNotificationChannel, http.MethodPost,
		"/api/v1/workflows/no-such-wf/notification-channels", projectID,
		`{"name":"ch","kind":"slack"}`, map[string]string{"wid": "no-such-wf"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("create with unknown workflow: status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "not found") {
		t.Errorf("body %q should contain 'not found'", rr.Body.String())
	}
}
