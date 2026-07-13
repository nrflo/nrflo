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

// newAPIModelsServer creates a minimal Server for API model handler tests.
func newAPIModelsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api_models_handler_test.db")
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

// newAPIModelsServerWithHub creates a Server with a running WS hub for broadcast tests.
func newAPIModelsServerWithHub(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api_models_hub_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})
	return &Server{pool: pool, clock: clock.Real(), wsHub: hub}
}

func decodeAPIModel(t *testing.T, rr *httptest.ResponseRecorder) *model.APIModel {
	t.Helper()
	var m model.APIModel
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode APIModel response: %v", err)
	}
	return &m
}

func decodeAPIModelList(t *testing.T, rr *httptest.ResponseRecorder) []*model.APIModel {
	t.Helper()
	var models []*model.APIModel
	if err := json.NewDecoder(rr.Body).Decode(&models); err != nil {
		t.Fatalf("decode APIModel list: %v", err)
	}
	return models
}

// --- List ---

func TestHandleListAPIModels(t *testing.T) {
	s := newAPIModelsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-models", nil)
	rr := httptest.NewRecorder()
	s.handleListAPIModels(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	models := decodeAPIModelList(t, rr)
	if len(models) != 20 {
		t.Errorf("len = %d, want 20 (8 anthropic + 12 openai seeded rows)", len(models))
	}
	// Verify mix of providers
	var anthropicCount, openaiCount int
	for _, m := range models {
		switch m.Provider {
		case "anthropic":
			anthropicCount++
		case "openai":
			openaiCount++
		}
	}
	if anthropicCount != 8 {
		t.Errorf("anthropic rows = %d, want 8", anthropicCount)
	}
	if openaiCount != 12 {
		t.Errorf("openai rows = %d, want 12", openaiCount)
	}
}

// --- Get ---

func TestHandleGetAPIModel(t *testing.T) {
	s := newAPIModelsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-models/opus_4_7", nil)
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleGetAPIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeAPIModel(t, rr)
	if m.ID != "opus_4_7" {
		t.Errorf("ID = %q, want opus_4_7", m.ID)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
}

func TestHandleGetAPIModel_NotFound(t *testing.T) {
	s := newAPIModelsServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-models/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	s.handleGetAPIModel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// --- Create ---

func TestHandleCreateAPIModel_InvalidProvider(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"id":"bad-prov","provider":"azure","display_name":"Bad","mapped_model":"gpt-4"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid provider")
}

func TestHandleCreateAPIModel_MissingID(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"provider":"anthropic","display_name":"No ID","mapped_model":"claude-sonnet"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "required")
}

func TestHandleCreateAPIModel_InvalidReasoningEffort(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"id":"bad-effort","provider":"anthropic","display_name":"Bad","mapped_model":"claude-opus-4-7","reasoning_effort":"nonsense"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "must be one of low, medium, high, xhigh, max")
}

func TestHandleCreateAPIModel_XhighOnNonOpus47(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"id":"xhigh-sonnet","provider":"anthropic","display_name":"Bad","mapped_model":"claude-sonnet-4-5","reasoning_effort":"xhigh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "only supported on Anthropic Opus 4.7")
}

func TestHandleCreateAPIModel_Valid_WithWSBroadcast(t *testing.T) {
	s := newAPIModelsServerWithHub(t)

	client, ch := ws.NewTestClient(s.wsHub, "create-api-model-client")
	s.wsHub.Register(client)
	// Subscribe to global scope (empty projectID) to receive api_model events.
	s.wsHub.Subscribe(client, "", "")
	defer s.wsHub.Unregister(client)

	body := `{"id":"new-api-model","provider":"openai","display_name":"New Model","mapped_model":"gpt-4"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPIModel(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeAPIModel(t, rr)
	if m.ID != "new-api-model" {
		t.Errorf("ID = %q, want new-api-model", m.ID)
	}
	if m.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", m.Provider)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var evt ws.Event
			if err := json.Unmarshal(msg, &evt); err != nil {
				continue
			}
			if evt.Type == ws.EventAPIModelCreated {
				return
			}
		case <-deadline:
			t.Errorf("timeout waiting for WS event %q", ws.EventAPIModelCreated)
			return
		}
	}
}

func TestHandleCreateAPIModel_Duplicate(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"id":"dup-handler","provider":"anthropic","display_name":"Dup","mapped_model":"claude-sonnet"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr1 := httptest.NewRecorder()
	s.handleCreateAPIModel(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(body))
	rr2 := httptest.NewRecorder()
	s.handleCreateAPIModel(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Errorf("duplicate create status = %d, want 409", rr2.Code)
	}
}

// --- Update ---

func TestHandleUpdateAPIModel_ReadOnly_LockedFields_Rejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "display_name", body: `{"display_name":"Foo"}`},
		{name: "mapped_model", body: `{"mapped_model":"gpt-5"}`},
		{name: "context_length", body: `{"context_length":100000}`},
		{name: "enabled_false", body: `{"enabled":false}`},
	}

	const wantMsg = "only reasoning_effort can be updated on built-in models"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newAPIModelsServer(t)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/api-models/opus_4_7", strings.NewReader(tc.body))
			req.SetPathValue("id", "opus_4_7")
			rr := httptest.NewRecorder()
			s.handleUpdateAPIModel(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, wantMsg)
		})
	}
}

func TestHandleUpdateAPIModel_ReadOnly_ReasoningEffort_Succeeds(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"reasoning_effort":"high"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/api-models/opus_4_7", strings.NewReader(body))
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleUpdateAPIModel(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeAPIModel(t, rr)
	if m.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want high", m.ReasoningEffort)
	}
	if !m.ReadOnly {
		t.Error("ReadOnly = false after reasoning_effort update, want true")
	}
}

func TestHandleUpdateAPIModel_NotFound(t *testing.T) {
	s := newAPIModelsServer(t)
	body := `{"reasoning_effort":"high"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/api-models/nonexistent", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	s.handleUpdateAPIModel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// --- Delete ---

func TestHandleDeleteAPIModel_ReadOnly(t *testing.T) {
	s := newAPIModelsServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-models/opus_4_7", nil)
	req.SetPathValue("id", "opus_4_7")
	rr := httptest.NewRecorder()
	s.handleDeleteAPIModel(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "cannot delete system model")
}

func TestHandleDeleteAPIModel_UserCreated_Succeeds(t *testing.T) {
	s := newAPIModelsServer(t)

	createBody := `{"id":"del-me","provider":"openai","display_name":"Del Me","mapped_model":"gpt-4"}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/api-models", strings.NewReader(createBody))
	rrC := httptest.NewRecorder()
	s.handleCreateAPIModel(rrC, reqC)
	if rrC.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d, want 201; body: %s", rrC.Code, rrC.Body.String())
	}

	reqD := httptest.NewRequest(http.MethodDelete, "/api/v1/api-models/del-me", nil)
	reqD.SetPathValue("id", "del-me")
	rrD := httptest.NewRecorder()
	s.handleDeleteAPIModel(rrD, reqD)

	if rrD.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rrD.Code, rrD.Body.String())
	}
}

func TestHandleDeleteAPIModel_NotFound(t *testing.T) {
	s := newAPIModelsServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-models/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()
	s.handleDeleteAPIModel(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}
