package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/repo"
)

func TestNotificationInboxHandlers(t *testing.T) {
	s := newNotificationServer(t)
	inbox := repo.NewNotificationInboxRepo(s.pool, s.clock)
	for _, project := range []string{"a", "b", ""} {
		if err := inbox.Insert(&model.InboxNotification{ProjectID: project, EventType: "orchestration.failed", Payload: []byte(`{"reason":"**Oops**"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	rr := doNotifyRequest(t, s, s.handleNotificationInbox, http.MethodGet, "/api/v1/notifications", "a", "", nil)
	var result model.NotificationInbox
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil || rr.Code != 200 || result.Total != 1 {
		t.Fatalf("project inbox: %d %s %v", rr.Code, rr.Body.String(), err)
	}
	rr = doNotifyRequest(t, s, s.handleNotificationInboxRead, http.MethodPost, "/api/v1/notifications/read", "a", `{}`, nil)
	if rr.Code != 200 {
		t.Fatalf("read: %d %s", rr.Code, rr.Body.String())
	}
	rr = doNotifyRequest(t, s, s.handleNotificationInbox, http.MethodGet, "/api/v1/notifications/global", "a", "", nil)
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil || result.Total != 3 || result.Unread != 2 {
		t.Fatalf("global inbox: %s %v", rr.Body.String(), err)
	}
	rr = doNotifyRequest(t, s, s.handleNotificationInbox, http.MethodGet, "/api/v1/notifications", "", "", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing scope: %d", rr.Code)
	}
}

func TestNotificationInboxGlobalRejectsProjectPrincipals(t *testing.T) {
	s := newNotificationServer(t)
	for _, path := range []string{"/api/v1/notifications/global", "/api/v1/notifications/global/read"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req = req.WithContext(context.WithValue(req.Context(), servicePrincipalKey, &ServicePrincipal{ProjectID: "a"}))
		rr := httptest.NewRecorder()
		if strings.HasSuffix(path, "/read") {
			s.handleNotificationInboxRead(rr, req)
		} else {
			s.handleNotificationInbox(rr, req)
		}
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestExactInboxNotificationScope(t *testing.T) {
	s := newNotificationServer(t)
	n := &model.InboxNotification{ProjectID: "a", EventType: "orchestration.completed", Payload: []byte(`{"reason":"done"}`)}
	if err := repo.NewNotificationInboxRepo(s.pool, s.clock).Insert(n); err != nil {
		t.Fatal(err)
	}
	rr := doNotifyRequest(t, s, s.handleGetInboxNotification, http.MethodGet, "/api/v1/notifications/"+n.ID, "b", "", map[string]string{"id": n.ID})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-project read: %d", rr.Code)
	}
	rr = doNotifyRequest(t, s, s.handleGetInboxNotification, http.MethodGet, "/api/v1/notifications/global/"+n.ID, "", "", map[string]string{"id": n.ID})
	if rr.Code != http.StatusOK {
		t.Fatalf("exact global item: %d %s", rr.Code, rr.Body.String())
	}
}

func TestNotificationInboxQueryCannotOverrideBearerScope(t *testing.T) {
	for _, principal := range []struct {
		key   contextKey
		value any
	}{
		{servicePrincipalKey, &ServicePrincipal{ProjectID: "a"}},
		{agentSessionKey, &model.AgentSession{ProjectID: "a"}},
	} {
		for _, path := range []string{"/api/v1/notifications?project=b", "/api/v1/notifications/read?project=b", "/api/v1/notifications/item?project=b"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			ctx := context.WithValue(req.Context(), projectKey, "a")
			req = req.WithContext(context.WithValue(ctx, principal.key, principal.value))
			rr := httptest.NewRecorder()
			_, ok := inboxProject(rr, req)
			if ok || rr.Code != http.StatusForbidden {
				t.Fatalf("%s escaped bearer scope: %d", path, rr.Code)
			}
		}
	}
}
