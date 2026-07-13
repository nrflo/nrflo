package spawner

import (
	"strings"
	"testing"
	"time"

	"be/internal/db"

	"github.com/google/uuid"
)

// insertNodeSession inserts a completed (or running, when endedAt=="") agent
// session with an explicit node_id, so a single agent_type can be attributed
// to more than one execution node (fan-out simulation).
func insertNodeSession(t *testing.T, pool *db.Pool, sessionID, projectID, ticketID, wfiID, nodeID, agentType, endedAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status, result := "completed", "pass"
	var endedAtVal interface{}
	if endedAt == "" {
		status, result = "running", ""
	} else {
		endedAtVal = endedAt
	}
	var resultVal interface{}
	if result != "" {
		resultVal = result
	}
	_, err := pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type,
			status, result, result_reason, pid, context_left, ancestor_session_id,
			spawn_command, prompt, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, ?, ?, ?)`,
		sessionID, projectID, ticketID, wfiID, agentType, nodeID, agentType, status, resultVal, now, endedAtVal, now, now)
	if err != nil {
		t.Fatalf("insertNodeSession(%s): %v", sessionID, err)
	}
}

// TestLoadTemplate_NodeFindings_SingleNode_ResolvesEarlierLayerNode verifies
// #{NODE_FINDINGS:scrape-g2} in a layer-2 agent's prompt expands to that
// node's own findings (single-node fixture reusing the layer-1 scraper setup,
// where node_id falls back to phase == the agent def id).
func TestLoadTemplate_NodeFindings_SingleNode_ResolvesEarlierLayerNode(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	wfiID, ticketID := setupLayer1ScraperFixture(t, env)

	createAgentDefWithLayer(t, env, "merger", "## G2\n#{NODE_FINDINGS:scrape-g2}\n\nDone.", 2)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("merger", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 2)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if strings.Contains(result, "#{NODE_FINDINGS:") {
		t.Error("#{NODE_FINDINGS:scrape-g2} was not consumed")
	}
	if !strings.Contains(result, "review_count: 5") {
		t.Errorf("expected 'review_count: 5' from node scrape-g2 in result:\n%s", result)
	}
	if strings.Contains(result, "posts") || strings.Contains(result, "reddit") {
		t.Errorf("expected only scrape-g2's own findings, got sibling data leaking in:\n%s", result)
	}
}

// TestLoadTemplate_NodeFindings_UnknownNode_ExpandsEmptyNoError verifies an
// unknown node_id expands to "" (with a logged warning) and loadTemplate
// still returns a nil error — same convention as #{ARTIFACT:name} misses.
func TestLoadTemplate_NodeFindings_UnknownNode_ExpandsEmptyNoError(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "consumer", "before[#{NODE_FINDINGS:ghost-node}]after", 0)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate returned unexpected error: %v", err)
	}
	if strings.Contains(result, "#{NODE_FINDINGS:") {
		t.Error("#{NODE_FINDINGS:ghost-node} was not consumed")
	}
	if result != "before[]after" {
		t.Errorf("expected 'before[]after' for an unknown node, got: %q", result)
	}
}

// TestLoadTemplate_NodeFindings_KnownNodeNoFindings_Placeholder verifies a
// node with a session but zero findings renders the standard missing-findings
// placeholder keyed by node id.
func TestLoadTemplate_NodeFindings_KnownNodeNoFindings_Placeholder(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "empty-node", "Scrape empty", 0)
	insertNodeSession(t, env.pool, "sess-empty-node", env.project, ticketID, wfiID, "empty-node", "empty-node", "2025-01-01T00:00:01Z")

	createAgentDefWithLayer(t, env, "consumer", "#{NODE_FINDINGS:empty-node}", 1)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 1)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "_No findings yet available from empty-node_") {
		t.Errorf("expected empty-node placeholder, got: %q", result)
	}
}

// TestLoadTemplate_NodeFindings_KeySelection verifies single-key selection
// renders a bare value and multi-key selection renders a two-line block.
func TestLoadTemplate_NodeFindings_KeySelection(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "picker", "Pick", 0)
	insertNodeSession(t, env.pool, "sess-picker", env.project, ticketID, wfiID, "picker", "picker", "2025-01-01T00:00:01Z")
	setLayerSessionFindings(t, env.pool, "sess-picker", `{"a":"1","b":"2"}`)

	createAgentDefWithLayer(t, env, "single-consumer", "Value: #{NODE_FINDINGS:picker:a}", 1)
	createAgentDefWithLayer(t, env, "multi-consumer", "#{NODE_FINDINGS:picker:a,b}", 1)

	sp := env.newSpawner()

	single, _, _, err := sp.loadTemplate("single-consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 1)
	if err != nil {
		t.Fatalf("loadTemplate(single-consumer) failed: %v", err)
	}
	if !strings.Contains(single, "Value:  1") {
		t.Errorf("expected bare value 'Value:  1', got: %q", single)
	}

	multi, _, _, err := sp.loadTemplate("multi-consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet", "", wfiID, nil, 1)
	if err != nil {
		t.Fatalf("loadTemplate(multi-consumer) failed: %v", err)
	}
	if !strings.Contains(multi, "a: 1") || !strings.Contains(multi, "b: 2") {
		t.Errorf("expected two-line 'a: 1' / 'b: 2' block, got: %q", multi)
	}
}
