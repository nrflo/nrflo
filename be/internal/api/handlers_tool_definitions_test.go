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

func newToolDefServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tooldef_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeToolDef(t *testing.T, rr *httptest.ResponseRecorder) *model.ToolDefinition {
	t.Helper()
	var got model.ToolDefinition
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return &got
}

func decodeToolDefList(t *testing.T, rr *httptest.ResponseRecorder) []*model.ToolDefinition {
	t.Helper()
	var got []*model.ToolDefinition
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return got
}

func postToolDef(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tool-definitions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateToolDefinition(rr, req)
	return rr
}

// TestHandleListToolDefinitions_Empty starts empty.
func TestHandleListToolDefinitions_Empty(t *testing.T) {
	s := newToolDefServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions", nil)
	rr := httptest.NewRecorder()
	s.handleListToolDefinitions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	list := decodeToolDefList(t, rr)
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

// TestHandleCreateToolDefinition_Success creates a tool definition.
func TestHandleCreateToolDefinition_Success(t *testing.T) {
	s := newToolDefServer(t)
	body := `{"id":"t1","name":"echo","description":"echoes","input_schema":{"type":"object"},"endpoint":"http://x/echo"}`
	rr := postToolDef(t, s, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	got := decodeToolDef(t, rr)
	if got.ID != "t1" {
		t.Errorf("ID = %q, want t1", got.ID)
	}
	if got.AuthMethod != "none" {
		t.Errorf("AuthMethod = %q, want none (default)", got.AuthMethod)
	}
}

// TestHandleCreateToolDefinition_ValidationFailures covers required-field checks
// and JSON validation.
func TestHandleCreateToolDefinition_ValidationFailures(t *testing.T) {
	s := newToolDefServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing id", `{"name":"x","input_schema":{},"endpoint":"http://x"}`},
		{"missing name", `{"id":"t1","input_schema":{},"endpoint":"http://x"}`},
		{"missing endpoint", `{"id":"t1","name":"x","input_schema":{}}`},
		{"missing input_schema", `{"id":"t1","name":"x","endpoint":"http://x"}`},
		{"malformed json", `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := postToolDef(t, s, tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("body=%s -> status = %d, want 400", tc.body, rr.Code)
			}
		})
	}
}

// TestHandleCreateToolDefinition_DuplicateName returns 409.
func TestHandleCreateToolDefinition_DuplicateName(t *testing.T) {
	s := newToolDefServer(t)
	body := `{"id":"t1","name":"echo","description":"","input_schema":{"type":"object"},"endpoint":"http://x/echo"}`
	if rr := postToolDef(t, s, body); rr.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", rr.Code)
	}
	body2 := `{"id":"t2","name":"echo","description":"","input_schema":{"type":"object"},"endpoint":"http://x/echo2"}`
	rr := postToolDef(t, s, body2)
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for duplicate name", rr.Code)
	}
}

// TestHandleGetToolDefinition_NotFound returns 404.
func TestHandleGetToolDefinition_NotFound(t *testing.T) {
	s := newToolDefServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions/missing", nil)
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()
	s.handleGetToolDefinition(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleUpdateToolDefinition_RoundTrip updates and verifies.
func TestHandleUpdateToolDefinition_RoundTrip(t *testing.T) {
	s := newToolDefServer(t)
	body := `{"id":"t1","name":"echo","description":"old","input_schema":{"type":"object"},"endpoint":"http://x/echo"}`
	if rr := postToolDef(t, s, body); rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rr.Code)
	}
	patch := `{"description":"new","input_schema":{"type":"object","required":["x"]}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tool-definitions/t1", strings.NewReader(patch))
	req.SetPathValue("id", "t1")
	rr := httptest.NewRecorder()
	s.handleUpdateToolDefinition(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	got := decodeToolDef(t, rr)
	if got.Description != "new" {
		t.Errorf("Description = %q, want new", got.Description)
	}
	if !strings.Contains(string(got.InputSchema), `"required"`) {
		t.Errorf("InputSchema = %q, want contains 'required'", string(got.InputSchema))
	}
}

// TestHandleUpdateToolDefinition_BadBody returns 400 on malformed body.
func TestHandleUpdateToolDefinition_BadBody(t *testing.T) {
	s := newToolDefServer(t)
	body := `{"id":"t1","name":"echo","description":"","input_schema":{},"endpoint":"http://x"}`
	postToolDef(t, s, body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tool-definitions/t1", strings.NewReader(`{not json`))
	req.SetPathValue("id", "t1")
	rr := httptest.NewRecorder()
	s.handleUpdateToolDefinition(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleUpdateToolDefinition_NotFound returns 404.
func TestHandleUpdateToolDefinition_NotFound(t *testing.T) {
	s := newToolDefServer(t)
	patch := `{"description":"x"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tool-definitions/nope", strings.NewReader(patch))
	req.SetPathValue("id", "nope")
	rr := httptest.NewRecorder()
	s.handleUpdateToolDefinition(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleDeleteToolDefinition deletes and verifies 404 after.
func TestHandleDeleteToolDefinition(t *testing.T) {
	s := newToolDefServer(t)
	postToolDef(t, s, `{"id":"t1","name":"echo","description":"","input_schema":{},"endpoint":"http://x"}`)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tool-definitions/t1", nil)
	req.SetPathValue("id", "t1")
	rr := httptest.NewRecorder()
	s.handleDeleteToolDefinition(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	// Subsequent delete returns 404
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/tool-definitions/t1", nil)
	req2.SetPathValue("id", "t1")
	rr2 := httptest.NewRecorder()
	s.handleDeleteToolDefinition(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", rr2.Code)
	}
}

// TestHandleListToolDefinitions_FilterByProject filters via ?project_id.
func TestHandleListToolDefinitions_FilterByProject(t *testing.T) {
	s := newToolDefServer(t)
	postToolDef(t, s, `{"id":"ta","name":"alpha","description":"","input_schema":{},"endpoint":"http://x/a","project_id":"p1"}`)
	postToolDef(t, s, `{"id":"tb","name":"beta","description":"","input_schema":{},"endpoint":"http://x/b","project_id":"p2"}`)
	postToolDef(t, s, `{"id":"tc","name":"gamma","description":"","input_schema":{},"endpoint":"http://x/c"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions?project_id=p1", nil)
	rr := httptest.NewRecorder()
	s.handleListToolDefinitions(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := decodeToolDefList(t, rr)
	names := map[string]bool{}
	for _, x := range got {
		names[x.Name] = true
	}
	if !names["alpha"] || !names["gamma"] {
		t.Errorf("filter p1 names = %v, want alpha+gamma", names)
	}
	if names["beta"] {
		t.Errorf("filter p1 should not include beta")
	}
}

// TestHandleListToolDefinitions_FilterByWorkflow filters via ?workflow_id.
func TestHandleListToolDefinitions_FilterByWorkflow(t *testing.T) {
	s := newToolDefServer(t)
	postToolDef(t, s, `{"id":"ta","name":"alpha","description":"","input_schema":{},"endpoint":"http://x","workflow_id":"wf-a"}`)
	postToolDef(t, s, `{"id":"tb","name":"beta","description":"","input_schema":{},"endpoint":"http://x","workflow_id":"wf-b"}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tool-definitions?workflow_id=wf-a", nil)
	rr := httptest.NewRecorder()
	s.handleListToolDefinitions(rr, req)
	got := decodeToolDefList(t, rr)
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("filter workflow_id=wf-a got len=%d names=%v, want [alpha]", len(got), got)
	}
}
