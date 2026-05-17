package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// TestFindingsGet_AgentTypeAndLayerMutuallyExclusive verifies that providing both
// agent_type and layer in a findings.get request returns INVALID_PARAMS (-32602).
func TestFindingsGet_AgentTypeAndLayerMutuallyExclusive(t *testing.T) {
	env := newHandlerTestEnv(t)

	layer := 0
	params, _ := json.Marshal(map[string]interface{}{
		"agent_type":  "analyzer",
		"layer":       layer,
		"instance_id": "some-instance",
	})
	req := Request{
		ID:      "req-layer-conflict",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected INVALID_PARAMS error when both agent_type and layer are set, got success")
	}
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("error code = %d, want %d (INVALID_PARAMS)", resp.Error.Code, ErrCodeInvalidParams)
	}
}

// TestFindingsGet_LayerOnly_HappyPath verifies findings.get with layer-only (no agent_type)
// returns a map keyed by agent_type for all agents in that layer.
func TestFindingsGet_LayerOnly_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)

	// Seed an agent_definition at layer 0 for the "test" workflow.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES ('analyzer', ?, 'test', '', 0, ?, ?)`,
		env.project, now, now); err != nil {
		t.Fatalf("insert agent_def: %v", err)
	}

	// Create a ticket + workflow instance.
	env.createTicketAndWorkflow(t, "LAYER-1")

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "LAYER-1", "test").Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}

	// Insert a completed session with findings.
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, pid, context_left, restart_count, created_at, updated_at)
		VALUES ('sess-layer', ?, 'LAYER-1', ?, 'analyzer', 'analyzer',
			'completed', 'pass', NULL, NULL, 0, ?, ?)`,
		env.project, wfiID, now, now); err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}
	// Seed the finding via FindingRepo.
	fr := repo.NewFindingRepo(env.pool, clock.Real())
	if err := fr.Upsert("session", "sess-layer", "layer_key", json.RawMessage(`"layer_val"`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert layer finding: %v", err)
	}

	layer := 0
	params, _ := json.Marshal(map[string]interface{}{
		"layer":       layer,
		"instance_id": wfiID,
	})
	req := Request{
		ID:      "req-layer-ok",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, ok := result["analyzer"]; !ok {
		t.Errorf("expected 'analyzer' key in layer result, got: %v", result)
	}
}

// TestFindingsGet_LayerOnly_MissingInstanceID verifies that a layer request with
// no instance_id returns an internal/not-found error (not a panic).
func TestFindingsGet_LayerOnly_MissingInstanceID(t *testing.T) {
	env := newHandlerTestEnv(t)

	layer := 0
	params, _ := json.Marshal(map[string]interface{}{
		"layer":       layer,
		"instance_id": "",
	})
	req := Request{
		ID:      "req-layer-no-instance",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for missing instance_id, got success")
	}
	// Expect internal error (instance_id required).
	if resp.Error.Code != ErrCodeInternal && resp.Error.Code != ErrCodeNotFound {
		t.Errorf("error code = %d, want %d (internal) or %d (not found)",
			resp.Error.Code, ErrCodeInternal, ErrCodeNotFound)
	}
}

// TestFindingsGet_BothNilLayer_OwnSessionRead verifies that omitting layer entirely
// (nil pointer in Go) still works as an own-session read (no regression).
func TestFindingsGet_BothNilLayer_OwnSessionRead(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "OWN-1")

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "OWN-1", "test").Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := "sess-own-read"
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, pid, context_left, restart_count, created_at, updated_at)
		VALUES (?, ?, 'OWN-1', ?, 'analyzer', 'analyzer',
			'running', NULL, NULL, NULL, 0, ?, ?)`,
		sessionID, env.project, wfiID, now, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	// Seed the finding via FindingRepo.
	frOwn := repo.NewFindingRepo(env.pool, clock.Real())
	if err := frOwn.Upsert("session", sessionID, "my_key", json.RawMessage(`"my_val"`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert own finding: %v", err)
	}

	// No layer, no agent_type → own-session read via session_id.
	params, _ := json.Marshal(types.FindingsGetRequest{
		SessionID: sessionID,
	})
	req := Request{
		ID:      "req-own-read",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error for own-session read: %v", resp.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["my_key"] != "my_val" {
		t.Errorf("own-session read: my_key = %v, want \"my_val\"", result["my_key"])
	}
}

// TestFindingsGet_LayerAndAgentType_ServiceGuardAlsoApplies verifies that the service-level
// guard (not just the socket guard) also returns an error for this combination.
func TestFindingsGet_LayerAndAgentType_ServiceGuardAlsoApplies(t *testing.T) {
	env := newHandlerTestEnv(t)

	svc := service.NewFindingsService(env.pool, clock.Real())
	layer := 0
	_, err := svc.Get(&types.FindingsGetRequest{
		AgentType:  "analyzer",
		Layer:      &layer,
		InstanceID: "any-instance",
	})
	if err == nil {
		t.Fatal("expected error from service when both AgentType and Layer set, got nil")
	}
}
