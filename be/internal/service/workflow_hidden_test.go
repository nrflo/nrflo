package service

import (
	"testing"
	"time"
)

func TestIsHiddenWorkflowName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want bool
	}{
		{"_delegate_host", true},
		{"__spec_import__", true},
		{"_x", true},
		{"dynamic", false},
		{"feature", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsHiddenWorkflowName(tc.name); got != tc.want {
			t.Errorf("IsHiddenWorkflowName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestListProjectWorkflowInstances_ExcludesHidden verifies _delegate_host and
// __spec_import__ instances never surface from ListProjectWorkflowInstances,
// while a normal project instance still does.
func TestListProjectWorkflowInstances_ExcludesHidden(t *testing.T) {
	t.Parallel()
	pool, svc := setupWorkflowDefsTestEnv(t)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, wf := range []string{"wf-visible-hidden-test", "_delegate_host", "__spec_import__"} {
		mustExec(t, pool, `INSERT INTO workflows (id, project_id, description, scope_type, created_at, updated_at)
			 VALUES (?, 'proj1', '', 'project', ?, ?)`, wf, now, now)
	}
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES ('wfi-visible', 'proj1', '', 'wf-visible-hidden-test', 'project', 'active', 0, ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES ('wfi-delegate', 'proj1', '', '_delegate_host', 'project', 'active', 0, ?, ?)`, now, now)
	mustExec(t, pool, `INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES ('wfi-spec', 'proj1', '', '__spec_import__', 'project', 'active', 0, ?, ?)`, now, now)

	instances, err := svc.ListProjectWorkflowInstances("proj1")
	if err != nil {
		t.Fatalf("ListProjectWorkflowInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances len = %d, want 1; got %+v", len(instances), instances)
	}
	if instances[0].ID != "wfi-visible" {
		t.Errorf("instances[0].ID = %q, want wfi-visible", instances[0].ID)
	}
}
