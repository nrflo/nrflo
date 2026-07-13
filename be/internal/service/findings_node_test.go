package service

import (
	"testing"

	"be/internal/db"
	"be/internal/types"
)

// insertNodeSession inserts a session with an explicit node_id distinct from
// its phase/agent_type, so a single agent_type can fan out to multiple nodes.
func insertNodeSession(t *testing.T, pool *db.Pool, id, wfiID, nodeID, agentType, status, result, createdAt string) {
	t.Helper()
	insertSession(t, pool, id, wfiID, agentType, status, result, createdAt)
	if _, err := pool.Exec(`UPDATE agent_sessions SET node_id = ? WHERE id = ?`, nodeID, id); err != nil {
		t.Fatalf("insertNodeSession(%s): set node_id: %v", id, err)
	}
}

// TestFindingsGetByNode_HappyPath verifies a node-keyed read returns the
// node's own findings.
func TestFindingsGetByNode_HappyPath(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-node-happy", wfiID, "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-node-happy", `{"result":"ok","score":7}`)

	result, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(node_id=analyzer): %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["result"] != "ok" || m["score"] != float64(7) {
		t.Errorf("m = %v, want {result:ok, score:7}", m)
	}
}

// TestFindingsGetByNode_KeyExtraction verifies single-key extraction returns
// the bare value and multi-key extraction returns a sub-map.
func TestFindingsGetByNode_KeyExtraction(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-node-keys", wfiID, "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-node-keys", `{"a":"1","b":"2","c":"3"}`)

	single, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", InstanceID: wfiID, Key: "a"})
	if err != nil {
		t.Fatalf("Get(node_id, key=a): %v", err)
	}
	if single != "1" {
		t.Errorf("single-key result = %v, want \"1\"", single)
	}

	multi, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", InstanceID: wfiID, Keys: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("Get(node_id, keys=[a,b]): %v", err)
	}
	m, ok := multi.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", multi)
	}
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("multi-key result = %v, want {a:1, b:2}", m)
	}
	if _, exists := m["c"]; exists {
		t.Errorf("multi-key result should not include unrequested key c, got %v", m)
	}
}

// TestFindingsGetByNode_NodeIDAndAgentType_Rejected verifies NodeID combined
// with AgentType is rejected as mutually exclusive.
func TestFindingsGetByNode_NodeIDAndAgentType_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupFindingsLayerEnv(t)

	_, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", AgentType: "analyzer", InstanceID: wfiID})
	if err == nil {
		t.Fatal("expected error when both NodeID and AgentType are set, got nil")
	}
}

// TestFindingsGetByNode_NodeIDAndLayer_Rejected verifies NodeID combined with
// Layer is rejected as mutually exclusive.
func TestFindingsGetByNode_NodeIDAndLayer_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupFindingsLayerEnv(t)

	layer := 0
	_, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", Layer: &layer, InstanceID: wfiID})
	if err == nil {
		t.Fatal("expected error when both NodeID and Layer are set, got nil")
	}
}

// TestFindingsGetByNode_MissingInstanceID_Rejected verifies a missing
// instance_id returns an error rather than panicking.
func TestFindingsGetByNode_MissingInstanceID_Rejected(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupFindingsLayerEnv(t)

	_, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", InstanceID: ""})
	if err == nil {
		t.Fatal("expected error for missing instance_id, got nil")
	}
}

// TestFindingsGetByNode_UnknownNode_ReturnsError verifies a node_id with no
// backing session returns an error (distinct from a known-but-empty node).
func TestFindingsGetByNode_UnknownNode_ReturnsError(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupFindingsLayerEnv(t)

	_, err := svc.Get(&types.FindingsGetRequest{NodeID: "ghost-node", InstanceID: wfiID})
	if err == nil {
		t.Fatal("expected error for unknown node_id, got nil")
	}
}

// TestFindingsGetByNode_KnownNodeNoFindings_ReturnsEmptyMap verifies a known
// node with a session but no findings yet returns an empty map and nil error.
func TestFindingsGetByNode_KnownNodeNoFindings_ReturnsEmptyMap(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-node-empty", wfiID, "analyzer", "completed", "pass", "")

	result, err := svc.Get(&types.FindingsGetRequest{NodeID: "analyzer", InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(node_id=analyzer, no findings): unexpected error: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

// TestFindingsGetByNode_FanOutSiblingsDisjoint verifies two sessions sharing
// one agent_type but distinct node_ids resolve to disjoint per-node findings.
func TestFindingsGetByNode_FanOutSiblingsDisjoint(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertNodeSession(t, pool, "sess-worker-1", wfiID, "worker#1", "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-worker-1", `{"picked":"g2"}`)

	insertNodeSession(t, pool, "sess-worker-2", wfiID, "worker#2", "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-worker-2", `{"picked":"reddit"}`)

	r1, err := svc.Get(&types.FindingsGetRequest{NodeID: "worker#1", InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(node_id=worker#1): %v", err)
	}
	m1 := r1.(map[string]interface{})
	if m1["picked"] != "g2" {
		t.Errorf("worker#1 picked = %v, want g2", m1["picked"])
	}

	r2, err := svc.Get(&types.FindingsGetRequest{NodeID: "worker#2", InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(node_id=worker#2): %v", err)
	}
	m2 := r2.(map[string]interface{})
	if m2["picked"] != "reddit" {
		t.Errorf("worker#2 picked = %v, want reddit", m2["picked"])
	}
}
