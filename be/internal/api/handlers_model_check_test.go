package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
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
		{"gpt-5.6-sol", "codex", "gpt-5.6-sol", "medium"},
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
