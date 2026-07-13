package spawner

import (
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestValidateAndAdvancePhase_PriorLayerGate_BlocksOnMissingNode verifies the
// prior-layer gate blocks when the prior layer's node has no terminal
// session — even though a session exists under the same agent_type but a
// different node_id (proving the gate checks node_id, not agent_type).
func TestValidateAndAdvancePhase_PriorLayerGate_BlocksOnMissingNode(t *testing.T) {
	pool := setupTestDB(t)
	s := New(Config{
		Pool:  pool,
		Clock: clock.Real(),
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{
					{NodeID: "analyzer", Agent: "analyzer", Layer: 0},
					{NodeID: "builder", Agent: "builder", Layer: 1},
				},
			},
		},
	})

	// A terminal session exists, but under a different node_id ("analyzer#2")
	// than the one the workflow def expects for layer 0 ("analyzer") — a
	// stand-in for a fan-out sibling sharing the analyzer template.
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, result, created_at, updated_at)
		VALUES ('sess-sibling', 'proj', 'T-1', 'wfi-1', 'analyzer', 'analyzer#2', 'analyzer', 'completed', 'pass', datetime('now'), datetime('now'))`)

	wi := &model.WorkflowInstance{ID: "wfi-1"}
	_, err := s.validateAndAdvancePhase(wi, "feature", "builder")
	if err == nil {
		t.Fatal("validateAndAdvancePhase: expected error (prior node incomplete), got nil")
	}
	if !strings.Contains(err.Error(), "analyzer") {
		t.Errorf("error = %q, want it to mention the incomplete prior node", err.Error())
	}
}

// TestValidateAndAdvancePhase_PriorLayerGate_PassesWhenNodeTerminal verifies
// the gate is satisfied once the prior layer's exact node_id has a terminal
// session.
func TestValidateAndAdvancePhase_PriorLayerGate_PassesWhenNodeTerminal(t *testing.T) {
	pool := setupTestDB(t)
	s := New(Config{
		Pool:  pool,
		Clock: clock.Real(),
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{
					{NodeID: "analyzer", Agent: "analyzer", Layer: 0},
					{NodeID: "builder", Agent: "builder", Layer: 1},
				},
			},
		},
	})

	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, status, result, created_at, updated_at)
		VALUES ('sess-analyzer', 'proj', 'T-1', 'wfi-1', 'analyzer', 'analyzer', 'analyzer', 'completed', 'pass', datetime('now'), datetime('now'))`)

	wi := &model.WorkflowInstance{ID: "wfi-1"}
	nodeID, err := s.validateAndAdvancePhase(wi, "feature", "builder")
	if err != nil {
		t.Fatalf("validateAndAdvancePhase: unexpected error: %v", err)
	}
	if nodeID != "builder" {
		t.Errorf("nodeID = %q, want builder", nodeID)
	}
}

// TestValidateAndAdvancePhase_NoPriorLayers_Passes verifies layer 0 nodes
// need no prior-layer session at all.
func TestValidateAndAdvancePhase_NoPriorLayers_Passes(t *testing.T) {
	pool := setupTestDB(t)
	s := New(Config{
		Pool:  pool,
		Clock: clock.Real(),
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{
					{NodeID: "analyzer", Agent: "analyzer", Layer: 0},
				},
			},
		},
	})

	wi := &model.WorkflowInstance{ID: "wfi-1"}
	nodeID, err := s.validateAndAdvancePhase(wi, "feature", "analyzer")
	if err != nil {
		t.Fatalf("validateAndAdvancePhase: unexpected error: %v", err)
	}
	if nodeID != "analyzer" {
		t.Errorf("nodeID = %q, want analyzer", nodeID)
	}
}

// TestValidateAndAdvancePhase_UnknownNode_Errors verifies a requested node
// absent from the workflow definition is rejected.
func TestValidateAndAdvancePhase_UnknownNode_Errors(t *testing.T) {
	pool := setupTestDB(t)
	s := New(Config{
		Pool:  pool,
		Clock: clock.Real(),
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{
					{NodeID: "analyzer", Agent: "analyzer", Layer: 0},
				},
			},
		},
	})

	wi := &model.WorkflowInstance{ID: "wfi-1"}
	_, err := s.validateAndAdvancePhase(wi, "feature", "nonexistent-node")
	if err == nil {
		t.Fatal("validateAndAdvancePhase: expected error for unknown node, got nil")
	}
}
