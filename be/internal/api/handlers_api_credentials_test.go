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

func newAPICredServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "apicred_test.db")
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

func decodeAPICred(t *testing.T, rr *httptest.ResponseRecorder) *model.APICredential {
	t.Helper()
	var got model.APICredential
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return &got
}

func decodeAPICredList(t *testing.T, rr *httptest.ResponseRecorder) []*model.APICredential {
	t.Helper()
	var got []*model.APICredential
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return got
}

func postAPICred(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-credentials", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleCreateAPICredential(rr, req)
	return rr
}

func getAPICred(t *testing.T, s *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	s.handleGetAPICredential(rr, req)
	return rr
}

func putAPICred(t *testing.T, s *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/api-credentials/"+id, strings.NewReader(body))
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	s.handleUpdateAPICredential(rr, req)
	return rr
}

func deleteAPICred(t *testing.T, s *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/api-credentials/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	s.handleDeleteAPICredential(rr, req)
	return rr
}

// TestHandleCreateAPICredential_ValidPrefixes accepts env:, file:, literal:.
func TestHandleCreateAPICredential_ValidPrefixes(t *testing.T) {
	cases := []struct {
		id  string
		ref string
	}{
		{"c-env", "env:ANTHROPIC_API_KEY"},
		{"c-file", "file:/etc/nrflo/secret"},
		{"c-lit", "literal:sk-test-abc"},
	}
	s := newAPICredServer(t)
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			body := `{"id":"` + tc.id + `","provider":"anthropic","secret_ref":"` + tc.ref + `"}`
			rr := postAPICred(t, s, body)
			if rr.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestHandleCreateAPICredential_InvalidPrefix returns 400.
func TestHandleCreateAPICredential_InvalidPrefix(t *testing.T) {
	s := newAPICredServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing id", `{"provider":"anthropic","secret_ref":"env:X"}`},
		{"missing provider", `{"id":"c1","secret_ref":"env:X"}`},
		{"missing secret_ref", `{"id":"c1","provider":"anthropic"}`},
		{"unknown prefix", `{"id":"c1","provider":"anthropic","secret_ref":"raw:abc"}`},
		{"no prefix", `{"id":"c1","provider":"anthropic","secret_ref":"sk-no-prefix"}`},
		{"malformed json", `{not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := postAPICred(t, s, tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("body=%s -> status = %d, want 400", tc.body, rr.Code)
			}
		})
	}
}

// TestHandleAPICredentials_LiteralRedactedOnList ensures list never returns plaintext literal:.
func TestHandleAPICredentials_LiteralRedactedOnList(t *testing.T) {
	s := newAPICredServer(t)
	if rr := postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"literal:sk-test-supersecret"}`); rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rr.Code)
	}
	if rr := postAPICred(t, s, `{"id":"c2","provider":"anthropic","secret_ref":"env:ANTHROPIC_API_KEY"}`); rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-credentials", nil)
	rr := httptest.NewRecorder()
	s.handleListAPICredentials(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	bodyText := rr.Body.String()
	if strings.Contains(bodyText, "sk-test-supersecret") {
		t.Errorf("list body must not contain plaintext literal value; got: %s", bodyText)
	}
	if !strings.Contains(bodyText, "literal:***") {
		t.Errorf("list body must redact as literal:***; got: %s", bodyText)
	}
	// env: refs are kept as-is (they're a reference, not the secret).
	if !strings.Contains(bodyText, "env:ANTHROPIC_API_KEY") {
		t.Errorf("list body should preserve env: references; got: %s", bodyText)
	}

	list := decodeAPICredList(t, rr)
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

// TestHandleAPICredentials_LiteralRedactedOnGet ensures GET hides plaintext.
func TestHandleAPICredentials_LiteralRedactedOnGet(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"literal:sk-very-secret"}`)

	rr := getAPICred(t, s, "c1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "sk-very-secret") {
		t.Errorf("GET body must not contain plaintext literal value; got: %s", rr.Body.String())
	}
	got := decodeAPICred(t, rr)
	if got.SecretRef != "literal:***" {
		t.Errorf("SecretRef = %q, want literal:***", got.SecretRef)
	}
}

// TestHandleAPICredentials_PutRoundTrip accepts plaintext literal on PUT but
// hides it on subsequent GET.
func TestHandleAPICredentials_PutRoundTrip(t *testing.T) {
	s := newAPICredServer(t)
	if rr := postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"literal:sk-old"}`); rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rr.Code)
	}

	rr := putAPICred(t, s, "c1", `{"secret_ref":"literal:sk-new-value"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-new-value") {
		t.Errorf("PUT response must not echo plaintext literal value; got: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "literal:***") {
		t.Errorf("PUT response must redact as literal:***; got: %s", rr.Body.String())
	}

	rr2 := getAPICred(t, s, "c1")
	got := decodeAPICred(t, rr2)
	if got.SecretRef != "literal:***" {
		t.Errorf("after PUT, GET SecretRef = %q, want literal:***", got.SecretRef)
	}
}

// TestHandleAPICredentials_PutRejectsRedactedPlaceholder prevents writing back
// the redacted token.
func TestHandleAPICredentials_PutRejectsRedactedPlaceholder(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"literal:sk-real"}`)

	rr := putAPICred(t, s, "c1", `{"secret_ref":"literal:***"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for redacted placeholder", rr.Code)
	}
}

// TestHandleAPICredentials_PutEmptySecretRef returns 400.
func TestHandleAPICredentials_PutEmptySecretRef(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:X"}`)

	rr := putAPICred(t, s, "c1", `{"secret_ref":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty secret_ref", rr.Code)
	}
}

// TestHandleAPICredentials_PutInvalidPrefix returns 400.
func TestHandleAPICredentials_PutInvalidPrefix(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:X"}`)

	rr := putAPICred(t, s, "c1", `{"secret_ref":"raw:abc"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown prefix", rr.Code)
	}
}

// TestHandleAPICredentials_PutBadBody returns 400 on malformed JSON.
func TestHandleAPICredentials_PutBadBody(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:X"}`)

	rr := putAPICred(t, s, "c1", `{not json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleAPICredentials_PutNotFound returns 404.
func TestHandleAPICredentials_PutNotFound(t *testing.T) {
	s := newAPICredServer(t)
	rr := putAPICred(t, s, "missing", `{"secret_ref":"env:X"}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleAPICredentials_DuplicateID returns 409.
func TestHandleAPICredentials_DuplicateID(t *testing.T) {
	s := newAPICredServer(t)
	if rr := postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:X"}`); rr.Code != http.StatusCreated {
		t.Fatalf("first create status = %d", rr.Code)
	}
	rr := postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:Y"}`)
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for duplicate id", rr.Code)
	}
}

// TestHandleAPICredentials_GetNotFound returns 404.
func TestHandleAPICredentials_GetNotFound(t *testing.T) {
	s := newAPICredServer(t)
	rr := getAPICred(t, s, "missing")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// TestHandleAPICredentials_Delete deletes and verifies 404 after.
func TestHandleAPICredentials_Delete(t *testing.T) {
	s := newAPICredServer(t)
	postAPICred(t, s, `{"id":"c1","provider":"anthropic","secret_ref":"env:X"}`)

	rr := deleteAPICred(t, s, "c1")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	rr2 := deleteAPICred(t, s, "c1")
	if rr2.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", rr2.Code)
	}
}
