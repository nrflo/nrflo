package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

func decodeModelTestResult(t *testing.T, rr *httptest.ResponseRecorder) modelTestResult {
	t.Helper()
	var result modelTestResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHandleTestModelRoutesByProvider(t *testing.T) {
	s := newModelsServer(t)
	var gotType, gotModel, gotEffort string
	s.cliAdapterFunc = func(cliType, mappedModel, effort string) (*exec.Cmd, bool) {
		gotType, gotModel, gotEffort = cliType, mappedModel, effort
		return exec.Command("echo", "ok"), false
	}
	for _, tc := range []struct {
		id, cliType, mapped, effort string
	}{
		{"sonnet-5", "claude", "claude-sonnet-5", ""},
		{"gpt-5.6-sol", "codex", "gpt-5.6-sol", "low"},
	} {
		rr := httptest.NewRecorder()
		s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/"+tc.id+"/test", tc.id, ""))
		if rr.Code != http.StatusOK || !decodeModelTestResult(t, rr).Success {
			t.Fatalf("%s status = %d; body: %s", tc.id, rr.Code, rr.Body.String())
		}
		if gotType != tc.cliType || gotModel != tc.mapped || gotEffort != tc.effort {
			t.Fatalf("%s adapter args = %q %q %q", tc.id, gotType, gotModel, gotEffort)
		}
	}
}

func TestHandleTestModelRejectsAPIMode(t *testing.T) {
	for _, req := range []*http.Request{
		modelRequest(http.MethodPost, "/api/v1/models/sonnet-5/test?mode=api", "sonnet-5", ""),
		modelRequest(http.MethodPost, "/api/v1/models/sonnet-5/test", "sonnet-5", `{"mode":"api"}`),
	} {
		s := newModelsServer(t)
		rr := httptest.NewRecorder()
		s.handleTestModel(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
		}
		assertErrorContains(t, rr, "not supported")
	}
}

func TestHandleTestModelRejectsAPIOnlyRow(t *testing.T) {
	s := newModelsServer(t)
	rr := httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", `{"id":"api-only","provider":"anthropic","display_name":"API only","api_model":"api-only"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("setup status = %d; body: %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/api-only/test", "api-only", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "does not support cli mode")
}

// TestHandleTestModelRejectsOpenRouter_EmptyCLIModel verifies the normal
// creation path: an openrouter row always has an empty cli_model (enforced
// by ModelService), so the pre-existing empty-cli_model guard rejects a test
// request before the provider switch is ever reached.
func TestHandleTestModelRejectsOpenRouter_EmptyCLIModel(t *testing.T) {
	s := newModelsServer(t)
	rr := httptest.NewRecorder()
	s.handleCreateModel(rr, modelRequest(http.MethodPost, "/api/v1/models", "", `{"id":"or-check","provider":"openrouter","display_name":"OR Check","api_model":"openai/gpt-4o"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("setup status = %d; body: %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/or-check/test", "or-check", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "does not support cli mode")
}

// TestHandleTestModelRejectsOpenRouter_ProviderSwitchGuard directly exercises
// the openrouter case in the cliType switch by fabricating a row with a
// non-empty cli_model via raw SQL (bypassing ModelService validation, which
// would never allow this combination in practice) — the defensive guard must
// still 400 with its own message rather than falling through.
func TestHandleTestModelRejectsOpenRouter_ProviderSwitchGuard(t *testing.T) {
	s := newModelsServer(t)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(`INSERT INTO models
		(id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
		 cli_context, api_context, fallback_models, default_effort, read_only, enabled,
		 created_at, updated_at)
		VALUES ('or-fabricated', 'openrouter', 'OR Fabricated', 'openai/gpt-4o', 'openai/gpt-4o',
		 '[]', '[]', 200000, 200000, '', '', 0, 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("seed fabricated row: %v", err)
	}
	rr := httptest.NewRecorder()
	s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/or-fabricated/test", "or-fabricated", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "API-mode only")
}

func TestHandleTestModelNotFoundAndStartFailure(t *testing.T) {
	s := newModelsServer(t)
	rr := httptest.NewRecorder()
	s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/missing/test", "missing", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d; body: %s", rr.Code, rr.Body.String())
	}

	s.cliAdapterFunc = func(_, _, _ string) (*exec.Cmd, bool) {
		return exec.Command("__nrflo_nonexistent_binary__"), false
	}
	rr = httptest.NewRecorder()
	s.handleTestModel(rr, modelRequest(http.MethodPost, "/api/v1/models/sonnet-5/test", "sonnet-5", ""))
	result := decodeModelTestResult(t, rr)
	if rr.Code != http.StatusOK || result.Success || result.Error == "" {
		t.Fatalf("unexpected failure result: status=%d result=%+v", rr.Code, result)
	}
}
