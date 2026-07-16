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

func newModelsServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "models_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

func decodeModel(t *testing.T, rr *httptest.ResponseRecorder) *model.Model {
	t.Helper()
	var m model.Model
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode Model response: %v", err)
	}
	return &m
}

func modelRequest(method, path, id, body string) *http.Request {
	reader := strings.NewReader(body)
	req := httptest.NewRequest(method, path, reader)
	if id != "" {
		req.SetPathValue("id", id)
	}
	return req
}

func TestHandleModelsListAndGetUnifiedRows(t *testing.T) {
	s := newModelsServer(t)
	rr := httptest.NewRecorder()
	s.handleListModels(rr, httptest.NewRequest(http.MethodGet, "/api/v1/models", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var models []*model.Model
	if err := json.NewDecoder(rr.Body).Decode(&models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 16 {
		t.Fatalf("model count = %d, want 16", len(models))
	}

	rr = httptest.NewRecorder()
	s.handleGetModel(rr, modelRequest(http.MethodGet, "/api/v1/models/opus-4-7", "opus-4-7", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeModel(t, rr)
	if m.CLIModel != "claude-opus-4-7" || m.APIModel != "claude-opus-4-7" || m.APIContext != 1000000 {
		t.Fatalf("unexpected unified row: %+v", m)
	}
}

func TestHandleCreateModelUnifiedModes(t *testing.T) {
	s := newModelsServer(t)
	body := `{"id":"CUSTOM","provider":"anthropic","display_name":"Custom","cli_model":"custom-cli","api_model":"custom-api","cli_efforts":["high","low"],"api_efforts":["medium"],"cli_context":123,"api_context":456,"default_effort":""}`
	rr := httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeModel(t, rr)
	if m.ID != "custom" || m.CLIModel != "custom-cli" || m.APIModel != "custom-api" {
		t.Fatalf("unexpected model: %+v", m)
	}
	if len(m.CLIEfforts) != 2 || m.CLIEfforts[0] != "low" || m.APIContext != 456 {
		t.Fatalf("unexpected mode metadata: %+v", m)
	}
}

func TestHandleCreateModelValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing mode", `{"id":"none","provider":"openai","display_name":"None"}`, "cli_model or api_model"},
		{"provider", `{"id":"bad","provider":"azure","display_name":"Bad","cli_model":"bad"}`, "invalid provider"},
		{"effort", `{"id":"bad","provider":"openai","display_name":"Bad","cli_model":"bad","cli_efforts":["warp"]}`, "invalid supported_efforts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newModelsServer(t)
			rr := httptest.NewRecorder()
			s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", tc.body))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, tc.want)
		})
	}
}

func TestHandleUpdateAndDeleteModel(t *testing.T) {
	s := newModelsServer(t)
	create := `{"id":"editable","provider":"openai","display_name":"Before","cli_model":"custom"}`
	rr := httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", create))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleUpdateModel(rr, modelRequest(http.MethodPatch, "/api/v1/models/editable", "editable", `{"display_name":"After","api_model":"custom-api"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", rr.Code, rr.Body.String())
	}
	m := decodeModel(t, rr)
	if m.DisplayName != "After" || m.APIModel != "custom-api" {
		t.Fatalf("unexpected update: %+v", m)
	}

	rr = httptest.NewRecorder()
	s.handleDeleteModel(rr, modelRequest(http.MethodDelete, "/api/v1/models/editable", "editable", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleModelReadOnlyAndNotFoundErrors(t *testing.T) {
	s := newModelsServer(t)
	rr := httptest.NewRecorder()
	s.handleUpdateModel(rr, modelRequest(http.MethodPatch, "/api/v1/models/sonnet-5", "sonnet-5", `{"display_name":"Changed"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("readonly update status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleDeleteModel(rr, modelRequest(http.MethodDelete, "/api/v1/models/sonnet-5", "sonnet-5", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("readonly delete status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleGetModel(rr, modelRequest(http.MethodGet, "/api/v1/models/missing", "missing", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing get status = %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateModelBroadcastsUnifiedEvent(t *testing.T) {
	s := newModelsServer(t)
	hub := ws.NewHub(clock.Real())
	s.wsHub = hub
	go hub.Run()
	t.Cleanup(hub.Stop)
	client, ch := ws.NewTestClient(hub, "model-events")
	hub.Register(client)
	hub.Subscribe(client, "", "")
	defer hub.Unregister(client)

	rr := httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", `{"id":"event-model","provider":"openai","display_name":"Event","cli_model":"event"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", rr.Code, rr.Body.String())
	}
	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != ws.EventModelCreated || event.Data["model_id"] != "event-model" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for model.created")
	}
}
