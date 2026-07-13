package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestInstanceNodeRepo_InsertAndList_OrderedByLayerThenNodeID verifies List
// returns nodes ordered by layer ASC, node_id ASC regardless of insert order.
func TestInstanceNodeRepo_InsertAndList_OrderedByLayerThenNodeID(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-node-order"
	seedPlanInstance(t, pool, instanceID)
	r := NewInstanceNodeRepo(pool, clock.Real())

	nodes := []model.InstanceNode{
		{NodeID: "b-node", Layer: 1, AgentType: "implementor", Instructions: "do b", PlanRevision: 1},
		{NodeID: "a-node", Layer: 0, AgentType: "setup-analyzer", Instructions: "do a", PlanRevision: 1},
		{NodeID: "c-node", Layer: 1, AgentType: "qa-verifier", Instructions: "do c", PlanRevision: 1},
		{NodeID: "z-node", Layer: 0, AgentType: "implementor", Instructions: "do z", PlanRevision: 1},
	}

	tx, err := pool.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.InsertNodes(tx, instanceID, nodes); err != nil {
		t.Fatalf("InsertNodes: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := r.List(instanceID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}

	wantOrder := []string{"a-node", "z-node", "b-node", "c-node"}
	for i, nodeID := range wantOrder {
		if got[i].NodeID != nodeID {
			t.Errorf("got[%d].NodeID = %q, want %q", i, got[i].NodeID, nodeID)
		}
		if got[i].InstanceID != instanceID {
			t.Errorf("got[%d].InstanceID = %q, want %q", i, got[i].InstanceID, instanceID)
		}
	}
	if got[0].Layer != 0 || got[1].Layer != 0 || got[2].Layer != 1 || got[3].Layer != 1 {
		t.Errorf("layers out of order: %+v", got)
	}
}

// TestInstanceNodeRepo_InsertLayerPoliciesAndList round-trips a layer -> pass
// policy map.
func TestInstanceNodeRepo_InsertLayerPoliciesAndList(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-layer-policies"
	seedPlanInstance(t, pool, instanceID)
	r := NewInstanceNodeRepo(pool, clock.Real())

	policies := map[int]string{
		0: "all",
		1: "any",
		2: "majority",
	}

	tx, err := pool.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.InsertLayerPolicies(tx, instanceID, policies); err != nil {
		t.Fatalf("InsertLayerPolicies: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := r.ListLayerPolicies(instanceID)
	if err != nil {
		t.Fatalf("ListLayerPolicies: %v", err)
	}
	if len(got) != len(policies) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(policies))
	}
	for layer, want := range policies {
		if got[layer] != want {
			t.Errorf("got[%d] = %q, want %q", layer, got[layer], want)
		}
	}
}

// TestInstanceNodeRepo_List_UnknownInstance_ReturnsEmptyNotError verifies
// List/ListLayerPolicies on a never-materialized instance id return an empty
// result, not an error.
func TestInstanceNodeRepo_List_UnknownInstance_ReturnsEmptyNotError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewInstanceNodeRepo(pool, clock.Real())

	nodes, err := r.List("no-such-instance")
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("List(unknown) = %+v, want empty", nodes)
	}

	policies, err := r.ListLayerPolicies("no-such-instance")
	if err != nil {
		t.Fatalf("ListLayerPolicies: unexpected error: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("ListLayerPolicies(unknown) = %+v, want empty", policies)
	}
}

// TestInstanceNodeRepo_InsertNodes_DuplicateNodeIDInSameInstance_PKViolation
// proves the table has no upsert semantics: a second InsertNodes call with an
// overlapping (instance_id, node_id) pair is a raw PK violation. Callers
// (service.PlanService.Materialize) are solely responsible for exactly-once
// semantics via their own hash-stamp guard, not this repo.
func TestInstanceNodeRepo_InsertNodes_DuplicateNodeIDInSameInstance_PKViolation(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-dup-node"
	seedPlanInstance(t, pool, instanceID)
	r := NewInstanceNodeRepo(pool, clock.Real())

	first := []model.InstanceNode{
		{NodeID: "dup-node", Layer: 0, AgentType: "implementor", Instructions: "v1", PlanRevision: 1},
	}
	tx1, err := pool.Begin()
	if err != nil {
		t.Fatalf("Begin (first): %v", err)
	}
	if err := r.InsertNodes(tx1, instanceID, first); err != nil {
		t.Fatalf("InsertNodes (first): %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("Commit (first): %v", err)
	}

	second := []model.InstanceNode{
		{NodeID: "dup-node", Layer: 0, AgentType: "implementor", Instructions: "v2", PlanRevision: 2},
	}
	tx2, err := pool.Begin()
	if err != nil {
		t.Fatalf("Begin (second): %v", err)
	}
	insertErr := r.InsertNodes(tx2, instanceID, second)
	_ = tx2.Rollback()
	if insertErr == nil {
		t.Fatal("InsertNodes (second, duplicate node_id): expected PK violation error, got nil")
	}
}

// TestInstanceNodeRepo_CascadeDeleteViaWorkflowInstance is the repo-level
// mirror of the db-level cascade test: deleting the parent workflow_instances
// row must empty both List and ListLayerPolicies.
func TestInstanceNodeRepo_CascadeDeleteViaWorkflowInstance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-cascade-node"
	seedPlanInstance(t, pool, instanceID)
	r := NewInstanceNodeRepo(pool, clock.Real())

	tx, err := pool.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.InsertNodes(tx, instanceID, []model.InstanceNode{
		{NodeID: "node-1", Layer: 0, AgentType: "implementor", Instructions: "do it", PlanRevision: 1},
	}); err != nil {
		t.Fatalf("InsertNodes: %v", err)
	}
	if err := r.InsertLayerPolicies(tx, instanceID, map[int]string{0: "all"}); err != nil {
		t.Fatalf("InsertLayerPolicies: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := pool.Exec(`DELETE FROM workflow_instances WHERE id = ?`, instanceID); err != nil {
		t.Fatalf("delete workflow_instances: %v", err)
	}

	nodes, err := r.List(instanceID)
	if err != nil {
		t.Fatalf("List after cascade delete: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("List after cascade delete = %+v, want empty", nodes)
	}

	policies, err := r.ListLayerPolicies(instanceID)
	if err != nil {
		t.Fatalf("ListLayerPolicies after cascade delete: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("ListLayerPolicies after cascade delete = %+v, want empty", policies)
	}
}
