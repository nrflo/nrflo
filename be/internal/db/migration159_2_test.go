package db

import (
	"testing"
)

// TestMigration159_WorkflowInstanceNodesCascadeDelete verifies that deleting
// a workflow_instances row cascades to both workflow_instance_nodes and
// workflow_instance_layer_policies.
func TestMigration159_WorkflowInstanceNodesCascadeDelete(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	seedProjectWorkflow(t, pool)
	seedInstance(t, pool, "wfi-node-cascade")

	if _, err := pool.Exec(`INSERT INTO workflow_instance_nodes (instance_id, node_id, layer, agent_type, instructions, plan_revision, created_at)
		VALUES ('wfi-node-cascade', 'node-1', 0, 'implementor', 'do the thing', 1, datetime('now'))`); err != nil {
		t.Fatalf("seed workflow_instance_nodes: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO workflow_instance_layer_policies (instance_id, layer, pass_policy)
		VALUES ('wfi-node-cascade', 0, 'all')`); err != nil {
		t.Fatalf("seed workflow_instance_layer_policies: %v", err)
	}

	if _, err := pool.Exec(`DELETE FROM workflow_instances WHERE id = 'wfi-node-cascade'`); err != nil {
		t.Fatalf("delete workflow_instances: %v", err)
	}

	var nodeCount, policyCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instance_nodes WHERE instance_id = 'wfi-node-cascade'`).Scan(&nodeCount); err != nil {
		t.Fatalf("count workflow_instance_nodes: %v", err)
	}
	if nodeCount != 0 {
		t.Errorf("workflow_instance_nodes rows after cascade delete = %d, want 0", nodeCount)
	}
	if err := pool.QueryRow(`SELECT COUNT(*) FROM workflow_instance_layer_policies WHERE instance_id = 'wfi-node-cascade'`).Scan(&policyCount); err != nil {
		t.Fatalf("count workflow_instance_layer_policies: %v", err)
	}
	if policyCount != 0 {
		t.Errorf("workflow_instance_layer_policies rows after cascade delete = %d, want 0", policyCount)
	}
}

// foreignKeyCheckRowCount runs PRAGMA foreign_key_check and returns the
// number of violation rows (0 means clean).
func foreignKeyCheckRowCount(t *testing.T, pool *Pool) int {
	t.Helper()
	rows, err := pool.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("PRAGMA foreign_key_check iterate: %v", err)
	}
	return count
}

// rebuiltWorkflowInstance mirrors every column of the rebuilt
// workflow_instances table for the full round-trip assertion below.
type rebuiltWorkflowInstance struct {
	id, projectID, ticketID, workflowID, scopeType, status string
	retryCount                                             int
	parentSession                                          string
	createdAt, updatedAt, skipTags                         string
	worktreePath, branchName                               string
	endlessLoop                                            int
	stopEndlessLoopAfterIteration                          int
	scheduledTaskID, externalID, externalContext           string
	purgeOnCompletion, launchDepth                         int
	parentInstanceID                                       string
	subworkflowDepth, subworkflowStarts                    int
}

// TestMigration159_WorkflowInstancesRebuildPreservesRowsAndFKIntegrity is the
// acceptance case for the 000159 workflow_instances rebuild: every one of the
// 23 columns must survive the DROP/RENAME dance with its exact value, and
// PRAGMA foreign_key_check must report zero violations both before and after
// inserting a fully-populated row (proving the re-declared FKs match reality,
// including the pre-existing system rows already in the template DB).
func TestMigration159_WorkflowInstancesRebuildPreservesRowsAndFKIntegrity(t *testing.T) {
	t.Parallel()
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if n := foreignKeyCheckRowCount(t, pool); n != 0 {
		t.Fatalf("foreign_key_check on freshly-copied template = %d violations, want 0", n)
	}

	seedProjectWorkflow(t, pool)
	if _, err := pool.Exec(`INSERT INTO scheduled_tasks (id, project_id, name, description, cron_expression, workflows, workflow_chain_ids, enabled, created_at, updated_at)
		VALUES ('sched1', 'p1', 'Test Task', '', '0 0 * * *', '[]', '[]', 1, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed scheduled_tasks: %v", err)
	}

	want := rebuiltWorkflowInstance{
		id:                            "wfi-full",
		projectID:                     "p1",
		ticketID:                      "TICKET-123",
		workflowID:                    "wf1",
		scopeType:                     "project",
		status:                        "planning",
		retryCount:                    3,
		parentSession:                 "sess-abc",
		createdAt:                     "2026-01-01T00:00:00Z",
		updatedAt:                     "2026-01-02T00:00:00Z",
		skipTags:                      `["tag1"]`,
		worktreePath:                  "/tmp/worktree",
		branchName:                    "feature/x",
		endlessLoop:                   1,
		stopEndlessLoopAfterIteration: 5,
		scheduledTaskID:               "sched1",
		externalID:                    "ext-123",
		externalContext:               "some external context",
		purgeOnCompletion:             1,
		launchDepth:                   2,
		parentInstanceID:              "parent-inst-1",
		subworkflowDepth:              3,
		subworkflowStarts:             4,
	}

	_, err = pool.Exec(`INSERT INTO workflow_instances (
		id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
		parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
		endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
		external_id, external_context, purge_on_completion, launch_depth,
		parent_instance_id, subworkflow_depth, subworkflow_starts
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		want.id, want.projectID, want.ticketID, want.workflowID, want.scopeType, want.status, want.retryCount,
		want.parentSession, want.createdAt, want.updatedAt, want.skipTags, want.worktreePath, want.branchName,
		want.endlessLoop, want.stopEndlessLoopAfterIteration, want.scheduledTaskID,
		want.externalID, want.externalContext, want.purgeOnCompletion, want.launchDepth,
		want.parentInstanceID, want.subworkflowDepth, want.subworkflowStarts,
	)
	if err != nil {
		t.Fatalf("insert fully-populated workflow_instances row: %v", err)
	}

	var got rebuiltWorkflowInstance
	row := pool.QueryRow(`SELECT
		id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
		parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
		endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
		external_id, external_context, purge_on_completion, launch_depth,
		parent_instance_id, subworkflow_depth, subworkflow_starts
		FROM workflow_instances WHERE id = ?`, want.id)
	if err := row.Scan(
		&got.id, &got.projectID, &got.ticketID, &got.workflowID, &got.scopeType, &got.status, &got.retryCount,
		&got.parentSession, &got.createdAt, &got.updatedAt, &got.skipTags, &got.worktreePath, &got.branchName,
		&got.endlessLoop, &got.stopEndlessLoopAfterIteration, &got.scheduledTaskID,
		&got.externalID, &got.externalContext, &got.purgeOnCompletion, &got.launchDepth,
		&got.parentInstanceID, &got.subworkflowDepth, &got.subworkflowStarts,
	); err != nil {
		t.Fatalf("scan rebuilt row: %v", err)
	}

	if got != want {
		t.Errorf("rebuilt workflow_instances row mismatch:\n got  = %+v\n want = %+v", got, want)
	}

	if n := foreignKeyCheckRowCount(t, pool); n != 0 {
		t.Errorf("foreign_key_check after fully-populated insert = %d violations, want 0", n)
	}
}
