package service

import (
	"context"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

// TestBuildV4State_PlanBlockAbsentWithNoPlan asserts a plain instance with no
// plan head at all omits the "plan" key entirely (sql.ErrNoRows swallowed).
func TestBuildV4State_PlanBlockAbsentWithNoPlan(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	if _, ok := state["plan"]; ok {
		t.Errorf(`state["plan"] = %+v, want key absent (no plan head)`, state["plan"])
	}
}

// TestBuildV4State_OriginOmittedWhenEmptyPresentWhenSet mirrors parent_session's
// omit-when-empty pattern for origin/origin_session_id.
func TestBuildV4State_OriginOmittedWhenEmptyPresentWhenSet(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clk)

	wi, err := wfiRepo.Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	state := svc.buildV4State(wi)
	if _, ok := state["origin"]; ok {
		t.Errorf(`state["origin"] = %+v, want key absent (empty origin)`, state["origin"])
	}
	if _, ok := state["origin_session_id"]; ok {
		t.Errorf(`state["origin_session_id"] = %+v, want key absent (empty origin_session_id)`, state["origin_session_id"])
	}

	wi.Origin = "console"
	wi.OriginSessionID = "sess-v4-1"
	state = svc.buildV4State(wi)
	if state["origin"] != "console" {
		t.Errorf(`state["origin"] = %v, want "console"`, state["origin"])
	}
	if state["origin_session_id"] != "sess-v4-1" {
		t.Errorf(`state["origin_session_id"] = %v, want "sess-v4-1"`, state["origin_session_id"])
	}
}

// TestBuildV4State_PlanBlockReflectsHeadAndMaterializedRevision asserts the
// plan block reports the approved status and the materialized revision number
// after Revise+Approve (which materializes in the same request).
func TestBuildV4State_PlanBlockReflectsHeadAndMaterializedRevision(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)
	planSvc := NewPlanService(pool, clk, nil)

	rev, err := planSvc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := planSvc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	planBlock, ok := state["plan"].(map[string]interface{})
	if !ok {
		t.Fatalf(`state["plan"] = %#v (%T), want map[string]interface{}`, state["plan"], state["plan"])
	}
	if planBlock["status"] != "approved" {
		t.Errorf(`plan["status"] = %v, want "approved"`, planBlock["status"])
	}
	if planBlock["materialized_revision"] != rev.Revision {
		t.Errorf(`plan["materialized_revision"] = %v, want %d`, planBlock["materialized_revision"], rev.Revision)
	}
}

// TestBuildV4State_MaterializedNodesAppearInPhaseOrderAndLayers asserts a
// materialized node merges into phase_order/phase_layers alongside the
// (empty, in this fixture) static def-derived phases, at its offset layer.
func TestBuildV4State_MaterializedNodesAppearInPhaseOrderAndLayers(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)
	planSvc := NewPlanService(pool, clk, nil)

	rev, err := planSvc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := planSvc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	phaseOrder, ok := state["phase_order"].([]string)
	if !ok {
		t.Fatalf(`state["phase_order"] = %#v (%T), want []string`, state["phase_order"], state["phase_order"])
	}
	found := false
	for _, id := range phaseOrder {
		if id == "step1" {
			found = true
		}
	}
	if !found {
		t.Errorf(`phase_order = %v, want it to contain "step1"`, phaseOrder)
	}

	phaseLayers, ok := state["phase_layers"].(map[string]int)
	if !ok {
		t.Fatalf(`state["phase_layers"] = %#v (%T), want map[string]int`, state["phase_layers"], state["phase_layers"])
	}
	// setupPlanTestEnv's project has no static agent_definitions, so
	// maxStaticExecutableLayer is -1 and the offset is 0.
	if phaseLayers["step1"] != 0 {
		t.Errorf(`phase_layers["step1"] = %d, want 0`, phaseLayers["step1"])
	}
}

// TestBuildV4State_MaterializedLayerPolicyMergedIntoLayerPolicies asserts the
// materialized layer's pass policy is merged into layer_policies.
func TestBuildV4State_MaterializedLayerPolicyMergedIntoLayerPolicies(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	clk := clock.Real()
	svc := NewWorkflowService(pool, clk)
	planSvc := NewPlanService(pool, clk, nil)

	rev, err := planSvc.Revise(context.Background(), instanceID, types.PlanReviseRequest{
		Revision: 0, Manifest: validPlanManifestJSON("goal", "do it"),
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := planSvc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	wi, err := repo.NewWorkflowInstanceRepo(pool, clk).Get(instanceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	state := svc.buildV4State(wi)
	policies, ok := state["layer_policies"].(map[int]string)
	if !ok {
		t.Fatalf(`state["layer_policies"] = %#v (%T), want map[int]string`, state["layer_policies"], state["layer_policies"])
	}
	if policies[0] != "all" {
		t.Errorf(`layer_policies[0] = %q, want "all"`, policies[0])
	}
}
