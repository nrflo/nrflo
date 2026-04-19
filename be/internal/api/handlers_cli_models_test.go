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
	// Pre-condition: store xhigh on opus_4_7, then PATCH to change mapped_model
	// only — overlay logic must catch the now-invalid combination.
	s := newCLIModelsServer(t)
	setXhigh := `{"reasoning_effort":"xhigh"}`
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(setXhigh))
	req1.SetPathValue("id", "opus_4_7")
	rr1 := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("setup PATCH status = %d, want 200; body: %s", rr1.Code, rr1.Body.String())
	}

	// Now attempt to flip the model without clearing effort.
	changeModel := `{"mapped_model":"claude-sonnet-4-5"}`
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/cli-models/opus_4_7", strings.NewReader(changeModel))
	req2.SetPathValue("id", "opus_4_7")
	rr2 := httptest.NewRecorder()
	s.handleUpdateCLIModel(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr2.Code, rr2.Body.String())
	}
	assertErrorContains(t, rr2, "only supported on Opus 4.7")
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
