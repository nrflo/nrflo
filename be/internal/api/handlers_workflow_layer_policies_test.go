package api

import (
	"context"
	"encoding/json"
	"fmt"
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

func newLayerPolicyServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "layer_policy_test.db")
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

func seedLayerPolicyWorkflow(t *testing.T, s *Server, projectID, workflowID string, agentLayers map[string]int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		 VALUES (?, ?, '', 'project', '[]', 1, ?, ?)`,
		workflowID, projectID, now, now)
	if err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	for agentID, layer := range agentLayers {
		_, err = s.pool.Exec(
			`INSERT OR IGNORE INTO agent_definitions
			 (id, workflow_id, project_id, model, timeout, prompt, layer, execution_mode, created_at, updated_at)
			 VALUES (?, ?, ?, 'sonnet', 300, '', ?, 'cli_interactive', ?, ?)`,
			agentID, workflowID, projectID, layer, now, now)
		if err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
	}
}

// doLayerPolicyRequest sends a request to the layer-policies endpoints and returns the recorder.
func doLayerPolicyRequest(t *testing.T, s *Server, method, projectID, workflowID string, layer *int, body string) *httptest.ResponseRecorder {
	t.Helper()
	var path string
	if layer != nil {
		path = fmt.Sprintf("/api/v1/workflows/%s/layer-policies/%d", workflowID, *layer)
	} else {
		path = fmt.Sprintf("/api/v1/workflows/%s/layer-policies", workflowID)
	}

	bodyReader := strings.NewReader(body)
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Inject project via context (bypasses projectMiddleware in unit tests)
	ctx := context.WithValue(req.Context(), projectKey, projectID)
	req = req.WithContext(ctx)
	req.SetPathValue("wid", workflowID)
	if layer != nil {
		req.SetPathValue("layer", fmt.Sprintf("%d", *layer))
	}

	rr := httptest.NewRecorder()

	switch method {
	case http.MethodGet:
		s.handleListLayerPolicies(rr, req)
	case http.MethodPut:
		s.handleSetLayerPolicy(rr, req)
	case http.MethodDelete:
		s.handleDeleteLayerPolicy(rr, req)
	}

	return rr
}

func layerPtr(n int) *int { return &n }

// TestLayerPolicies_GetEmpty verifies that GET returns an empty object when no policies exist.
func TestLayerPolicies_GetEmpty(t *testing.T) {
	s := newLayerPolicyServer(t)
	seedLayerPolicyWorkflow(t, s, "proj", "wf1", map[string]int{"agent-a": 0, "agent-b": 0})

	rr := doLayerPolicyRequest(t, s, http.MethodGet, "proj", "wf1", nil, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("expected empty policies, got %v", result)
	}
}

// TestLayerPolicies_PutGetDelete tests the happy-path lifecycle.
func TestLayerPolicies_PutGetDelete(t *testing.T) {
	s := newLayerPolicyServer(t)
	seedLayerPolicyWorkflow(t, s, "proj2", "wf2", map[string]int{"a": 1, "b": 1})

	// PUT quorum:2 on layer 1
	rr := doLayerPolicyRequest(t, s, http.MethodPut, "proj2", "wf2", layerPtr(1), `{"pass_policy":"quorum:2"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// GET — should see the policy
	rrGet := doLayerPolicyRequest(t, s, http.MethodGet, "proj2", "wf2", nil, "")
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d", rrGet.Code)
	}
	var policies map[string]interface{}
	json.NewDecoder(rrGet.Body).Decode(&policies)
	if policies["1"] != "quorum:2" {
		t.Errorf("expected layer 1 policy quorum:2, got %v", policies)
	}

	// DELETE layer 1
	rrDel := doLayerPolicyRequest(t, s, http.MethodDelete, "proj2", "wf2", layerPtr(1), "")
	if rrDel.Code != http.StatusOK {
		t.Fatalf("DELETE: expected 200, got %d: %s", rrDel.Code, rrDel.Body.String())
	}

	// GET again — should be empty
	rrGet2 := doLayerPolicyRequest(t, s, http.MethodGet, "proj2", "wf2", nil, "")
	var policies2 map[string]interface{}
	json.NewDecoder(rrGet2.Body).Decode(&policies2)
	if len(policies2) != 0 {
		t.Errorf("expected empty after DELETE, got %v", policies2)
	}
}

// TestLayerPolicies_Put_InvalidPolicy verifies 400 on invalid policy string.
func TestLayerPolicies_Put_InvalidPolicy(t *testing.T) {
	s := newLayerPolicyServer(t)
	seedLayerPolicyWorkflow(t, s, "proj3", "wf3", map[string]int{"a": 0})

	rr := doLayerPolicyRequest(t, s, http.MethodPut, "proj3", "wf3", layerPtr(0), `{"pass_policy":"bogus"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid policy, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestLayerPolicies_Put_QuorumExceedsAgentCount verifies 400 when quorum > agent count.
func TestLayerPolicies_Put_QuorumExceedsAgentCount(t *testing.T) {
	s := newLayerPolicyServer(t)
	// Only 1 agent in layer 0
	seedLayerPolicyWorkflow(t, s, "proj4", "wf4", map[string]int{"only": 0})

	rr := doLayerPolicyRequest(t, s, http.MethodPut, "proj4", "wf4", layerPtr(0), `{"pass_policy":"quorum:5"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when quorum exceeds agents, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestLayerPolicies_WS_BroadcastOnPut verifies that PUT triggers a workflow_def.updated WS event.
func TestLayerPolicies_WS_BroadcastOnPut(t *testing.T) {
	s := newLayerPolicyServer(t)
	seedLayerPolicyWorkflow(t, s, "proj5", "wf5", map[string]int{"a": 0, "b": 0})

	client, ch := ws.NewTestClient(s.wsHub, "lp-ws-client")
	s.wsHub.Subscribe(client, "proj5", "")

	doLayerPolicyRequest(t, s, http.MethodPut, "proj5", "wf5", layerPtr(0), `{"pass_policy":"all"}`)

	if !drainForEvent(ch, ws.EventWorkflowDefUpdated, 500*time.Millisecond) {
		t.Error("expected workflow_def.updated WS event after PUT, got none")
	}
}

// TestLayerPolicies_Put_MissingProject verifies 400 when X-Project header is absent.
func TestLayerPolicies_Put_MissingProject(t *testing.T) {
	s := newLayerPolicyServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/workflows/wf/layer-policies/0",
		strings.NewReader(`{"pass_policy":"any"}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("wid", "wf")
	req.SetPathValue("layer", "0")
	rr := httptest.NewRecorder()
	s.handleSetLayerPolicy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when project missing, got %d", rr.Code)
	}
}
