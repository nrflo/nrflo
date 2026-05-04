package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/ws"
)

// projectFindingsWSEnv is a test env with a running wsHub for upsert/delete handler tests.
type projectFindingsWSEnv struct {
	s   *Server
	rec *wsRecorder
}

func newProjectFindingsWSEnv(t *testing.T) *projectFindingsWSEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "pf_ws_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	rec := &wsRecorder{ch: make(chan *ws.Event, 32)}
	hub.RegisterListener(rec)
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})
	return &projectFindingsWSEnv{
		s:   &Server{pool: pool, clock: clock.Real(), wsHub: hub},
		rec: rec,
	}
}

// --- POST (upsert) ---

// TestHandleUpsertProjectFinding_CreatesThenGets verifies POST creates a new key
// and a subsequent GET reflects it.
func TestHandleUpsertProjectFinding_CreatesThenGets(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-upsert-new"
	seedProjectForFindings(t, env.s, pid)

	body, _ := json.Marshal(map[string]string{"key": "status", "value": "running"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/findings", strings.NewReader(string(body)))
	req.SetPathValue("id", pid)
	rr := httptest.NewRecorder()
	env.s.handleUpsertProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "saved" {
		t.Errorf("response.status = %q, want saved", resp["status"])
	}
	if resp["key"] != "status" {
		t.Errorf("response.key = %q, want status", resp["key"])
	}

	getReq := projectFindingsReq(t, pid)
	getRR := httptest.NewRecorder()
	env.s.handleGetProjectFindings(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRR.Code)
	}
	m := decodeMapResponse(t, getRR)
	if m["status"] != "running" {
		t.Errorf("findings[status] = %v, want running", m["status"])
	}
}

// TestHandleUpsertProjectFinding_OverwritesExistingKey verifies that re-posting
// with the same key updates the value (upsert) and does not create a duplicate.
func TestHandleUpsertProjectFinding_OverwritesExistingKey(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-upsert-upd"
	addProjectFinding(t, env.s, pid, "phase", "init")

	body, _ := json.Marshal(map[string]string{"key": "phase", "value": "done"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/findings", strings.NewReader(string(body)))
	req.SetPathValue("id", pid)
	rr := httptest.NewRecorder()
	env.s.handleUpsertProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	m := decodeMapResponse(t, func() *httptest.ResponseRecorder {
		getRR := httptest.NewRecorder()
		env.s.handleGetProjectFindings(getRR, projectFindingsReq(t, pid))
		return getRR
	}())
	if len(m) != 1 {
		t.Errorf("len(findings) = %d, want 1 (no duplicate after upsert)", len(m))
	}
	if m["phase"] != "done" {
		t.Errorf("findings[phase] = %v, want done", m["phase"])
	}
}

// TestHandleUpsertProjectFinding_EmptyKey_Returns400 verifies that an empty key
// field returns HTTP 400.
func TestHandleUpsertProjectFinding_EmptyKey_Returns400(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-upsert-emptykey"
	seedProjectForFindings(t, env.s, pid)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/findings",
		strings.NewReader(`{"key":"","value":"x"}`))
	req.SetPathValue("id", pid)
	rr := httptest.NewRecorder()
	env.s.handleUpsertProjectFinding(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "key")
}

// TestHandleUpsertProjectFinding_MissingProjectID_Returns400 verifies that a missing
// project ID path value returns HTTP 400.
func TestHandleUpsertProjectFinding_MissingProjectID_Returns400(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects//findings",
		strings.NewReader(`{"key":"k","value":"v"}`))
	// PathValue("id") returns "" when not set.
	rr := httptest.NewRecorder()
	env.s.handleUpsertProjectFinding(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID")
}

// TestHandleUpsertProjectFinding_BroadcastsWSEvent verifies that a successful POST
// emits exactly one project_findings.updated event with action "add".
func TestHandleUpsertProjectFinding_BroadcastsWSEvent(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-upsert-ws"
	seedProjectForFindings(t, env.s, pid)

	body, _ := json.Marshal(map[string]string{"key": "mykey", "value": "myval"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/findings", strings.NewReader(string(body)))
	req.SetPathValue("id", pid)
	rr := httptest.NewRecorder()
	env.s.handleUpsertProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", rr.Code)
	}

	ev := env.rec.waitEvent(t, ws.EventProjectFindingsUpdated)
	if ev.Data["key"] != "mykey" {
		t.Errorf("event.key = %v, want mykey", ev.Data["key"])
	}
	if ev.Data["action"] != "add" {
		t.Errorf("event.action = %v, want add", ev.Data["action"])
	}
	if ev.ProjectID != pid {
		t.Errorf("event.project_id = %q, want %q", ev.ProjectID, pid)
	}

	env.rec.mu.Lock()
	count := len(env.rec.events)
	env.rec.mu.Unlock()
	if count != 1 {
		t.Errorf("event count = %d, want 1", count)
	}
}

// --- DELETE ---

// TestHandleDeleteProjectFinding_DeletesExistingKey verifies DELETE removes the key,
// returns 200 {status:deleted,key}, and the key is absent on subsequent GET.
func TestHandleDeleteProjectFinding_DeletesExistingKey(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-del-ok"
	addProjectFinding(t, env.s, pid, "result", "pass")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+pid+"/findings/result", nil)
	req.SetPathValue("id", pid)
	req.SetPathValue("key", "result")
	rr := httptest.NewRecorder()
	env.s.handleDeleteProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("response.status = %q, want deleted", resp["status"])
	}
	if resp["key"] != "result" {
		t.Errorf("response.key = %q, want result", resp["key"])
	}

	// Key must no longer appear in GET.
	getRR := httptest.NewRecorder()
	env.s.handleGetProjectFindings(getRR, projectFindingsReq(t, pid))
	m := decodeMapResponse(t, getRR)
	if _, exists := m["result"]; exists {
		t.Error("GET still returns deleted key 'result'")
	}
}

// TestHandleDeleteProjectFinding_NotFound_Returns404 verifies DELETE on a missing key
// returns 404 and emits no WS event.
func TestHandleDeleteProjectFinding_NotFound_Returns404(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-del-miss"
	seedProjectForFindings(t, env.s, pid)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+pid+"/findings/ghost", nil)
	req.SetPathValue("id", pid)
	req.SetPathValue("key", "ghost")
	rr := httptest.NewRecorder()
	env.s.handleDeleteProjectFinding(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("DELETE status = %d, want 404", rr.Code)
	}

	// No hub.Broadcast was called; event count must remain zero.
	env.rec.mu.Lock()
	count := len(env.rec.events)
	env.rec.mu.Unlock()
	if count != 0 {
		t.Errorf("event count after 404 delete = %d, want 0", count)
	}
}

// TestHandleDeleteProjectFinding_BroadcastsWSEvent verifies DELETE emits one
// project_findings.updated event with action "delete" and the correct key list.
func TestHandleDeleteProjectFinding_BroadcastsWSEvent(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-del-ws"
	addProjectFinding(t, env.s, pid, "delkey", "delval")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+pid+"/findings/delkey", nil)
	req.SetPathValue("id", pid)
	req.SetPathValue("key", "delkey")
	rr := httptest.NewRecorder()
	env.s.handleDeleteProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200", rr.Code)
	}

	ev := env.rec.waitEvent(t, ws.EventProjectFindingsUpdated)
	if ev.Data["action"] != "delete" {
		t.Errorf("event.action = %v, want delete", ev.Data["action"])
	}
	deleted, ok := ev.Data["deleted"].([]string)
	if !ok || len(deleted) != 1 || deleted[0] != "delkey" {
		t.Errorf("event.deleted = %v, want [delkey]", ev.Data["deleted"])
	}
	if ev.ProjectID != pid {
		t.Errorf("event.project_id = %q, want %q", ev.ProjectID, pid)
	}
}

// TestHandleDeleteProjectFinding_MissingProjectID_Returns400 verifies that DELETE
// without a project ID path value returns HTTP 400.
func TestHandleDeleteProjectFinding_MissingProjectID_Returns400(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects//findings/somekey", nil)
	req.SetPathValue("key", "somekey")
	// PathValue("id") returns "" when not set.
	rr := httptest.NewRecorder()
	env.s.handleDeleteProjectFinding(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project ID")
}

// TestHandleDeleteProjectFinding_URLEncodedKey verifies that URL-encoded keys
// (e.g. colons encoded as %3A) are correctly decoded before lookup.
func TestHandleDeleteProjectFinding_URLEncodedKey(t *testing.T) {
	env := newProjectFindingsWSEnv(t)
	const pid = "proj-del-url"
	addProjectFinding(t, env.s, pid, "ns:key", "value")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+pid+"/findings/ns%3Akey", nil)
	req.SetPathValue("id", pid)
	req.SetPathValue("key", "ns%3Akey") // raw (not yet decoded) path value
	rr := httptest.NewRecorder()
	env.s.handleDeleteProjectFinding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE URL-encoded key status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["key"] != "ns:key" {
		t.Errorf("response.key = %q, want ns:key", resp["key"])
	}
}
