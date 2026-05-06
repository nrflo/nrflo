package service

import (
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// insertFindingsAgentDef adds an agent_definition at a given layer under test-proj/test-wf.
func insertFindingsAgentDef(t *testing.T, pool *db.Pool, id string, layer int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, created_at, updated_at)
		 VALUES (?, 'test-proj', 'test-wf', '', ?, ?, ?)`,
		id, layer, now, now)
	if err != nil {
		t.Fatalf("insertFindingsAgentDef(%s, layer=%d): %v", id, layer, err)
	}
}

// setSessionFindings updates the findings JSON on a session row.
func setSessionFindings(t *testing.T, pool *db.Pool, sessionID, findingsJSON string) {
	t.Helper()
	_, err := pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`, findingsJSON, sessionID)
	if err != nil {
		t.Fatalf("setSessionFindings(%s): %v", sessionID, err)
	}
}

// setupFindingsLayerEnv returns a pool, FindingsService, and wfiID using the
// shared derive test environment (project "test-proj", workflow "test-wf",
// agent_defs "analyzer" (layer 0) and "builder" (layer 1)).
func setupFindingsLayerEnv(t *testing.T) (*db.Pool, *FindingsService, string) {
	t.Helper()
	pool, _, wfiID := setupDeriveTestEnv(t)
	return pool, NewFindingsService(pool, clock.Real()), wfiID
}

// assertLayerMap checks the result is a map[string]interface{} and returns it.
func assertLayerMap(t *testing.T, result interface{}) map[string]interface{} {
	t.Helper()
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T: %v", result, result)
	}
	return m
}

// TestFindingsGetByLayer_EmptyLayer verifies that a layer with no agent_definitions returns an empty map.
func TestFindingsGetByLayer_EmptyLayer(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupFindingsLayerEnv(t)

	layer := 99
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=99): unexpected error: %v", err)
	}
	m := assertLayerMap(t, result)
	if len(m) != 0 {
		t.Errorf("expected empty map for empty layer, got %d entries: %v", len(m), m)
	}
}

// TestFindingsGetByLayer_SingleAgent verifies a single agent at a layer returns its parsed findings.
func TestFindingsGetByLayer_SingleAgent(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-single", wfiID, "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-single", `{"result":"ok","score":7}`)

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0): %v", err)
	}
	m := assertLayerMap(t, result)
	if len(m) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(m), m)
	}
	af, ok := m["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("m[analyzer] should be map, got %T", m["analyzer"])
	}
	if af["result"] != "ok" {
		t.Errorf("m[analyzer][result] = %v, want \"ok\"", af["result"])
	}
}

// TestFindingsGetByLayer_MultipleAgentsSameLayer verifies findings are returned for all siblings.
func TestFindingsGetByLayer_MultipleAgentsSameLayer(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	// "builder" is at layer 1; add "reviewer" at layer 1 too.
	insertFindingsAgentDef(t, pool, "reviewer", 1)

	insertSession(t, pool, "sess-builder", wfiID, "builder", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-builder", `{"build":"done"}`)

	insertSession(t, pool, "sess-reviewer", wfiID, "reviewer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-reviewer", `{"review":"approved"}`)

	layer := 1
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=1): %v", err)
	}
	m := assertLayerMap(t, result)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(m), m)
	}

	builderF, ok := m["builder"].(map[string]interface{})
	if !ok {
		t.Fatalf("m[builder] should be map, got %T", m["builder"])
	}
	if builderF["build"] != "done" {
		t.Errorf("m[builder][build] = %v, want \"done\"", builderF["build"])
	}

	reviewerF, ok := m["reviewer"].(map[string]interface{})
	if !ok {
		t.Fatalf("m[reviewer] should be map, got %T", m["reviewer"])
	}
	if reviewerF["review"] != "approved" {
		t.Errorf("m[reviewer][review] = %v, want \"approved\"", reviewerF["review"])
	}
}

// TestFindingsGetByLayer_SiblingWithNoSession verifies that a sibling with no
// terminal session appears in the result with a nil value.
func TestFindingsGetByLayer_SiblingWithNoSession(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertFindingsAgentDef(t, pool, "reviewer", 1)

	// Only builder has a session with findings; reviewer has none.
	insertSession(t, pool, "sess-builder2", wfiID, "builder", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-builder2", `{"output":"binary"}`)

	layer := 1
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=1): %v", err)
	}
	m := assertLayerMap(t, result)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries (builder+reviewer), got %d: %v", len(m), m)
	}

	if m["builder"] == nil {
		t.Error("m[builder] should not be nil")
	}
	if m["reviewer"] != nil {
		t.Errorf("m[reviewer] should be nil (no session), got %v", m["reviewer"])
	}
}

// TestFindingsGetByLayer_ContinuationRows verifies only the latest session's findings
// are returned when the same agent type has multiple sessions.
func TestFindingsGetByLayer_ContinuationRows(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	t1 := "2025-01-01T00:00:00Z"
	t2 := "2025-01-01T00:00:01Z"

	insertSession(t, pool, "sess-old", wfiID, "analyzer", "completed", "pass", t1)
	setSessionFindings(t, pool, "sess-old", `{"data":"old"}`)

	insertSession(t, pool, "sess-new", wfiID, "analyzer", "completed", "pass", t2)
	setSessionFindings(t, pool, "sess-new", `{"data":"new"}`)

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0): %v", err)
	}
	m := assertLayerMap(t, result)

	af, ok := m["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("m[analyzer] should be map, got %T", m["analyzer"])
	}
	if af["data"] != "new" {
		t.Errorf("m[analyzer][data] = %v, want \"new\" (latest session)", af["data"])
	}
}

// TestFindingsGetByLayer_AgentTypeAndLayerMutuallyExclusive verifies that setting
// both AgentType and Layer returns an error.
func TestFindingsGetByLayer_AgentTypeAndLayerMutuallyExclusive(t *testing.T) {
	t.Parallel()
	_, svc, wfiID := setupFindingsLayerEnv(t)

	layer := 0
	_, err := svc.Get(&types.FindingsGetRequest{
		AgentType:  "analyzer",
		Layer:      &layer,
		InstanceID: wfiID,
	})
	if err == nil {
		t.Fatal("expected error when both AgentType and Layer are set, got nil")
	}
}

// TestFindingsGetByLayer_LayerZeroPointer verifies that a Layer pointer to 0
// is treated as layer 0 (not as unset/nil).
func TestFindingsGetByLayer_LayerZeroPointer(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-zero", wfiID, "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-zero", `{"layer_zero":true}`)

	zero := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &zero, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=&0): %v", err)
	}
	m := assertLayerMap(t, result)
	if len(m) == 0 {
		t.Fatal("expected non-empty map for layer 0, got empty (pointer-to-zero may be treated as nil)")
	}
	if _, ok := m["analyzer"]; !ok {
		t.Errorf("expected 'analyzer' key in layer-0 result, got: %v", m)
	}
}

// TestFindingsGetByLayer_MissingInstanceID verifies that a missing instance_id returns an error.
func TestFindingsGetByLayer_MissingInstanceID(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupFindingsLayerEnv(t)

	layer := 0
	_, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: ""})
	if err == nil {
		t.Fatal("expected error for missing instance_id, got nil")
	}
}

// TestFindingsGetByLayer_CallbackSessionExcluded verifies that callback status sessions
// are excluded from the layer findings result (agent appears with nil value).
func TestFindingsGetByLayer_CallbackSessionExcluded(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-cb", wfiID, "analyzer", "callback", "callback", "")
	setSessionFindings(t, pool, "sess-cb", `{"secret":"excluded"}`)

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0): %v", err)
	}
	m := assertLayerMap(t, result)
	// "analyzer" is in the result (agent_def exists) but with nil value (callback excluded).
	if v, ok := m["analyzer"]; ok && v != nil {
		t.Errorf("m[analyzer] should be nil (callback excluded), got %v", v)
	}
}

// TestFindingsGetByLayer_InvalidFindingsJSON verifies that a session with non-JSON
// findings yields nil for that agent type.
func TestFindingsGetByLayer_InvalidFindingsJSON(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-bad-json", wfiID, "analyzer", "completed", "pass", "")
	setSessionFindings(t, pool, "sess-bad-json", "not-valid-json")

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0) with invalid JSON: %v", err)
	}
	m := assertLayerMap(t, result)
	if v, exists := m["analyzer"]; exists && v != nil {
		t.Errorf("m[analyzer] should be nil for invalid JSON, got %v", v)
	}
}
