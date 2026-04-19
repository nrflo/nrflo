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

// newCLIModelsServer creates a minimal Server for CLI model CRUD handler tests.
func newCLIModelsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cli_models_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeCLIModel(t *testing.T, rr *httptest.ResponseRecorder) *model.CLIModel {
	t.Helper()
	var m model.CLIModel
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode CLIModel response: %v", err)
	}
	return &m
}

// --- Create: reasoning_effort validation ---

func TestHandleCreateCLIModel_InvalidReasoningEffort(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"id":"bad-effort","cli_type":"claude","display_name":"Bad","mapped_model":"claude-opus-4-7","reasoning_effort":"nonsense"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateCLIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "must be one of low, medium, high, xhigh, max")
}

func TestHandleCreateCLIModel_XhighOnNonOpus47Claude(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"id":"xhigh-sonnet","cli_type":"claude","display_name":"Bad","mapped_model":"claude-sonnet-4-5","reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateCLIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "only supported on Opus 4.7")
}

func TestHandleCreateCLIModel_XhighOnOpus47_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"id":"my-opus47","cli_type":"claude","display_name":"My Opus","mapped_model":"claude-opus-4-7","reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateCLIModel(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, "xhigh")
	}
}

func TestHandleCreateCLIModel_XhighOnOpencode_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)
	// For non-claude cli_types, xhigh must not hit the opus-4.7 gate.
	body := `{"id":"my-opencode","cli_type":"opencode","display_name":"OC","mapped_model":"openai/gpt-5.4","reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateCLIModel(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Update: reasoning_effort validation ---

func TestHandleUpdateCLIModel_InvalidReasoningEffort(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":"nonsense"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/sonnet", strings.NewReader(body))
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "must be one of low, medium, high, xhigh, max")
}

func TestHandleUpdateCLIModel_XhighOnNonOpus47(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/sonnet", strings.NewReader(body))
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "only supported on Opus 4.7")
}

func TestHandleUpdateCLIModel_XhighOnOpus47_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(body))
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, "xhigh")
	}
}

func TestHandleUpdateCLIModel_HighOnAnyClaude_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)
	// "high" is valid for any model regardless of mapped_model.
	body := `{"reasoning_effort":"high"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/sonnet", strings.NewReader(body))
	req.SetPathValue("id", "sonnet")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, "high")
	}
}

func TestHandleUpdateCLIModel_ClearEffort_Succeeds(t *testing.T) {
	// Empty string is a valid value (inherits CLI default).
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":""}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(body))
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort = %q, want empty", m.ReasoningEffort)
	}
}

func TestHandleUpdateCLIModel_MappedModelChange_RejectsIncompatibleStoredXhigh(t *testing.T) {
	// Pre-condition: create a user-owned opus_4_7 row (read_only rows now block mapped_model edits)
	// store xhigh on it, then PATCH to change mapped_model only — overlay logic must catch
	// the now-invalid combination.
	s := newCLIModelsServer(t)
	createBody := `{"id":"user-opus","cli_type":"claude","display_name":"User Opus","mapped_model":"claude-opus-4-7"}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(createBody))
	rrC := httptest.NewRecorder()
	s.handleCreateCLIModel(rrC, reqC)
	if rrC.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d, want 201; body: %s", rrC.Code, rrC.Body.String())
	}

	setXhigh := `{"reasoning_effort":"xhigh"}`
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/user-opus", strings.NewReader(setXhigh))
	req1.SetPathValue("id", "user-opus")
	rr1 := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("setup PATCH status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// Now attempt to flip the model without clearing effort.
	changeModel := `{"mapped_model":"claude-sonnet-4-5"}`
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/user-opus", strings.NewReader(changeModel))
	req2.SetPathValue("id", "user-opus")
	rr2 := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr2.Code, rr2.Body.String())
	}
	assertErrorContains(t, rr2, "only supported on Opus 4.7")
}

// --- Update: read_only guard (only reasoning_effort editable on built-in rows) ---

func TestHandleUpdateCLIModel_ReadOnly_LockedFields_Rejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "display_name", body: `{"display_name":"Foo"}`},
		{name: "mapped_model", body: `{"mapped_model":"claude-opus-4-7"}`},
		{name: "context_length", body: `{"context_length":100000}`},
		{name: "enabled_false", body: `{"enabled":false}`},
		{name: "enabled_true", body: `{"enabled":true}`},
	}

	const wantMsg = "only reasoning_effort can be updated on built-in models"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newCLIModelsServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(tc.body))
			req.SetPathValue("id", "opus_4_7")
			rr := httptest.NewRecorder()
			s.handleUpdateCLIModel(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, wantMsg)
		})
	}
}

func TestHandleUpdateCLIModel_ReadOnly_ReasoningEffort_High_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":"high"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(body))
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, "high")
	}
	if !m.ReadOnly {
		t.Error("ReadOnly = false after reasoning_effort update, want true")
	}

	// Persisted via subsequent GET.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/cli-models/opus_4_7", nil)
	getReq.SetPathValue("id", "opus_4_7")
	getRR := httptest.NewRecorder()
	s.handleGetCLIModel(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRR.Code)
	}
	got := decodeCLIModel(t, getRR)
	if got.ReasoningEffort != "high" {
		t.Errorf("persisted ReasoningEffort = %q, want %q", got.ReasoningEffort, "high")
	}
}

// Regression: user-owned rows still accept any field.
func TestHandleUpdateCLIModel_UserRow_DisplayName_Succeeds(t *testing.T) {
	s := newCLIModelsServer(t)

	createBody := `{"id":"user-row","cli_type":"claude","display_name":"Orig","mapped_model":"claude-sonnet-4-5"}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/cli-models", strings.NewReader(createBody))
	rrC := httptest.NewRecorder()
	s.handleCreateCLIModel(rrC, reqC)
	if rrC.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d, want 201; body: %s", rrC.Code, rrC.Body.String())
	}

	body := `{"display_name":"Foo"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/user-row", strings.NewReader(body))
	req.SetPathValue("id", "user-row")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeCLIModel(t, rr)
	if m.DisplayName != "Foo" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "Foo")
	}
}

func TestHandleUpdateCLIModel_NotFound(t *testing.T) {
	s := newCLIModelsServer(t)
	body := `{"reasoning_effort":"high"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/nonexistent", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}
