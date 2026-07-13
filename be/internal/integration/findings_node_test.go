package integration

import (
	"testing"
	"time"

	"be/internal/socket"
)

// insertAgentSessionWithNode inserts an agent session with an explicit
// node_id distinct from its phase, so fan-out (one agent_type spawning
// multiple execution nodes) can be exercised end-to-end over the socket.
func (e *TestEnv) insertAgentSessionWithNode(t *testing.T, id, ticketID, wfiID, nodeID, agentType string) {
	t.Helper()
	e.InsertAgentSession(t, id, ticketID, wfiID, nodeID /* phase */, agentType, "")
}

// TestFindingsNodeGet_FullFlow exercises the complete node-scoped findings
// read path over the socket: two sessions sharing one agent_type but
// spawned under distinct node_ids write disjoint findings, each is readable
// by its own node_id (disjoint from its sibling), the template-keyed
// aggregate still merges both, an unknown node_id errors, and node_id
// combined with agent_type is rejected as mutually exclusive.
func TestFindingsNodeGet_FullFlow(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "NODE-FLOW-1", "Node findings test")
	env.InitWorkflow(t, "NODE-FLOW-1")
	wfiID := env.GetWorkflowInstanceID(t, "NODE-FLOW-1", "test")

	env.insertAgentSessionWithNode(t, "sess-node-1", "NODE-FLOW-1", wfiID, "worker#1", "worker")
	env.insertAgentSessionWithNode(t, "sess-node-2", "NODE-FLOW-1", wfiID, "worker#2", "worker")

	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-node-1",
		"instance_id": wfiID,
		"key":         "picked",
		"value":       `"g2"`,
	}, nil)
	env.MustExecute(t, "findings.add", map[string]interface{}{
		"session_id":  "sess-node-2",
		"instance_id": wfiID,
		"key":         "picked",
		"value":       `"reddit"`,
	}, nil)

	// Complete the sessions with distinct ended_at so the template-keyed
	// aggregate (which prefers the most-recently-ended session on key
	// collision) resolves deterministically instead of racing on tied NULLs.
	env.CompleteAgentSession(t, "sess-node-1", "pass")
	env.Clock.Advance(time.Second)
	env.CompleteAgentSession(t, "sess-node-2", "pass")

	// Each node reads only its own finding.
	var node1Result map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"node_id":     "worker#1",
		"instance_id": wfiID,
	}, &node1Result)
	if node1Result["picked"] != "g2" {
		t.Fatalf("node worker#1 picked = %v, want g2", node1Result["picked"])
	}

	var node2Result map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"node_id":     "worker#2",
		"instance_id": wfiID,
	}, &node2Result)
	if node2Result["picked"] != "reddit" {
		t.Fatalf("node worker#2 picked = %v, want reddit", node2Result["picked"])
	}

	// The template-keyed aggregate still merges across both sibling nodes.
	var aggResult map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"agent_type":  "worker",
		"instance_id": wfiID,
	}, &aggResult)
	if aggResult["picked"] != "reddit" {
		t.Fatalf("aggregate picked = %v, want reddit (most-recently-ended session wins)", aggResult["picked"])
	}

	// Unknown node_id errors rather than silently returning empty.
	env.ExpectError(t, "findings.get", map[string]interface{}{
		"node_id":     "ghost-node",
		"instance_id": wfiID,
	}, socket.ErrCodeNotFound)

	// node_id + agent_type is rejected up front as mutually exclusive.
	env.ExpectError(t, "findings.get", map[string]interface{}{
		"node_id":     "worker#1",
		"agent_type":  "worker",
		"instance_id": wfiID,
	}, socket.ErrCodeInvalidParams)
}

// TestFindingsNodeGet_KnownNodeNoFindings verifies a node with a session but
// no findings yet returns an empty map (not an error) over the socket.
func TestFindingsNodeGet_KnownNodeNoFindings(t *testing.T) {
	env := NewTestEnv(t)

	env.CreateTicket(t, "NODE-FLOW-2", "Node findings empty test")
	env.InitWorkflow(t, "NODE-FLOW-2")
	wfiID := env.GetWorkflowInstanceID(t, "NODE-FLOW-2", "test")

	env.insertAgentSessionWithNode(t, "sess-node-empty", "NODE-FLOW-2", wfiID, "idle-node", "worker")

	var result map[string]interface{}
	env.MustExecute(t, "findings.get", map[string]interface{}{
		"node_id":     "idle-node",
		"instance_id": wfiID,
	}, &result)
	if len(result) != 0 {
		t.Fatalf("expected empty map for a known node with no findings, got %v", result)
	}
}
