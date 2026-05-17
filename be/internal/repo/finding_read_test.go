package repo

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
)

type findingReadEnv struct {
	repo  *FindingRepo
	clk   *clock.TestClock
	pool  *db.Pool
	wfiID string
}

// setupFindingReadDB creates a full DB with project, workflow, workflow_instance, agent_defs, and sessions.
func setupFindingReadDB(t *testing.T) *findingReadEnv {
	t.Helper()
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)

	exec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := pool.Exec(q, args...); err != nil {
			t.Fatalf("setupFindingReadDB: %v", err)
		}
	}

	exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('proj-r', 'Test', ?, ?)`, now, now)
	exec(`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('proj-r', 'wf-r', 'Test wf', 'ticket', ?, ?)`, now, now)

	wfiID := "wfi-read-test"
	exec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES (?, 'proj-r', 'tkt-r', 'wf-r', 'ticket', 'active', ?, ?)`, wfiID, now, now)

	exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, layer, model, prompt, tag, low_consumption_model, tools, created_at, updated_at)
		VALUES ('analyzer', 'proj-r', 'wf-r', 0, 'sonnet', '', '', '', '', ?, ?)`, now, now)
	exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, layer, model, prompt, tag, low_consumption_model, tools, created_at, updated_at)
		VALUES ('builder', 'proj-r', 'wf-r', 0, 'sonnet', '', '', '', '', ?, ?)`, now, now)

	// Completed session for analyzer (no model)
	exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result, result_reason, pid, context_left, ancestor_session_id, spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES ('sess-r-1', 'proj-r', 'tkt-r', ?, 'analyzer', 'analyzer', NULL, 'completed', 'pass', NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
		wfiID, now, now, now, now)
	// Running session for builder with model_id
	exec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result, result_reason, pid, context_left, ancestor_session_id, spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES ('sess-r-2', 'proj-r', 'tkt-r', ?, 'builder', 'builder', 'sonnet', 'running', NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		wfiID, now, now, now)

	return &findingReadEnv{repo: NewFindingRepo(pool, clk), clk: clk, pool: pool, wfiID: wfiID}
}

// TestFindingRepo_GetByAgentAllModels_DefaultKey verifies empty model_id maps to "default".
func TestFindingRepo_GetByAgentAllModels_DefaultKey(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)

	env.repo.Upsert("session", "sess-r-1", "result", json.RawMessage(`"ok"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer"}, Actor{Source: "agent"})

	byModel, err := env.repo.GetByAgentAllModels(env.wfiID, "analyzer")
	if err != nil {
		t.Fatalf("GetByAgentAllModels: %v", err)
	}
	if _, ok := byModel["default"]; !ok {
		t.Errorf("expected 'default' key, got: %v", sortedKeys(byModel))
	}
	if v := string(byModel["default"]["result"]); v != `"ok"` {
		t.Errorf("default[result] = %s, want \"ok\"", v)
	}
}

// TestFindingRepo_GetByAgentAllModels_ModelKey verifies non-empty model_id used as map key.
func TestFindingRepo_GetByAgentAllModels_ModelKey(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)

	env.repo.Upsert("session", "sess-r-2", "result", json.RawMessage(`"built"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "builder", ModelID: "sonnet"}, Actor{Source: "agent"})

	byModel, err := env.repo.GetByAgentAllModels(env.wfiID, "builder")
	if err != nil {
		t.Fatalf("GetByAgentAllModels: %v", err)
	}
	if _, ok := byModel["sonnet"]; !ok {
		t.Errorf("expected 'sonnet' key, got: %v", sortedKeys(byModel))
	}
}

// TestFindingRepo_GetByAgentAllModels_MultipleModels verifies multiple model groups returned.
func TestFindingRepo_GetByAgentAllModels_MultipleModels(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)
	actor := Actor{Source: "agent"}

	env.repo.Upsert("session", "sess-r-1", "k", json.RawMessage(`"no-model"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer"}, actor)
	// A second session for same agent but different model
	env.repo.Upsert("session", "sess-r-extra", "k", json.RawMessage(`"with-model"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer", ModelID: "opus"}, actor)

	byModel, err := env.repo.GetByAgentAllModels(env.wfiID, "analyzer")
	if err != nil {
		t.Fatalf("GetByAgentAllModels: %v", err)
	}
	if len(byModel) != 2 {
		t.Errorf("model groups = %d, want 2; keys: %v", len(byModel), sortedKeys(byModel))
	}
	if _, ok := byModel["default"]; !ok {
		t.Error("expected 'default' group for no model_id")
	}
	if _, ok := byModel["opus"]; !ok {
		t.Error("expected 'opus' group")
	}
}

// TestFindingRepo_GetByLayer_AgentWithNoFindings verifies nil inner map for agents with no findings.
func TestFindingRepo_GetByLayer_AgentWithNoFindings(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)

	// Add finding only for "analyzer", not "builder"
	env.repo.Upsert("session", "sess-r-1", "k", json.RawMessage(`1`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer"}, Actor{Source: "agent"})

	byAgent, err := env.repo.GetByLayer(env.wfiID, 0)
	if err != nil {
		t.Fatalf("GetByLayer: %v", err)
	}

	if _, ok := byAgent["analyzer"]; !ok {
		t.Error("expected analyzer in result")
	}
	if byAgent["analyzer"] == nil {
		t.Error("analyzer has findings, inner map should not be nil")
	}

	if _, ok := byAgent["builder"]; !ok {
		t.Error("expected builder in result (agent with no findings)")
	}
	if byAgent["builder"] != nil {
		t.Errorf("builder has no findings, inner map should be nil, got %v", byAgent["builder"])
	}
}

// TestFindingRepo_GetByLayer_EmptyLayer verifies empty map for a layer with no agent_definitions.
func TestFindingRepo_GetByLayer_EmptyLayer(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)

	// Layer 99 has no agent_definitions
	byAgent, err := env.repo.GetByLayer(env.wfiID, 99)
	if err != nil {
		t.Fatalf("GetByLayer layer 99: %v", err)
	}
	if len(byAgent) != 0 {
		t.Errorf("expected empty map for unknown layer, got %v", byAgent)
	}
}

// TestFindingRepo_ListByWorkflowInstance_ExcludesSystemAgents verifies context-saver and conflict-resolver excluded.
func TestFindingRepo_ListByWorkflowInstance_ExcludesSystemAgents(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)
	actor := Actor{Source: "agent"}

	env.repo.Upsert("session", "sess-r-1", "k", json.RawMessage(`"v"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer"}, actor)
	env.repo.Upsert("session", "sess-cs", "k", json.RawMessage(`"cs"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "context-saver"}, actor)
	env.repo.Upsert("session", "sess-cr", "k", json.RawMessage(`"cr"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "conflict-resolver"}, actor)

	result, err := env.repo.ListByWorkflowInstance(env.wfiID)
	if err != nil {
		t.Fatalf("ListByWorkflowInstance: %v", err)
	}
	if _, ok := result["context-saver"]; ok {
		t.Error("context-saver should be excluded")
	}
	if _, ok := result["conflict-resolver"]; ok {
		t.Error("conflict-resolver should be excluded")
	}
	if _, ok := result["analyzer"]; !ok {
		t.Error("analyzer should be included")
	}
}

// TestFindingRepo_ListByWorkflowInstance_KeyFormat verifies key includes model_id when non-empty.
func TestFindingRepo_ListByWorkflowInstance_KeyFormat(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)
	actor := Actor{Source: "agent"}

	// analyzer: no model_id → key = "analyzer"
	env.repo.Upsert("session", "sess-r-1", "k", json.RawMessage(`1`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "analyzer"}, actor)
	// builder: model_id=sonnet → key = "builder:sonnet"
	env.repo.Upsert("session", "sess-r-2", "k", json.RawMessage(`2`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID, AgentType: "builder", ModelID: "sonnet"}, actor)

	result, err := env.repo.ListByWorkflowInstance(env.wfiID)
	if err != nil {
		t.Fatalf("ListByWorkflowInstance: %v", err)
	}
	if _, ok := result["analyzer"]; !ok {
		t.Errorf("expected key 'analyzer', got keys: %v", sortedKeys(result))
	}
	if _, ok := result["builder:sonnet"]; !ok {
		t.Errorf("expected key 'builder:sonnet', got keys: %v", sortedKeys(result))
	}
}

// TestFindingRepo_GetSessionFindingByKey_PrioritizesCompleted verifies completed sessions take priority.
func TestFindingRepo_GetSessionFindingByKey_PrioritizesCompleted(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)
	actor := Actor{Source: "agent"}

	// sess-r-1 is completed, sess-r-2 is running
	env.repo.Upsert("session", "sess-r-1", "answer", json.RawMessage(`"from-completed"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID}, actor)
	env.repo.Upsert("session", "sess-r-2", "answer", json.RawMessage(`"from-running"`), //nolint:errcheck
		Denorm{WorkflowInstanceID: env.wfiID}, actor)

	val, ok := env.repo.GetSessionFindingByKey(env.wfiID, "answer")
	if !ok {
		t.Fatal("GetSessionFindingByKey returned not-found")
	}
	if string(val) != `"from-completed"` {
		t.Errorf("value = %s, want \"from-completed\" (completed session takes priority)", val)
	}
}

// TestFindingRepo_GetSessionFindingByKey_NotFound verifies false returned when key absent.
func TestFindingRepo_GetSessionFindingByKey_NotFound(t *testing.T) {
	t.Parallel()
	env := setupFindingReadDB(t)

	_, ok := env.repo.GetSessionFindingByKey(env.wfiID, "nonexistent-key")
	if ok {
		t.Error("expected not-found for missing key, got ok=true")
	}
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
