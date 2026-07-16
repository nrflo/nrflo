package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The handlers used to map every model/effort validation failure to HTTP 500.
// These tests pin the corrected behavior: user-input validation failures in the
// agent-definition and system-agent-definition create/update paths return 400.

// TestHandleCreateAgentDef_APIOnlyModelInCLIMode_400 verifies that creating a
// cli-mode def pointed at an api-only model row (gpt-5.3-codex has an empty
// cli_model in the current catalog) returns 400, not 500.
func TestHandleCreateAgentDef_APIOnlyModelInCLIMode_400(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"cli-apionly","prompt":"p","execution_mode":"cli_interactive","model":"gpt-5.3-codex"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid model")
}

// TestHandleCreateAgentDef_UnknownCLIModel_400 verifies that a cli-mode def with
// an unknown model id returns 400, not 500.
func TestHandleCreateAgentDef_UnknownCLIModel_400(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"cli-unknown","prompt":"p","execution_mode":"cli_interactive","model":"no-such-model"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid model")
}

// TestHandleCreateAgentDef_UnsupportedEffort_400 verifies that an effort the
// model row does not support (gpt-5.4-mini cli efforts omit "ultra") returns 400.
func TestHandleCreateAgentDef_UnsupportedEffort_400(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"cli-badeffort","prompt":"p","execution_mode":"cli_interactive","model":"gpt-5.4-mini","reasoning_effort":"ultra"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "reasoning_effort")
}

// TestHandleUpdateAgentDef_UnknownModel_400 verifies that PATCHing a def's model
// to an unknown id returns 400, not 500.
func TestHandleUpdateAgentDef_UnknownModel_400(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)

	if rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"upd-model","prompt":"p","execution_mode":"cli_interactive"}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup create status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr := patchAgentDefRequest(t, s, pid, wid, "upd-model", `{"model":"no-such-model"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid model")
}

// TestHandleCreateSystemAgentDef_InvalidModel_400 verifies the system-agent-def
// create path maps an unknown model to 400, not 500.
func TestHandleCreateSystemAgentDef_InvalidModel_400(t *testing.T) {
	s := newSystemAgentServer(t)

	body := `{"id":"sys-badmodel","prompt":"p","model":"no-such-model"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system-agents", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateSystemAgentDef(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "invalid model")
}
