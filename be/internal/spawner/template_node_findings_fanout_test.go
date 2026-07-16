package spawner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestLoadTemplate_NodeFindings_FanOutSiblingsDisjoint_AggregateStillMerges
// verifies two sessions sharing one agent_type but distinct node_ids resolve
// to disjoint #{NODE_FINDINGS:<node>} blocks, while #{FINDINGS:<agent_type>}
// stays template-keyed and aggregates across both nodes.
func TestLoadTemplate_NodeFindings_FanOutSiblingsDisjoint_AggregateStillMerges(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "worker", "Scrape one source", 0)
	insertNodeSession(t, env.pool, "sess-worker-1", env.project, ticketID, wfiID, "worker#1", "worker", "2025-01-01T00:00:01Z")
	setLayerSessionFindings(t, env.pool, "sess-worker-1", `{"picked":"g2"}`)
	insertNodeSession(t, env.pool, "sess-worker-2", env.project, ticketID, wfiID, "worker#2", "worker", "2025-01-01T00:00:02Z")
	setLayerSessionFindings(t, env.pool, "sess-worker-2", `{"picked":"reddit"}`)

	createAgentDefWithLayer(t, env, "consumer",
		"W1: #{NODE_FINDINGS:worker#1:picked}\nW2: #{NODE_FINDINGS:worker#2:picked}\nAgg:\n#{FINDINGS:worker}", 1)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet-5", "", wfiID, nil, 1)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "W1:  g2") {
		t.Errorf("expected worker#1's disjoint value 'g2', got:\n%s", result)
	}
	if !strings.Contains(result, "W2:  reddit") {
		t.Errorf("expected worker#2's disjoint value 'reddit', got:\n%s", result)
	}
	// #{FINDINGS:worker} aggregates across both sibling nodes of the template.
	if !strings.Contains(result, "picked:") {
		t.Errorf("expected aggregate #{FINDINGS:worker} block, got:\n%s", result)
	}
}

// TestLoadTemplate_NodeFindings_EndedSessionWinsOverRunning verifies that
// among multiple sessions for one node, the most-recently-ended session's
// value wins, and a still-running retry never shadows an already-ended one.
func TestLoadTemplate_NodeFindings_EndedSessionWinsOverRunning(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "flaky", "Flaky scraper", 0)
	insertNodeSession(t, env.pool, "sess-flaky-early", env.project, ticketID, wfiID, "flaky-node", "flaky", "2025-01-01T00:00:01Z")
	setLayerSessionFindings(t, env.pool, "sess-flaky-early", `{"status":"first"}`)
	insertNodeSession(t, env.pool, "sess-flaky-late", env.project, ticketID, wfiID, "flaky-node", "flaky", "2025-01-01T00:00:05Z")
	setLayerSessionFindings(t, env.pool, "sess-flaky-late", `{"status":"second"}`)
	insertNodeSession(t, env.pool, "sess-flaky-running", env.project, ticketID, wfiID, "flaky-node", "flaky", "" /* still running */)
	setLayerSessionFindings(t, env.pool, "sess-flaky-running", `{"status":"running-retry"}`)

	createAgentDefWithLayer(t, env, "consumer", "#{NODE_FINDINGS:flaky-node:status}", 1)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("consumer", ticketID, env.project, "p", "c", "test", "claude:sonnet-5", "", wfiID, nil, 1)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, " second") {
		t.Errorf("expected most-recently-ended session's value 'second', got: %q", result)
	}
	if strings.Contains(result, "running-retry") || strings.Contains(result, "first") {
		t.Errorf("running/earlier sessions must not shadow the latest ended session, got: %q", result)
	}
}

// TestLoadTemplate_NodeID_ExpandsToArgument verifies ${NODE_ID} expands to the
// nodeID argument passed to loadTemplate (the execution slot id).
func TestLoadTemplate_NodeID_ExpandsToArgument(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "worker", "Running as ${NODE_ID}", 0)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("worker", ticketID, env.project, "p", "c", "test", "claude:sonnet-5", "worker#3", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "Running as worker#3") {
		t.Errorf("expected ${NODE_ID} to expand to 'worker#3', got: %q", result)
	}
}

// TestLoadTemplate_NodeID_FallsBackToAgentType_WhenEmpty verifies ${NODE_ID}
// falls back to the agent type when nodeID is unset (Preview / interactive
// L0 starts, which have no execution slot yet).
func TestLoadTemplate_NodeID_FallsBackToAgentType_WhenEmpty(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "NF-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)

	createAgentDefWithLayer(t, env, "worker", "Running as ${NODE_ID}", 0)

	sp := env.newSpawner()
	result, _, _, err := sp.loadTemplate("worker", ticketID, env.project, "p", "c", "test", "claude:sonnet-5", "" /* nodeID unset */, wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if !strings.Contains(result, "Running as worker") {
		t.Errorf("expected ${NODE_ID} to fall back to agent type 'worker', got: %q", result)
	}
}
