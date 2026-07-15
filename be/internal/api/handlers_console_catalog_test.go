package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

func TestHandleGetConsoleCatalog_ReturnsServerDiscovery(t *testing.T) {
	s, factory := newChatTestServer(t)
	seedConsoleProject(t, s, "proj-console-catalog")
	adminID := createTestUser(t, s, "catalog-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, s, adminID)
	sid, _ := createChatSession(t, s, factory, "proj-console-catalog", cookie)

	chain := s.sessionMgr.LoadAndSave(s.requireProjectAdmin(http.HandlerFunc(s.handleGetConsoleCatalog)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/console/catalog?project=proj-console-catalog", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var catalog types.ConsoleCatalog
	if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	if catalog.ProjectID != "proj-console-catalog" || len(catalog.Engines) != 3 {
		t.Fatalf("catalog = %+v", catalog)
	}
	if len(catalog.Sessions) != 1 || catalog.Sessions[0].SessionID != sid {
		t.Fatalf("sessions = %+v", catalog.Sessions)
	}
}
