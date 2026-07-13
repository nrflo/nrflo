package repo

import (
	"errors"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
)

// seedPlanInstance inserts the minimal projects/workflows/workflow_instances
// rows needed to satisfy plan_revisions/workflow_plans FKs for instanceID.
func seedPlanInstance(t *testing.T, pool *db.Pool, instanceID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := "proj-" + instanceID
	workflowID := "wf-" + instanceID
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		projectID, now, now); err != nil {
		t.Fatalf("insert project %s: %v", projectID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'ticket', ?, ?)`,
		workflowID, projectID, now, now); err != nil {
		t.Fatalf("insert workflow %s: %v", workflowID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		 VALUES (?, ?, '', ?, 'active', 'ticket', ?, ?)`,
		instanceID, projectID, workflowID, now, now); err != nil {
		t.Fatalf("insert workflow instance %s: %v", instanceID, err)
	}
}

func TestPlanRepo_Append_ImmutableRevisions(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-append"
	seedPlanInstance(t, pool, instanceID)
	repo := NewPlanRepo(pool, clock.Real())

	rev1, err := repo.Append(instanceID, "manifest-v1", "hash1", model.PlanAuthorPlanner, "sess-1", "goal1")
	if err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if rev1 != 1 {
		t.Fatalf("first Append revision = %d, want 1", rev1)
	}

	rev2, err := repo.Append(instanceID, "manifest-v2", "hash2", model.PlanAuthorPlanner, "sess-2", "goal2")
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if rev2 != 2 {
		t.Fatalf("second Append revision = %d, want 2", rev2)
	}

	revisions, err := repo.ListRevisions(instanceID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("len(revisions) = %d, want 2", len(revisions))
	}
	if revisions[0].Revision != 1 || revisions[0].Manifest != "manifest-v1" {
		t.Errorf("revisions[0] = %+v, want revision 1 manifest-v1", revisions[0])
	}
	if revisions[1].Revision != 2 || revisions[1].Manifest != "manifest-v2" {
		t.Errorf("revisions[1] = %+v, want revision 2 manifest-v2", revisions[1])
	}

	// The first row must still be untouched after the second Append landed.
	first, err := repo.GetRevision(instanceID, 1)
	if err != nil {
		t.Fatalf("GetRevision(1): %v", err)
	}
	if first.Manifest != "manifest-v1" {
		t.Errorf("GetRevision(1).Manifest = %q, want %q (overwritten!)", first.Manifest, "manifest-v1")
	}
}

func TestPlanRepo_Approve_StaleRevisionRejected(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-stale"
	seedPlanInstance(t, pool, instanceID)
	repo := NewPlanRepo(pool, clock.Real())

	if _, err := repo.Append(instanceID, "manifest-v1", "hash1", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append rev1: %v", err)
	}
	if _, err := repo.Append(instanceID, "manifest-v2", "hash2", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append rev2: %v", err)
	}

	err := repo.Approve(instanceID, 1)
	if !errors.Is(err, ErrPlanStaleOrNotDraft) {
		t.Fatalf("Approve(1) with head at rev2 err = %v, want ErrPlanStaleOrNotDraft", err)
	}

	head, err := repo.GetHead(instanceID)
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head.Status != model.PlanStatusDraft {
		t.Errorf("head.Status = %q, want %q (unchanged)", head.Status, model.PlanStatusDraft)
	}
	if head.LatestRevision != 2 {
		t.Errorf("head.LatestRevision = %d, want 2", head.LatestRevision)
	}
}

func TestPlanRepo_Approve_ExactHeadRevisionSucceeds(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-approve-ok"
	seedPlanInstance(t, pool, instanceID)
	repo := NewPlanRepo(pool, clock.Real())

	if _, err := repo.Append(instanceID, "manifest-v1", "hash1", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := repo.Approve(instanceID, 1); err != nil {
		t.Fatalf("Approve(1): %v", err)
	}

	head, err := repo.GetHead(instanceID)
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head.Status != model.PlanStatusApproved {
		t.Errorf("head.Status = %q, want %q", head.Status, model.PlanStatusApproved)
	}
	if head.ApprovedRevision != 1 {
		t.Errorf("head.ApprovedRevision = %d, want 1", head.ApprovedRevision)
	}
}

func TestPlanRepo_Approve_AlreadyApprovedRejected(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	instanceID := "inst-reapprove"
	seedPlanInstance(t, pool, instanceID)
	repo := NewPlanRepo(pool, clock.Real())

	if _, err := repo.Append(instanceID, "manifest-v1", "hash1", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := repo.Approve(instanceID, 1); err != nil {
		t.Fatalf("first Approve(1): %v", err)
	}

	err := repo.Approve(instanceID, 1)
	if !errors.Is(err, ErrPlanStaleOrNotDraft) {
		t.Fatalf("second Approve(1) err = %v, want ErrPlanStaleOrNotDraft (re-approve not idempotent)", err)
	}
}

func TestPlanRepo_Cancel_NoopOnMissingHead(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	repo := NewPlanRepo(pool, clock.Real())

	if err := repo.Cancel("no-such-instance"); err != nil {
		t.Fatalf("Cancel on missing head returned error: %v", err)
	}
}

func TestPlanRepo_Cancel_DraftToCancelled_ApprovedIsNoop(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	repo := NewPlanRepo(pool, clock.Real())

	// Draft head: Cancel flips it to cancelled.
	draftID := "inst-cancel-draft"
	seedPlanInstance(t, pool, draftID)
	if _, err := repo.Append(draftID, "manifest", "hash", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append(draft): %v", err)
	}
	if err := repo.Cancel(draftID); err != nil {
		t.Fatalf("Cancel(draft): %v", err)
	}
	draftHead, err := repo.GetHead(draftID)
	if err != nil {
		t.Fatalf("GetHead(draft): %v", err)
	}
	if draftHead.Status != model.PlanStatusCancelled {
		t.Errorf("draft head.Status = %q, want %q", draftHead.Status, model.PlanStatusCancelled)
	}

	// Approved head: Cancel must not un-approve it.
	approvedID := "inst-cancel-approved"
	seedPlanInstance(t, pool, approvedID)
	if _, err := repo.Append(approvedID, "manifest", "hash", model.PlanAuthorPlanner, "", "goal"); err != nil {
		t.Fatalf("Append(approved): %v", err)
	}
	if err := repo.Approve(approvedID, 1); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := repo.Cancel(approvedID); err != nil {
		t.Fatalf("Cancel(approved): %v", err)
	}
	approvedHead, err := repo.GetHead(approvedID)
	if err != nil {
		t.Fatalf("GetHead(approved): %v", err)
	}
	if approvedHead.Status != model.PlanStatusApproved {
		t.Errorf("approved head.Status = %q, want %q (Cancel must not un-approve)", approvedHead.Status, model.PlanStatusApproved)
	}
}

func TestPlanRepo_ListExpiredDrafts(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	repo := NewPlanRepo(pool, clock.Real())

	expiredID := "inst-expired"
	approvedID := "inst-not-draft"
	freshID := "inst-fresh"
	for _, id := range []string{expiredID, approvedID, freshID} {
		seedPlanInstance(t, pool, id)
		if _, err := repo.Append(id, "manifest", "hash", model.PlanAuthorPlanner, "", "goal"); err != nil {
			t.Fatalf("Append(%s): %v", id, err)
		}
	}

	if err := repo.Approve(approvedID, 1); err != nil {
		t.Fatalf("Approve(%s): %v", approvedID, err)
	}

	now := time.Now().UTC()
	backdated := now.Add(-1 * time.Hour).Format(time.RFC3339Nano)
	cutoff := now.Add(-30 * time.Minute).Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE workflow_plans SET updated_at = ? WHERE instance_id = ?`, backdated, expiredID); err != nil {
		t.Fatalf("backdate %s: %v", expiredID, err)
	}

	ids, err := repo.ListExpiredDrafts(cutoff)
	if err != nil {
		t.Fatalf("ListExpiredDrafts: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("ListExpiredDrafts returned %v, want exactly [%s]", ids, expiredID)
	}
	if ids[0] != expiredID {
		t.Errorf("ListExpiredDrafts returned %q, want %q", ids[0], expiredID)
	}
}
