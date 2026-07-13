package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// insertApprovedPlanBypassingService writes an approved plan head + revision
// directly via SQL, skipping PlanService.Revise/Approve (and therefore
// ValidatePlanManifest) entirely. This simulates a manifest that reached
// 'approved' status without passing the service's own validation gate, which
// is exactly why Materialize must re-validate independently rather than trust
// the stored status.
func insertApprovedPlanBypassingService(t *testing.T, pool *db.Pool, instanceID string, m PlanManifest) {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO plan_revisions (instance_id, revision, manifest, hash, author, planner_session_id, created_at)
		 VALUES (?, 1, ?, ?, 'caller', '', ?)`,
		instanceID, string(raw), HashManifest(m), now)
	mustExec(t, pool, `INSERT INTO workflow_plans (instance_id, status, latest_revision, approved_revision, goal, materialized_revision, materialized_hash, created_at, updated_at)
		 VALUES (?, 'approved', 1, 1, ?, 0, '', ?, ?)`,
		instanceID, m.Goal, now, now)
}

// TestPlanMaterialize_QuorumExceedsFinalNodeCount_RejectedAtMaterializeTime is
// the acceptance-critical case for Materialize re-validating pass policy
// against final node counts independent of revise/approve-time checks: a
// manifest that reached 'approved' via direct SQL (bypassing the service's own
// gate) with quorum:3 over 2 nodes must be rejected, all-or-nothing.
func TestPlanMaterialize_QuorumExceedsFinalNodeCount_RejectedAtMaterializeTime(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	m := PlanManifest{
		Version: 1,
		Goal:    "g",
		Layers: []PlanLayer{
			{Layer: 0, Policy: "quorum:3", Nodes: []PlanNode{
				{ID: "n1", Template: planTestTemplateID, Instructions: "x"},
				{ID: "n2", Template: planTestTemplateID, Instructions: "y"},
			}},
		},
	}
	insertApprovedPlanBypassingService(t, pool, instanceID, m)

	_, err := svc.Materialize(instanceID)
	if err == nil {
		t.Fatal("Materialize with quorum:3 over 2 nodes: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "quorum") && !strings.Contains(err.Error(), "layer") {
		t.Errorf("expected error mentioning quorum or layer, got: %v", err)
	}
	if got := countInstanceNodes(t, pool, instanceID); got != 0 {
		t.Errorf("workflow_instance_nodes count = %d, want 0 (all-or-nothing)", got)
	}
}

// TestPlanMaterialize_ZeroNodeLayer_Rejected forces an approved head with an
// empty-nodes layer via direct SQL; Materialize's internal ValidatePlanManifest
// re-check (>=1 node per layer) must reject it and write zero rows.
func TestPlanMaterialize_ZeroNodeLayer_Rejected(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	m := PlanManifest{
		Version: 1,
		Goal:    "g",
		Layers:  []PlanLayer{{Layer: 0, Policy: "any", Nodes: nil}},
	}
	insertApprovedPlanBypassingService(t, pool, instanceID, m)

	if _, err := svc.Materialize(instanceID); err == nil {
		t.Fatal("Materialize with a zero-node layer: expected error, got nil")
	}
	if got := countInstanceNodes(t, pool, instanceID); got != 0 {
		t.Errorf("workflow_instance_nodes count = %d, want 0", got)
	}
}

// TestPlanMaterialize_ZeroLayerManifest_Rejected mirrors the zero-node-layer
// case one level up: no layers at all.
func TestPlanMaterialize_ZeroLayerManifest_Rejected(t *testing.T) {
	t.Parallel()
	pool, instanceID := setupPlanTestEnv(t)
	svc := NewPlanService(pool, clock.Real(), nil)

	m := PlanManifest{Version: 1, Goal: "g", Layers: nil}
	insertApprovedPlanBypassingService(t, pool, instanceID, m)

	if _, err := svc.Materialize(instanceID); err == nil {
		t.Fatal("Materialize with zero layers: expected error, got nil")
	}
	if got := countInstanceNodes(t, pool, instanceID); got != 0 {
		t.Errorf("workflow_instance_nodes count = %d, want 0", got)
	}
}

// TestPlanMaterialize_LayerOffsetAboveStaticExecutableLayers needs its own
// fixture: setupPlanTestEnv's project has no static agent_definitions (only
// the fanout_template), so maxStaticExecutableLayer is always -1 there. This
// test seeds two static defs (layers 0 and 1) plus a fanout_template, then
// asserts a manifest's Layer:0 materializes to engine layer 2 (= 1 + 1).
func TestPlanMaterialize_LayerOffsetAboveStaticExecutableLayers(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "plan_offset_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const (
		projectID  = "offset-proj"
		workflowID = "offset-wf"
		templateID = "offset-template"
		instanceID = "offset-wfi"
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Offset', '/tmp', ?, ?)`,
		projectID, now, now)
	mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		workflowID, projectID, now, now)
	mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, model, execution_mode, created_at, updated_at)
		 VALUES ('static-l0', ?, ?, 'p', 0, 'sonnet', 'cli_interactive', ?, ?)`, projectID, workflowID, now, now)
	mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, model, execution_mode, created_at, updated_at)
		 VALUES ('static-l1', ?, ?, 'p', 1, 'sonnet', 'cli_interactive', ?, ?)`, projectID, workflowID, now, now)
	mustExec(t, pool, `INSERT INTO agent_definitions (id, project_id, workflow_id, prompt, layer, model, execution_mode, node_role, created_at, updated_at)
		 VALUES (?, ?, ?, 'p', 0, 'sonnet', 'cli_interactive', 'fanout_template', ?, ?)`, templateID, projectID, workflowID, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'ticket', 'active', 0, ?, ?)`, instanceID, projectID, workflowID, now, now)

	svc := NewPlanService(pool, clock.Real(), nil)
	manifest := PlanManifest{
		Version: 1,
		Goal:    "goal",
		Layers: []PlanLayer{
			{Layer: 0, Policy: "all", Nodes: []PlanNode{{ID: "step1", Template: templateID, Instructions: "do it"}}},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	rev, err := svc.Revise(context.Background(), instanceID, types.PlanReviseRequest{Revision: 0, Manifest: raw})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if _, err := svc.Approve(instanceID, rev.Revision); err != nil {
		t.Fatalf("approve: %v", err)
	}

	nodes, err := repo.NewInstanceNodeRepo(pool, clock.Real()).List(instanceID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(nodes))
	}
	if nodes[0].Layer != 2 {
		t.Errorf("materialized node layer = %d, want 2 (maxStaticExecutableLayer=1 + 1)", nodes[0].Layer)
	}
}
