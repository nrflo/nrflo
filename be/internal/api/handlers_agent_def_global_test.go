package api

import (
	"net/http"
	"testing"
	"time"

	"be/internal/service"
)

// newGlobalAgentDefServer creates a Server seeded with a fanout_template
// agent_definitions row under the reserved __global__ project (mirroring the
// shape EnsureGlobalDynamicWorkflow seeds), for denyNonAdminGlobalWrite
// coverage on the agent-def routes.
func newGlobalAgentDefServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, _, _ := newAgentDefAPIModeServer(t, false)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		service.GlobalProjectID, "Global", "/tmp", now, now,
	); err != nil {
		t.Fatalf("insert __global__ project: %v", err)
	}
	if _, err := s.pool.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		service.GlobalProjectID, "dynamic", "", "project", now, now,
	); err != nil {
		t.Fatalf("insert __global__ workflow: %v", err)
	}
	if _, err := s.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, node_role, description, created_at, updated_at)
		 VALUES ('global-tpl', ?, 'dynamic', 'sonnet-5', 20, 'do work', 'cli_interactive', 'fanout_template', 'a template', ?, ?)`,
		service.GlobalProjectID, now, now,
	); err != nil {
		t.Fatalf("insert __global__ agent_definitions row: %v", err)
	}
	return s, service.GlobalProjectID
}

// TestHandleUpdateAgentDef_GlobalProject_NonAdmin_Forbidden extends the
// denyNonAdminGlobalWrite coverage (api/global_write_guard_test.go documents
// the guard's boundary; api/handlers_plan_test.go documents where it does
// NOT apply — plan revise/approve/cancel) to the agent-def PATCH route:
// mutating a __global__ agent_definitions row (e.g. one of the seeded
// `dynamic` workflow's fanout_template defs) without an admin user must 403,
// unlike a per-project agent def PATCH.
func TestHandleUpdateAgentDef_GlobalProject_NonAdmin_Forbidden(t *testing.T) {
	s, globalProject := newGlobalAgentDefServer(t)

	// No user in context at all (mirrors a bearer/service-token request,
	// which never populates userKey) -> 403.
	rr := patchAgentDefRequest(t, s, globalProject, "dynamic", "global-tpl", `{"timeout":45}`)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("PATCH __global__ agent def (no user) status = %d, want 403; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleUpdateAgentDef_ProjectScoped_NonAdmin_Allowed is the control
// case: the same PATCH against a normal (non-__global__) project succeeds
// with no user in context at all, proving denyNonAdminGlobalWrite is
// conditional on projectID=="__global__" and does not gate ordinary
// per-project agent-def CRUD.
func TestHandleUpdateAgentDef_ProjectScoped_NonAdmin_Allowed(t *testing.T) {
	s, pid, wid := newAgentDefAPIModeServer(t, false)
	if rr := postAgentDefRequest(t, s, pid, wid,
		`{"id":"proj-scoped-agent","prompt":"do stuff","execution_mode":"cli_interactive"}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup: create agent status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr := patchAgentDefRequest(t, s, pid, wid, "proj-scoped-agent", `{"timeout":45}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH project-scoped agent def (no user) status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
}
