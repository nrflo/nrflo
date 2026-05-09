package api

import (
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
	"be/internal/ws"
)

func newEnvVarServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ev_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})

	projectID := "proj-ev-handler"
	if _, err := pool.Exec(`INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, 'TestProject', datetime('now'), datetime('now'))`, projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return &Server{pool: pool, clock: clock.Real(), wsHub: hub}, projectID
}

func doEnvVarRequest(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, path, projectID, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if projectID != "" {
		req.SetPathValue("id", projectID)
	}
	if name != "" {
		req.SetPathValue("name", name)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func decodeEnvVarList(t *testing.T, rr *httptest.ResponseRecorder) []*model.ProjectEnvVar {
	t.Helper()
	var list []*model.ProjectEnvVar
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return list
}

func decodeEnvVar(t *testing.T, rr *httptest.ResponseRecorder) *model.ProjectEnvVar {
	t.Helper()
	var v model.ProjectEnvVar
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &v
}

func waitForEnvVarWSEvent(t *testing.T, ch <-chan []byte, eventType string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var evt ws.Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				continue
			}
			if evt.Type == eventType {
				return
			}
		case <-deadline:
			t.Errorf("timeout waiting for WS event %q", eventType)
			return
		}
	}
}

// --- List ---

func TestHandleListProjectEnvVars_Empty(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handleListProjectEnvVars, http.MethodGet,
		"/api/v1/projects/"+projectID+"/env-vars", projectID, "", "")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	list := decodeEnvVarList(t, rr)
	if list == nil {
		t.Error("list = nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("list = %d items, want 0", len(list))
	}
}

func TestHandleListProjectEnvVars_MissingProjectID(t *testing.T) {
	s, _ := newEnvVarServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//env-vars", nil)
	rr := httptest.NewRecorder()
	s.handleListProjectEnvVars(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleListProjectEnvVars_WithVars(t *testing.T) {
	s, projectID := newEnvVarServer(t)

	putRR := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/MY_VAR", projectID, "MY_VAR", `{"value":"hello"}`)
	if putRR.Code != http.StatusOK {
		t.Fatalf("Put status = %d; body: %s", putRR.Code, putRR.Body.String())
	}

	rr := doEnvVarRequest(t, s, s.handleListProjectEnvVars, http.MethodGet,
		"/api/v1/projects/"+projectID+"/env-vars", projectID, "", "")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	list := decodeEnvVarList(t, rr)
	if len(list) != 1 {
		t.Fatalf("list = %d items, want 1", len(list))
	}
	if list[0].Name != "MY_VAR" {
		t.Errorf("list[0].Name = %q, want MY_VAR", list[0].Name)
	}
	if list[0].Value != "hello" {
		t.Errorf("list[0].Value = %q, want hello", list[0].Value)
	}
}

// --- Put ---

func TestHandlePutProjectEnvVar_HappyPath(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/GOOD_VAR", projectID, "GOOD_VAR", `{"value":"test-value"}`)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	v := decodeEnvVar(t, rr)
	if v.Name != "GOOD_VAR" {
		t.Errorf("Name = %q, want GOOD_VAR", v.Name)
	}
	if v.Value != "test-value" {
		t.Errorf("Value = %q, want test-value", v.Value)
	}
}

func TestHandlePutProjectEnvVar_MissingProjectID(t *testing.T) {
	s, _ := newEnvVarServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects//env-vars/MY_VAR", strings.NewReader(`{"value":"x"}`))
	req.SetPathValue("name", "MY_VAR")
	rr := httptest.NewRecorder()
	s.handlePutProjectEnvVar(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectEnvVar_InvalidJSON(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/MY_VAR", projectID, "MY_VAR", "not-json")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutProjectEnvVar_InvalidName(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/1INVALID", projectID, "1INVALID", `{"value":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid env var name")
}

func TestHandlePutProjectEnvVar_ReservedName(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/NRF_SESSION_ID", projectID, "NRF_SESSION_ID", `{"value":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "is reserved")
}

func TestHandlePutProjectEnvVar_ReservedPATH(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/PATH", projectID, "PATH", `{"value":"x"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "is reserved")
}

func TestHandlePutProjectEnvVar_ValueTooLong(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	bigValue := strings.Repeat("x", 4097)
	body := `{"value":"` + bigValue + `"}`
	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/BIG_VAR", projectID, "BIG_VAR", body)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "exceeds maximum length")
}

func TestHandlePutProjectEnvVar_Overwrite(t *testing.T) {
	s, projectID := newEnvVarServer(t)

	doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/OVER_VAR", projectID, "OVER_VAR", `{"value":"original"}`)

	rr := doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/OVER_VAR", projectID, "OVER_VAR", `{"value":"updated"}`)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	v := decodeEnvVar(t, rr)
	if v.Value != "updated" {
		t.Errorf("Value = %q, want updated", v.Value)
	}
}

// --- Delete ---

func TestHandleDeleteProjectEnvVar_HappyPath(t *testing.T) {
	s, projectID := newEnvVarServer(t)

	doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/DEL_VAR", projectID, "DEL_VAR", `{"value":"value"}`)

	rr := doEnvVarRequest(t, s, s.handleDeleteProjectEnvVar, http.MethodDelete,
		"/api/v1/projects/"+projectID+"/env-vars/DEL_VAR", projectID, "DEL_VAR", "")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "deleted" {
		t.Errorf("status = %q, want deleted", resp["status"])
	}
}

func TestHandleDeleteProjectEnvVar_NotFound(t *testing.T) {
	s, projectID := newEnvVarServer(t)
	rr := doEnvVarRequest(t, s, s.handleDeleteProjectEnvVar, http.MethodDelete,
		"/api/v1/projects/"+projectID+"/env-vars/MISSING", projectID, "MISSING", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleDeleteProjectEnvVar_MissingProjectID(t *testing.T) {
	s, _ := newEnvVarServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects//env-vars/MY_VAR", nil)
	req.SetPathValue("name", "MY_VAR")
	rr := httptest.NewRecorder()
	s.handleDeleteProjectEnvVar(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Broadcast assertions ---

func TestHandlePutProjectEnvVar_BroadcastsEvent(t *testing.T) {
	s, projectID := newEnvVarServer(t)

	client, ch := ws.NewTestClient(s.wsHub, "ev-put-client")
	s.wsHub.Register(client)

	doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/BC_VAR", projectID, "BC_VAR", `{"value":"broadcast-test"}`)

	waitForEnvVarWSEvent(t, ch, ws.EventProjectEnvVarsUpdated)
}

func TestHandleDeleteProjectEnvVar_BroadcastsEvent(t *testing.T) {
	s, projectID := newEnvVarServer(t)

	doEnvVarRequest(t, s, s.handlePutProjectEnvVar, http.MethodPut,
		"/api/v1/projects/"+projectID+"/env-vars/BC_DEL_VAR", projectID, "BC_DEL_VAR", `{"value":"x"}`)

	client, ch := ws.NewTestClient(s.wsHub, "ev-del-client")
	s.wsHub.Register(client)

	doEnvVarRequest(t, s, s.handleDeleteProjectEnvVar, http.MethodDelete,
		"/api/v1/projects/"+projectID+"/env-vars/BC_DEL_VAR", projectID, "BC_DEL_VAR", "")

	waitForEnvVarWSEvent(t, ch, ws.EventProjectEnvVarsUpdated)
}
