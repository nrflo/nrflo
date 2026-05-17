package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spec_import"
	"be/internal/ws"
)

// --- stub adapter ---

type stubSpecImportAdapter struct {
	src          spec_import.Source
	fetchResult  spec_import.FetchedSpec
	fetchErr     error
	ghResults    []spec_import.GitHubIssueSummary
	ghErr        error
	jiraResults  []spec_import.JiraIssueSummary
	jiraErr      error
	capturedEnv  map[string]string
}

func (s *stubSpecImportAdapter) Source() spec_import.Source { return s.src }
func (s *stubSpecImportAdapter) Fetch(_ context.Context, _ spec_import.Input) (spec_import.FetchedSpec, error) {
	return s.fetchResult, s.fetchErr
}
func (s *stubSpecImportAdapter) Search(_ context.Context, q, _ string, env map[string]string) ([]spec_import.GitHubIssueSummary, error) {
	s.capturedEnv = env
	return s.ghResults, s.ghErr
}

type stubJiraAdapter struct {
	stubSpecImportAdapter
	capturedEnv map[string]string
}

func (s *stubJiraAdapter) Search(_ context.Context, _ string, env map[string]string) ([]spec_import.JiraIssueSummary, error) {
	s.capturedEnv = env
	return s.jiraResults, s.jiraErr
}

// --- test server ---

func newSpecImportServer(t *testing.T) (*Server, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "spec_import_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(func() {
		hub.Stop()
		pool.Close()
	})

	projectID := "proj-spec-import"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, 'TestSpec', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	// Seed __spec_import__ workflow so workflow_instance FK is satisfied.
	if _, err := pool.Exec(`
		INSERT OR IGNORE INTO workflows
			(id, project_id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, created_at, updated_at)
		VALUES ('__spec_import__', ?, 'Spec import (internal)', 'project', '[]', 0, '', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed __spec_import__ workflow: %v", err)
	}

	s := &Server{pool: pool, clock: clock.Real(), wsHub: hub}
	return s, projectID
}

// injectProject injects the projectID into the request context so getProjectID(r) returns it.
func injectProject(req *http.Request, projectID string) *http.Request {
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	return req.WithContext(ctx)
}

// --- handleStartSpecImport ---

func TestHandleStartSpecImport_Markdown_HappyPath(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src:         spec_import.SourceMarkdown,
		fetchResult: spec_import.FetchedSpec{RawText: "# Hello\n\nSpec body."},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	body := `{"source":"markdown","body":"# Hello\n\nSpec body."}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(body))
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleStartSpecImport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["instance_id"] == "" {
		t.Error("instance_id should be non-empty")
	}
	if resp["status"] != "ready" {
		t.Errorf("status = %v, want ready", resp["status"])
	}

	// Verify findings were seeded in the findings table.
	instanceID, _ := resp["instance_id"].(string)
	var specSourceVal string
	err := s.pool.QueryRow(
		`SELECT value FROM findings WHERE scope = 'workflow_instance' AND scope_id = ? AND key = '_spec_source'`,
		instanceID,
	).Scan(&specSourceVal)
	if err != nil {
		t.Fatalf("query _spec_source finding: %v", err)
	}
	var specSource string
	json.Unmarshal([]byte(specSourceVal), &specSource)
	if specSource != "markdown" {
		t.Errorf("_spec_source = %v, want markdown", specSource)
	}
	var rawSpecVal string
	if err := s.pool.QueryRow(
		`SELECT value FROM findings WHERE scope = 'workflow_instance' AND scope_id = ? AND key = 'raw_spec'`,
		instanceID,
	).Scan(&rawSpecVal); err != nil {
		t.Fatalf("query raw_spec finding: %v", err)
	}
	if rawSpecVal == "" || rawSpecVal == `""` {
		t.Error("raw_spec should be non-empty")
	}
}

func TestHandleStartSpecImport_MissingProjectID_400(t *testing.T) {
	s, _ := newSpecImportServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(`{"source":"markdown","body":"x"}`))
	// no project in context
	rr := httptest.NewRecorder()
	s.handleStartSpecImport(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleStartSpecImport_MissingEnv_412(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src:      spec_import.SourceGitHubIssue,
		fetchErr: spec_import.MissingEnvError{Source: spec_import.SourceGitHubIssue, Missing: []string{"GITHUB_TOKEN"}},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	body := `{"source":"github_issue","body":"https://github.com/owner/repo/issues/1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(body))
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleStartSpecImport(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "missing_env" {
		t.Errorf("error = %v, want missing_env", resp["error"])
	}
	missing, _ := resp["missing"].([]interface{})
	if len(missing) == 0 {
		t.Error("missing array should be non-empty")
	}
}

func TestHandleStartSpecImport_AdapterFailure_502_and_ErrorLog(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src:      spec_import.SourceGitHubIssue,
		fetchErr: spec_import.ErrAdapterNotFound,
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	body := `{"source":"github_issue","body":"https://github.com/owner/repo/issues/999"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(body))
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleStartSpecImport(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "adapter_failure" {
		t.Errorf("error = %v, want adapter_failure", resp["error"])
	}

	// Verify an error row was inserted.
	var count int
	s.pool.QueryRow(`SELECT COUNT(*) FROM errors WHERE project_id = ?`, projectID).Scan(&count)
	if count == 0 {
		t.Error("expected at least one error row in DB")
	}
}

// --- handleGetSpecImport ---

func TestHandleGetSpecImport_CompletedWithFindings(t *testing.T) {
	s, projectID := newSpecImportServer(t)

	// Start a successful import first.
	stub := &stubSpecImportAdapter{
		src:         spec_import.SourceMarkdown,
		fetchResult: spec_import.FetchedSpec{RawText: "hello spec"},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(`{"source":"markdown","body":"hello spec"}`))
	startReq = injectProject(startReq, projectID)
	startRR := httptest.NewRecorder()
	s.handleStartSpecImport(startRR, startReq)
	if startRR.Code != http.StatusOK {
		t.Fatalf("start: %d: %s", startRR.Code, startRR.Body.String())
	}
	var startResp map[string]interface{}
	json.NewDecoder(startRR.Body).Decode(&startResp)
	instanceID := startResp["instance_id"].(string)

	// Now GET the import session.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/import/spec/"+instanceID, nil)
	getReq.SetPathValue("instance_id", instanceID)
	getRR := httptest.NewRecorder()
	s.handleGetSpecImport(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("get: %d: %s", getRR.Code, getRR.Body.String())
	}
	var getResp map[string]interface{}
	json.NewDecoder(getRR.Body).Decode(&getResp)
	if getResp["status"] != "ready" {
		t.Errorf("status = %v, want ready", getResp["status"])
	}
	if getResp["raw_spec"] == "" {
		t.Error("raw_spec should be non-empty")
	}
	if getResp["source"] != "markdown" {
		t.Errorf("source = %v, want markdown", getResp["source"])
	}
	if getResp["title"] == nil || getResp["title"] == "" {
		t.Errorf("title should be derived from raw_spec, got %v", getResp["title"])
	}
}

func TestHandleGetSpecImport_NotFound_404(t *testing.T) {
	s, _ := newSpecImportServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/spec/nonexistent", nil)
	req.SetPathValue("instance_id", "nonexistent")
	rr := httptest.NewRecorder()
	s.handleGetSpecImport(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// --- handleEnvVarCatalog ---

func TestHandleEnvVarCatalog_ReturnsAllVars(t *testing.T) {
	s, _ := newSpecImportServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/env-var-catalog", nil)
	rr := httptest.NewRecorder()
	s.handleEnvVarCatalog(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	vars, _ := resp["vars"].([]interface{})
	if len(vars) != 6 {
		t.Errorf("len(vars) = %d, want 6", len(vars))
	}

	// Verify the expected names appear.
	want := map[string]bool{
		"GITHUB_TOKEN":         true,
		"JIRA_BASE_URL":        true,
		"JIRA_EMAIL":           true,
		"JIRA_API_TOKEN":       true,
		"ANTHROPIC_API_KEY":    true,
		"ANTHROPIC_OAUTH_TOKEN": true,
	}
	for _, v := range vars {
		m, _ := v.(map[string]interface{})
		name, _ := m["name"].(string)
		delete(want, name)
	}
	if len(want) > 0 {
		t.Errorf("missing catalog entries: %v", want)
	}
}

// --- handleGitHubSearch ---

func TestHandleGitHubSearch_MissingEnv_412(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src:   spec_import.SourceGitHubIssue,
		ghErr: spec_import.MissingEnvError{Source: spec_import.SourceGitHubIssue, Missing: []string{"GITHUB_TOKEN"}},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/github/search?q=test", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleGitHubSearch(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "missing_env" {
		t.Errorf("error = %v, want missing_env", resp["error"])
	}
}

func TestHandleGitHubSearch_HappyPath_200(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src: spec_import.SourceGitHubIssue,
		ghResults: []spec_import.GitHubIssueSummary{
			{Number: 1, Title: "Fix bug", HTMLURL: "https://github.com/o/r/issues/1", State: "open"},
		},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/github/search?q=fix+bug", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleGitHubSearch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	results, _ := resp["results"].([]interface{})
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestHandleGitHubSearch_MissingQ_400(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	s.specImportAdapterFunc = func(src string) (interface{}, error) {
		return &stubSpecImportAdapter{src: spec_import.SourceGitHubIssue}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/github/search", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleGitHubSearch(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- handleJiraSearch ---

func TestHandleJiraSearch_MissingEnv_412(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubJiraAdapter{
		stubSpecImportAdapter: stubSpecImportAdapter{
			src:      spec_import.SourceJira,
			jiraErr:  spec_import.MissingEnvError{Source: spec_import.SourceJira, Missing: []string{"JIRA_BASE_URL", "JIRA_EMAIL", "JIRA_API_TOKEN"}},
		},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/jira/search?q=test", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleJiraSearch(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	missing, _ := resp["missing"].([]interface{})
	if len(missing) == 0 {
		t.Error("missing array should be non-empty")
	}
}

func TestHandleJiraSearch_HappyPath_200(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubJiraAdapter{
		stubSpecImportAdapter: stubSpecImportAdapter{
			src: spec_import.SourceJira,
			jiraResults: []spec_import.JiraIssueSummary{
				{Key: "PROJ-1", Summary: "Fix thing", Status: "Open"},
			},
		},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import/jira/search?q=fix", nil)
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleJiraSearch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	results, _ := resp["results"].([]interface{})
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

// --- WS broadcast ---

func TestHandleStartSpecImport_BroadcastsEvent(t *testing.T) {
	s, projectID := newSpecImportServer(t)
	stub := &stubSpecImportAdapter{
		src:         spec_import.SourceMarkdown,
		fetchResult: spec_import.FetchedSpec{RawText: "spec content"},
	}
	s.specImportAdapterFunc = func(src string) (interface{}, error) { return stub, nil }

	client, ch := ws.NewTestClient(s.wsHub, "spec-import-broadcast-client")
	s.wsHub.Register(client)
	s.wsHub.Subscribe(client, projectID, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/spec", strings.NewReader(`{"source":"markdown","body":"spec content"}`))
	req = injectProject(req, projectID)
	rr := httptest.NewRecorder()
	s.handleStartSpecImport(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start: %d: %s", rr.Code, rr.Body.String())
	}

	waitForEnvVarWSEvent(t, ch, ws.EventSpecImportStarted)
}
