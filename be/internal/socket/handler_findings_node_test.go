package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// TestFindingsGet_NodeIDAndAgentType_MutuallyExclusive verifies node_id
// combined with agent_type returns INVALID_PARAMS.
func TestFindingsGet_NodeIDAndAgentType_MutuallyExclusive(t *testing.T) {
	env := newHandlerTestEnv(t)

	params, _ := json.Marshal(map[string]interface{}{
		"node_id":     "analyzer",
		"agent_type":  "analyzer",
		"instance_id": "some-instance",
	})
	req := Request{
		ID:      "req-node-agenttype-conflict",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected INVALID_PARAMS error when both node_id and agent_type are set, got success")
	}
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("error code = %d, want %d (INVALID_PARAMS)", resp.Error.Code, ErrCodeInvalidParams)
	}
}

// TestFindingsGet_NodeIDAndLayer_MutuallyExclusive verifies node_id combined
// with layer returns INVALID_PARAMS.
func TestFindingsGet_NodeIDAndLayer_MutuallyExclusive(t *testing.T) {
	env := newHandlerTestEnv(t)

	layer := 0
	params, _ := json.Marshal(map[string]interface{}{
		"node_id":     "analyzer",
		"layer":       layer,
		"instance_id": "some-instance",
	})
	req := Request{
		ID:      "req-node-layer-conflict",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected INVALID_PARAMS error when both node_id and layer are set, got success")
	}
	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("error code = %d, want %d (INVALID_PARAMS)", resp.Error.Code, ErrCodeInvalidParams)
	}
}

// TestFindingsGet_NodeOnly_HappyPath verifies findings.get with node_id only
// returns that node's own findings.
func TestFindingsGet_NodeOnly_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES ('analyzer', ?, 'test', '', 0, ?, ?)`,
		env.project, now, now); err != nil {
		t.Fatalf("insert agent_def: %v", err)
	}
	env.createTicketAndWorkflow(t, "NODE-1")

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "NODE-1", "test").Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}

	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, result, pid, context_left, restart_count, created_at, updated_at)
		VALUES ('sess-node', ?, 'NODE-1', ?, 'analyzer', 'analyzer', 'analyzer',
			'completed', 'pass', NULL, NULL, 0, ?, ?)`,
		env.project, wfiID, now, now); err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}
	fr := repo.NewFindingRepo(env.pool, clock.Real())
	if err := fr.Upsert("session", "sess-node", "node_key", json.RawMessage(`"node_val"`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert node finding: %v", err)
	}

	params, _ := json.Marshal(map[string]interface{}{
		"node_id":     "analyzer",
		"instance_id": wfiID,
	})
	req := Request{
		ID:      "req-node-ok",
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
	if result["node_key"] != "node_val" {
		t.Errorf("result[node_key] = %v, want \"node_val\"", result["node_key"])
	}
}

// TestFindingsGet_NodeOnly_UnknownNode verifies an unknown node_id surfaces
// as a not-found-style error (not a panic, not a silent empty map).
func TestFindingsGet_NodeOnly_UnknownNode(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "NODE-2")

	var wfiID string
	if err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
		env.project, "NODE-2", "test").Scan(&wfiID); err != nil {
		t.Fatalf("get wfi: %v", err)
	}

	params, _ := json.Marshal(map[string]interface{}{
		"node_id":     "ghost-node",
		"instance_id": wfiID,
	})
	req := Request{
		ID:      "req-node-unknown",
		Method:  "findings.get",
		Project: env.project,
		Params:  params,
	}
	resp := env.handler.Handle(req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown node_id, got success")
	}
}
