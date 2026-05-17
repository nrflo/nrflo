package service

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
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

// setSessionFindings stores findingsJSON keys into the findings table for the given session.
// It looks up the session's agent_type and workflow_instance_id from the DB.
// Invalid JSON is silently skipped (no findings stored).
func setSessionFindings(t *testing.T, pool *db.Pool, sessionID, findingsJSON string) {
	t.Helper()
	var agentType, wfiID string
	err := pool.QueryRow(
		`SELECT agent_type, workflow_instance_id FROM agent_sessions WHERE id = ?`, sessionID,
	).Scan(&agentType, &wfiID)
	if err != nil {
		t.Fatalf("setSessionFindings: lookup session %s: %v", sessionID, err)
	}
	var m map[string]json.RawMessage
	if jsonErr := json.Unmarshal([]byte(findingsJSON), &m); jsonErr != nil {
		// Invalid JSON — no findings stored. Tests asserting nil result will pass.
		return
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	for k, v := range m {
		if err := fr.Upsert("session", sessionID, k, v,
			repo.Denorm{WorkflowInstanceID: wfiID, AgentType: agentType},
			repo.Actor{Source: "system"}); err != nil {
			t.Fatalf("setSessionFindings(%s, key=%s): %v", sessionID, k, err)
		}
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
// session appears in the result with a nil value.
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

// TestFindingsGetByLayer_MultipleSessionsSameAgent verifies that when the same agent_type
// has multiple sessions, findings from both sessions are aggregated (each session may
// contribute distinct keys to the merged result).
func TestFindingsGetByLayer_MultipleSessionsSameAgent(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	t1 := "2025-01-01T00:00:00Z"
	t2 := "2025-01-01T00:00:01Z"

	// Two sessions for the same agent_type; each writes a unique key.
	insertSession(t, pool, "sess-first", wfiID, "analyzer", "completed", "pass", t1)
	setSessionFindings(t, pool, "sess-first", `{"from_first":"yes"}`)

	insertSession(t, pool, "sess-second", wfiID, "analyzer", "completed", "pass", t2)
	setSessionFindings(t, pool, "sess-second", `{"from_second":"yes"}`)

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
	// Both sessions' distinct keys should appear in the merged result.
	if af["from_first"] != "yes" {
		t.Errorf("m[analyzer][from_first] = %v, want \"yes\"", af["from_first"])
	}
	if af["from_second"] != "yes" {
		t.Errorf("m[analyzer][from_second] = %v, want \"yes\"", af["from_second"])
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

// TestFindingsGetByLayer_SessionWithNoFindings verifies that a session with no
// findings stored yields nil for that agent type in the layer result.
func TestFindingsGetByLayer_SessionWithNoFindings(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	// Session exists but no findings stored in findings table.
	insertSession(t, pool, "sess-empty", wfiID, "analyzer", "completed", "pass", "")

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0) with no findings: %v", err)
	}
	m := assertLayerMap(t, result)
	if v, exists := m["analyzer"]; exists && v != nil {
		t.Errorf("m[analyzer] should be nil for session with no findings, got %v", v)
	}
}

// TestFindingsGetByLayer_InvalidFindingsJSON verifies that a session with invalid
// JSON passed to setSessionFindings (which skips invalid input) yields nil for that agent type.
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
		t.Errorf("m[analyzer] should be nil for invalid JSON (no findings stored), got %v", v)
	}
}

// TestFindingsGetByLayer_CallbackSessionFindings verifies that findings stored
// for a callback-status session ARE included in the layer result (findings are
// stored per-session in the findings table, not filtered by session status).
func TestFindingsGetByLayer_CallbackSessionFindings(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupFindingsLayerEnv(t)

	insertSession(t, pool, "sess-cb", wfiID, "analyzer", "callback", "callback", "")
	setSessionFindings(t, pool, "sess-cb", `{"stored":"yes"}`)

	layer := 0
	result, err := svc.Get(&types.FindingsGetRequest{Layer: &layer, InstanceID: wfiID})
	if err != nil {
		t.Fatalf("Get(layer=0): %v", err)
	}
	m := assertLayerMap(t, result)
	af, ok := m["analyzer"].(map[string]interface{})
	if !ok {
		t.Fatalf("m[analyzer] should be map (findings are stored regardless of session status), got %T", m["analyzer"])
	}
	if af["stored"] != "yes" {
		t.Errorf("m[analyzer][stored] = %v, want \"yes\"", af["stored"])
	}
}
