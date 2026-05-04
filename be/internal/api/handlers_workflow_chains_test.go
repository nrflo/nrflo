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
	"be/internal/ws"
)

func newWorkflowChainServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "wfchain_test.db")
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
	return &Server{pool: pool, clock: clock.Real(), wsHub: hub}
}

func seedChainProject(t *testing.T, s *Server, projectID, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seedChainProject(%q): %v", projectID, err)
	}
	if workflowID != "" {
		if _, err := s.pool.Exec(
			`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
			 VALUES (?, ?, '', ?, '[]', 1, ?, ?)`,
			workflowID, projectID, scopeType, now, now,
		); err != nil {
			t.Fatalf("seedWorkflow(%q): %v", workflowID, err)
		}
	}
}

type wfChainResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Steps []struct {
		ID           string `json:"id"`
		Position     int    `json:"position"`
		WorkflowName string `json:"workflow_name"`
		ScopeType    string `json:"scope_type"`
	} `json:"steps"`
}

func decodeChainResp(t *testing.T, rr *httptest.ResponseRecorder) wfChainResp {
	t.Helper()
	var chain wfChainResp
	if err := json.NewDecoder(rr.Body).Decode(&chain); err != nil {
		t.Fatalf("decode chain: %v (body: %s)", err, rr.Body.String())
	}
	return chain
}

// doCreateChain creates a chain via handler with a single project-scope step.
func doCreateChain(t *testing.T, s *Server, projectID, chainID, stepID, workflowID string) wfChainResp {
	t.Helper()
	body := `{"id":"` + chainID + `","name":"Chain ` + chainID + `","steps":[{"id":"` + stepID + `","workflow_name":"` + workflowID + `","scope_type":"project"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-chains", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleCreateWorkflowChain(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("doCreateChain(%q): status=%d body=%s", chainID, rr.Code, rr.Body.String())
	}
	return decodeChainResp(t, rr)
}

func doChainReq(t *testing.T, s *Server, handler func(http.ResponseWriter, *http.Request),
	method, path, projectID, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if projectID != "" {
		ctx := context.WithValue(req.Context(), projectKey, projectID)
		req = req.WithContext(ctx)
	}
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

// -- List --

func TestHandleListWorkflowChains_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleListWorkflowChains, http.MethodGet, "/api/v1/workflow-chains", "", "", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "X-Project")
}

func TestHandleListWorkflowChains_Empty(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-ls", "", "")
	rr := doChainReq(t, s, s.handleListWorkflowChains, http.MethodGet, "/api/v1/workflow-chains", "proj-wc-ls", "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var list []interface{}
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("len = %d, want 0", len(list))
	}
}

func TestHandleListWorkflowChains_ReturnsChains(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-ls2", "wf-ls2", "project")
	doCreateChain(t, s, "proj-wc-ls2", "chain-ls2", "step-ls2", "wf-ls2")
	rr := doChainReq(t, s, s.handleListWorkflowChains, http.MethodGet, "/api/v1/workflow-chains", "proj-wc-ls2", "", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var list []interface{}
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len = %d, want 1", len(list))
	}
}

// -- Create --

func TestHandleCreateWorkflowChain_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"", `{"name":"X","steps":[]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateWorkflowChain_BadJSON(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-bj", "", "")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-bj", "not-json", nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateWorkflowChain_EmptySteps(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-es", "", "")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-es", `{"name":"X","steps":[]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "at least one step")
}

func TestHandleCreateWorkflowChain_Step0NotProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-s0", "wf-s0", "ticket")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-s0", `{"name":"X","steps":[{"workflow_name":"wf-s0","scope_type":"ticket"}]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "scope_type=project")
}

func TestHandleCreateWorkflowChain_UnknownWorkflow(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-uw", "", "")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-uw", `{"name":"X","steps":[{"workflow_name":"no-such-wf","scope_type":"project"}]}`, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "workflow not found")
}

func TestHandleCreateWorkflowChain_RequireHandoffOnProjectStep(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-rh", "wf-rh", "project")
	body := `{"name":"X","steps":[{"workflow_name":"wf-rh","scope_type":"project","require_ticket_handoff":true}]}`
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-rh", body, nil)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "require_ticket_handoff")
}

func TestHandleCreateWorkflowChain_Valid(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-ok", "wf-ok", "project")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-ok", `{"id":"chain-ok","name":"My Chain","steps":[{"workflow_name":"wf-ok","scope_type":"project"}]}`, nil)
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if chain.ID != "chain-ok" {
		t.Errorf("ID = %q, want 'chain-ok'", chain.ID)
	}
	if chain.Name != "My Chain" {
		t.Errorf("Name = %q, want 'My Chain'", chain.Name)
	}
	if len(chain.Steps) != 1 {
		t.Errorf("Steps len = %d, want 1", len(chain.Steps))
	}
	if chain.Steps[0].Position != 0 {
		t.Errorf("Step[0].Position = %d, want 0", chain.Steps[0].Position)
	}
}

func TestHandleCreateWorkflowChain_DuplicateID(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-dup", "wf-dup", "project")
	doCreateChain(t, s, "proj-wc-dup", "chain-dup", "step-dup", "wf-dup")
	rr := doChainReq(t, s, s.handleCreateWorkflowChain, http.MethodPost, "/api/v1/workflow-chains",
		"proj-wc-dup", `{"id":"chain-dup","name":"D2","steps":[{"workflow_name":"wf-dup","scope_type":"project"}]}`, nil)
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

// -- Get --

func TestHandleGetWorkflowChain_MissingProject(t *testing.T) {
	s := newWorkflowChainServer(t)
	rr := doChainReq(t, s, s.handleGetWorkflowChain, http.MethodGet, "/api/v1/workflow-chains/x",
		"", "", map[string]string{"id": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetWorkflowChain_NotFound(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-gf", "", "")
	rr := doChainReq(t, s, s.handleGetWorkflowChain, http.MethodGet, "/api/v1/workflow-chains/no-chain",
		"proj-wc-gf", "", map[string]string{"id": "no-chain"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleGetWorkflowChain_Found(t *testing.T) {
	s := newWorkflowChainServer(t)
	seedChainProject(t, s, "proj-wc-gok", "wf-gok", "project")
	doCreateChain(t, s, "proj-wc-gok", "chain-gok", "step-gok", "wf-gok")
	rr := doChainReq(t, s, s.handleGetWorkflowChain, http.MethodGet, "/api/v1/workflow-chains/chain-gok",
		"proj-wc-gok", "", map[string]string{"id": "chain-gok"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	chain := decodeChainResp(t, rr)
	if chain.ID != "chain-gok" {
		t.Errorf("ID = %q, want 'chain-gok'", chain.ID)
	}
	if len(chain.Steps) != 1 {
		t.Errorf("Steps len = %d, want 1", len(chain.Steps))
	}
}
