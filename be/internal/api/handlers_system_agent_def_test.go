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
	"be/internal/model"
)

// newSystemAgentServer creates a minimal Server for system agent definition handler tests.
func newSystemAgentServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sys_agent_handler_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// decodeSystemAgentDef decodes a SystemAgentDefinition from the response recorder.
func decodeSystemAgentDef(t *testing.T, rr *httptest.ResponseRecorder) *model.SystemAgentDefinition {
	t.Helper()
	var def model.SystemAgentDefinition
	if err := json.NewDecoder(rr.Body).Decode(&def); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &def
}

// decodeSystemAgentDefList decodes a list response.
func decodeSystemAgentDefList(t *testing.T, rr *httptest.ResponseRecorder) []*model.SystemAgentDefinition {
	t.Helper()
	var defs []*model.SystemAgentDefinition
	if err := json.NewDecoder(rr.Body).Decode(&defs); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return defs
}

// --- List ---

func TestHandleListSystemAgentDefs_Empty(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentDefs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	defs := decodeSystemAgentDefList(t, rr)
	if len(defs) != 0 {
		t.Errorf("len = %d, want 0", len(defs))
	}
}

func TestHandleListSystemAgentDefs_WithEntries(t *testing.T) {
	s := newSystemAgentServer(t)

	for _, id := range []string{"agent-z", "agent-a"} {
		body := `{"id":"` + id + `","prompt":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
		rr := httptest.NewRecorder()
		s.handleCreateSystemAgentDef(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %q status = %d, want 201", id, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	rr := httptest.NewRecorder()
	s.handleListSystemAgentDefs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	defs := decodeSystemAgentDefList(t, rr)
	if len(defs) != 2 {
		t.Fatalf("len = %d, want 2", len(defs))
	}
	// Ordered by id ascending.
	if defs[0].ID != "agent-a" {
		t.Errorf("defs[0].ID = %q, want %q", defs[0].ID, "agent-a")
	}
	if defs[1].ID != "agent-z" {
		t.Errorf("defs[1].ID = %q, want %q", defs[1].ID, "agent-z")
	}
}

// --- Create ---

func TestHandleCreateSystemAgentDef_Valid(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"id":"conflict-resolver","prompt":"resolve it","model":"opus","timeout":45}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	def := decodeSystemAgentDef(t, rr)
	if def.ID != "conflict-resolver" {
		t.Errorf("ID = %q, want %q", def.ID, "conflict-resolver")
	}
	if def.Model != "opus" {
		t.Errorf("Model = %q, want %q", def.Model, "opus")
	}
	if def.Timeout != 45 {
		t.Errorf("Timeout = %d, want 45", def.Timeout)
	}
}

func TestHandleCreateSystemAgentDef_DefaultsApplied(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"id":"defaults-agent","prompt":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
	def := decodeSystemAgentDef(t, rr)
	if def.Model != "sonnet" {
		t.Errorf("default Model = %q, want %q", def.Model, "sonnet")
	}
	if def.Timeout != 20 {
		t.Errorf("default Timeout = %d, want 20", def.Timeout)
	}
}

func TestHandleCreateSystemAgentDef_MissingID(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"prompt":"do something"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "id")
}

func TestHandleCreateSystemAgentDef_InvalidJSON(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateSystemAgentDef_Duplicate(t *testing.T) {
	s := newSystemAgentServer(t)
	body := `{"id":"dup-agent","prompt":"p"}`

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr1 := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr2 := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Errorf("duplicate create status = %d, want 409", rr2.Code)
	}
	assertErrorContains(t, rr2, "already exists")
}

// --- Get ---

func TestHandleGetSystemAgentDef_Valid(t *testing.T) {
	s := newSystemAgentServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"my-agent","prompt":"hello","model":"haiku"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/my-agent", nil)
	req.SetPathValue("id", "my-agent")
	rr := httptest.NewRecorder()
	s.handleGetSystemAgentDef(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	def := decodeSystemAgentDef(t, rr)
	if def.ID != "my-agent" {
		t.Errorf("ID = %q, want %q", def.ID, "my-agent")
	}
	if def.Model != "haiku" {
		t.Errorf("Model = %q, want %q", def.Model, "haiku")
	}
}

func TestHandleGetSystemAgentDef_NotFound(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/no-such", nil)
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleGetSystemAgentDef(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

// --- Update ---

func TestHandleUpdateSystemAgentDef_Valid(t *testing.T) {
	s := newSystemAgentServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"upd-agent","prompt":"p","model":"haiku"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/upd-agent",
		strings.NewReader(`{"model":"opus"}`))
	req.SetPathValue("id", "upd-agent")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Verify update persisted.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/upd-agent", nil)
	getReq.SetPathValue("id", "upd-agent")
	getRR := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR, getReq)
	def := decodeSystemAgentDef(t, getRR)
	if def.Model != "opus" {
		t.Errorf("after update Model = %q, want %q", def.Model, "opus")
	}
	// Prompt unchanged.
	if def.Prompt != "p" {
		t.Errorf("after update Prompt = %q, want %q", def.Prompt, "p")
	}
}

func TestHandleUpdateSystemAgentDef_NotFound(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/no-such",
		strings.NewReader(`{"model":"opus"}`))
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

func TestHandleUpdateSystemAgentDef_InvalidJSON(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/x",
		strings.NewReader("bad json"))
	req.SetPathValue("id", "x")
	rr := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Delete ---

func TestHandleDeleteSystemAgentDef_Valid(t *testing.T) {
	s := newSystemAgentServer(t)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"del-agent","prompt":"p"}`))
	s.handleCreateSystemAgentDef(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/system-agents/del-agent", nil)
	req.SetPathValue("id", "del-agent")
	rr := httptest.NewRecorder()
	s.handleDeleteSystemAgentDef(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Subsequent GET returns 404.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/del-agent", nil)
	getReq.SetPathValue("id", "del-agent")
	getRR := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR, getReq)
	if getRR.Code != http.StatusNotFound {
		t.Errorf("post-delete GET status = %d, want 404", getRR.Code)
	}
}

func TestHandleDeleteSystemAgentDef_NotFound(t *testing.T) {
	s := newSystemAgentServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/system-agents/no-such", nil)
	req.SetPathValue("id", "no-such")
	rr := httptest.NewRecorder()
	s.handleDeleteSystemAgentDef(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	assertErrorContains(t, rr, "not found")
}

// --- Full CRUD flow ---

func TestHandleSystemAgentDef_FullCRUDFlow(t *testing.T) {
	s := newSystemAgentServer(t)

	// 1. List — empty.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	listRR := httptest.NewRecorder()
	s.handleListSystemAgentDefs(listRR, listReq)
	defs := decodeSystemAgentDefList(t, listRR)
	if len(defs) != 0 {
		t.Fatalf("initial list len = %d, want 0", len(defs))
	}

	// 2. Create.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents",
		strings.NewReader(`{"id":"conflict-resolver","prompt":"fix it"}`))
	createRR := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", createRR.Code, createRR.Body.String())
	}

	// 3. Get.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/conflict-resolver", nil)
	getReq.SetPathValue("id", "conflict-resolver")
	getRR := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRR.Code)
	}
	def := decodeSystemAgentDef(t, getRR)
	if def.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", def.Model, "sonnet")
	}

	// 4. List — one entry.
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	listRR2 := httptest.NewRecorder()
	s.handleListSystemAgentDefs(listRR2, listReq2)
	defs2 := decodeSystemAgentDefList(t, listRR2)
	if len(defs2) != 1 {
		t.Fatalf("list after create len = %d, want 1", len(defs2))
	}

	// 5. Update.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/system-agents/conflict-resolver",
		strings.NewReader(`{"model":"opus"}`))
	patchReq.SetPathValue("id", "conflict-resolver")
	patchRR := httptest.NewRecorder()
	s.handleUpdateSystemAgentDef(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", patchRR.Code)
	}

	// 6. Get — model updated, prompt unchanged.
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/conflict-resolver", nil)
	getReq2.SetPathValue("id", "conflict-resolver")
	getRR2 := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR2, getReq2)
	def2 := decodeSystemAgentDef(t, getRR2)
	if def2.Model != "opus" {
		t.Errorf("after update Model = %q, want %q", def2.Model, "opus")
	}
	if def2.Prompt != "fix it" {
		t.Errorf("after update Prompt = %q, want %q", def2.Prompt, "fix it")
	}

	// 7. Delete.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/system-agents/conflict-resolver", nil)
	delReq.SetPathValue("id", "conflict-resolver")
	delRR := httptest.NewRecorder()
	s.handleDeleteSystemAgentDef(delRR, delReq)
	if delRR.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", delRR.Code)
	}

	// 8. Get after delete — 404.
	getReq3 := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents/conflict-resolver", nil)
	getReq3.SetPathValue("id", "conflict-resolver")
	getRR3 := httptest.NewRecorder()
	s.handleGetSystemAgentDef(getRR3, getReq3)
	if getRR3.Code != http.StatusNotFound {
		t.Errorf("post-delete get status = %d, want 404", getRR3.Code)
	}

	// 9. List — empty again.
	listReq3 := httptest.NewRequest(http.MethodGet, "/api/v1/system-agents", nil)
	listRR3 := httptest.NewRecorder()
	s.handleListSystemAgentDefs(listRR3, listReq3)
	defs3 := decodeSystemAgentDefList(t, listRR3)
	if len(defs3) != 0 {
		t.Errorf("post-delete list len = %d, want 0", len(defs3))
	}
}
