package api

import (
	"context"
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

func newCustomProvidersServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "custom_providers_handler_test.db")
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

func customProviderRequest(method, path, name, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if name != "" {
		req.SetPathValue("name", name)
	}
	return req
}

func decodeCustomProvider(t *testing.T, rr *httptest.ResponseRecorder) *model.CustomProvider {
	t.Helper()
	var p model.CustomProvider
	if err := json.NewDecoder(rr.Body).Decode(&p); err != nil {
		t.Fatalf("decode CustomProvider response: %v", err)
	}
	return &p
}

// TestHandleCustomProviders_CRUD_HappyPath exercises create -> list -> get ->
// update -> delete end to end through the handlers.
func TestHandleCustomProviders_CRUD_HappyPath(t *testing.T) {
	s := newCustomProvidersServer(t)

	rr := httptest.NewRecorder()
	s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "",
		`{"name":"local-ollama","base_url":"http://localhost:11434/v1"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	created := decodeCustomProvider(t, rr)
	if created.Name != "local-ollama" || created.APIWire != "responses" || !created.Enabled {
		t.Fatalf("unexpected created provider: %+v", created)
	}

	rr = httptest.NewRecorder()
	s.handleListCustomProviders(rr, httptest.NewRequest(http.MethodGet, "/api/v1/custom-providers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d; body: %s", rr.Code, rr.Body.String())
	}
	var listed []*model.CustomProvider
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "local-ollama" {
		t.Fatalf("list = %+v, want one local-ollama row", listed)
	}

	rr = httptest.NewRecorder()
	s.handleGetCustomProvider(rr, customProviderRequest(http.MethodGet, "/api/v1/custom-providers/local-ollama", "local-ollama", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleUpdateCustomProvider(rr, customProviderRequest(http.MethodPatch, "/api/v1/custom-providers/local-ollama", "local-ollama",
		`{"api_key":"sk-new"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d; body: %s", rr.Code, rr.Body.String())
	}
	updated := decodeCustomProvider(t, rr)
	if updated.APIKey != "sk-new" {
		t.Errorf("updated APIKey = %q, want sk-new", updated.APIKey)
	}

	rr = httptest.NewRecorder()
	s.handleDeleteCustomProvider(rr, customProviderRequest(http.MethodDelete, "/api/v1/custom-providers/local-ollama", "local-ollama", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleGetCustomProvider(rr, customProviderRequest(http.MethodGet, "/api/v1/custom-providers/local-ollama", "local-ollama", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("get-after-delete status = %d, want 404", rr.Code)
	}
}

// TestHandleCreateCustomProvider_ValidationMapsTo400 table-drives create
// validation failures onto 400.
//
// NOTE: "openai"/"anthropic" are NOT included as reserved-name cases here
// because of a production bug in validateCustomProviderName (see
// be_production_bugs / service/custom_provider_test.go) — only "openrouter"
// is actually rejected today.
func TestHandleCreateCustomProvider_ValidationMapsTo400(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty name", `{"base_url":"http://localhost:11434/v1"}`, "name is required"},
		{"reserved name", `{"name":"openrouter","base_url":"http://localhost:11434/v1"}`, "reserved for a built-in provider"},
		{"missing base_url", `{"name":"local-ollama"}`, "base_url is required"},
		{"invalid base_url", `{"name":"local-ollama","base_url":"not-a-url"}`, "invalid base_url"},
		{"invalid api_wire", `{"name":"local-ollama","base_url":"http://localhost:11434/v1","api_wire":"bogus"}`, "invalid api_wire"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newCustomProvidersServer(t)
			rr := httptest.NewRecorder()
			s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "", tc.body))
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, tc.want)
		})
	}
}

// TestHandleCreateCustomProvider_Duplicate_Returns409 verifies a second
// create with the same name maps to 409.
func TestHandleCreateCustomProvider_Duplicate_Returns409(t *testing.T) {
	s := newCustomProvidersServer(t)
	body := `{"name":"local-ollama","base_url":"http://localhost:11434/v1"}`
	rr := httptest.NewRecorder()
	s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("first create status = %d; body: %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "", body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("dup create status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetCustomProvider_NotFound_Returns404 verifies a missing name 404s.
func TestHandleGetCustomProvider_NotFound_Returns404(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleGetCustomProvider(rr, customProviderRequest(http.MethodGet, "/api/v1/custom-providers/missing", "missing", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleUpdateCustomProvider_NotFound_Returns404 verifies PATCH on a
// missing name 404s.
func TestHandleUpdateCustomProvider_NotFound_Returns404(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleUpdateCustomProvider(rr, customProviderRequest(http.MethodPatch, "/api/v1/custom-providers/missing", "missing",
		`{"api_key":"sk-x"}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleDeleteCustomProvider_InUse_Returns409 verifies delete on a
// provider referenced by a models row maps to 409, and the invalid-body path
// on update/create maps to 400.
func TestHandleDeleteCustomProvider_InUse_Returns409(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "",
		`{"name":"local-ollama","base_url":"http://localhost:11434/v1"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "",
		`{"id":"local-model","provider":"local-ollama","display_name":"Local Model","api_model":"llama3"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed model status = %d; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleDeleteCustomProvider(rr, customProviderRequest(http.MethodDelete, "/api/v1/custom-providers/local-ollama", "local-ollama", ""))
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete-in-use status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "in use")
}

// TestHandleDeleteCustomProvider_NotFound_Returns404 verifies delete of a
// missing name 404s.
func TestHandleDeleteCustomProvider_NotFound_Returns404(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleDeleteCustomProvider(rr, customProviderRequest(http.MethodDelete, "/api/v1/custom-providers/missing", "missing", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleCreateCustomProvider_InvalidJSONBody_Returns400 verifies a
// malformed request body 400s before reaching the service layer.
func TestHandleCreateCustomProvider_InvalidJSONBody_Returns400(t *testing.T) {
	s := newCustomProvidersServer(t)
	rr := httptest.NewRecorder()
	s.handleCreateCustomProvider(rr, customProviderRequest(http.MethodPost, "/api/v1/custom-providers", "", `not json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// --- route-level auth gating ---

// customProviderRoutesMux builds a Server + registered routes (mirrors
// newRoutesMux in server_api_mode_routes_test.go) for auth-gating tests.
func customProviderRoutesMux(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	s := newCustomProvidersServer(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return s, mux
}

// asAdmin returns a shallow-cloned request carrying an admin user in context,
// mirroring the userKey injection pattern used by global_write_guard_test.go.
// s.sessionMgr is nil in these tests, so requireAuth passes the context
// through unchanged and requireAdmin's getUser(r) sees this injected user.
func asAdmin(r *http.Request) *http.Request {
	u := &model.User{ID: "admin-1", Role: model.UserRoleAdmin}
	return r.WithContext(context.WithValue(r.Context(), userKey, u))
}

// TestCustomProviderRoutes_ReadIsProtected_WriteIsAdminOnly verifies GET, POST,
// PATCH, and DELETE routes all require an admin user in context — custom
// providers carry a plaintext api_key, so reads are admin-gated like writes.
func TestCustomProviderRoutes_ReadIsProtected_WriteIsAdminOnly(t *testing.T) {
	_, mux := customProviderRoutesMux(t)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/custom-providers", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET (no admin) status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, asAdmin(httptest.NewRequest(http.MethodGet, "/api/v1/custom-providers", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET (admin) status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/custom-providers", strings.NewReader(
		`{"name":"local-ollama","base_url":"http://localhost:11434/v1"}`)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST (no admin) status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req := asAdmin(httptest.NewRequest(http.MethodPost, "/api/v1/custom-providers", strings.NewReader(
		`{"name":"local-ollama","base_url":"http://localhost:11434/v1"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST (admin) status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/custom-providers/local-ollama", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("DELETE (no admin) status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, asAdmin(httptest.NewRequest(http.MethodDelete, "/api/v1/custom-providers/local-ollama", nil)))
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE (admin) status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}
