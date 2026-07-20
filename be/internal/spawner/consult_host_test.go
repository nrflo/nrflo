package spawner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"be/internal/spawner/apirun/provider/mock"
)

// seedActiveProjectWorkflowInstance inserts a project-scoped active
// workflow_instances row — ConsultHost's ScopeType="project" spawn requires
// one to already exist (getProjectWorkflowInstance), unlike the session-bound
// Consult path, which spawns under the caller's own ticket-scoped instance.
func seedActiveProjectWorkflowInstance(t *testing.T, env *consultTestEnv, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO workflow_instances
			(id, project_id, def_project_id, ticket_id, workflow_id, scope_type, status, created_at, updated_at)
		VALUES (?, ?, ?, '', ?, 'project', 'active', ?, ?)`,
		uuid.New().String(), projectID, projectID, workflowID, now, now,
	); err != nil {
		t.Fatalf("seed project workflow instance: %v", err)
	}
}

// TestConsultHost_HappyPath_WritesAnswer mirrors TestConsult_HappyPath but
// drives the hidden-host path (ConsultHost), which has no caller session and
// resolves the consultant via repo.AgentDefinitionRepo.FindConsultant instead
// of a caller-known workflow.
func TestConsultHost_HappyPath_WritesAnswer(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupConsultTestEnv(t)
	defer env.cleanup()
	seedActiveProjectWorkflowInstance(t, env, env.projectID, "feature")

	sp := buildConsultSpawner(t, env, mock.New(consultMockScripts("the host answer")...))

	answer, err := sp.ConsultHost(context.Background(), env.projectID, "consultant", "how?")
	if err != nil {
		t.Fatalf("ConsultHost() error: %v", err)
	}
	if answer != "the host answer" {
		t.Errorf("answer = %q, want %q", answer, "the host answer")
	}
}

// TestConsultHost_UnknownConsultantID_ReturnsError verifies FindConsultant's
// not-found error propagates through ConsultHost.
func TestConsultHost_UnknownConsultantID_ReturnsError(t *testing.T) {
	env := setupConsultTestEnv(t)
	defer env.cleanup()

	sp := buildConsultSpawner(t, env, mock.New())

	_, err := sp.ConsultHost(context.Background(), env.projectID, "no-such-consultant", "q")
	if err == nil {
		t.Fatal("ConsultHost() returned nil error; want error for unresolved consultant id")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want contains 'not found'", err.Error())
	}
}

// TestConsultHost_GlobalOnlyDef_FindsButFailsToSpawn documents a confirmed
// production bug (see be_production_bugs / consult_host.go + consult_run.go):
// repo.FindConsultant deliberately falls back to a '__global__'-project
// agent_definitions row when the caller's own project has none (mirroring
// orchestrator.resolveWorkflowDef's definitionProjectID pattern for global
// workflows) — but runConsult's spawn never receives that resolved
// definition project. It always passes the caller's own projectID as
// req.ProjectID, and the spawned sub-Spawner's loadPromptContent/
// loadAgentDefinition re-look-up the prompt scoped to req.ProjectID alone
// (with no '__global__' fallback of its own, unlike loadPromptContent's
// separate system_agent_definitions fallback). So a consultant that exists
// ONLY as a '__global__' agent_definitions row (not also registered in
// system_agent_definitions) is successfully *found* by FindConsultant but
// then fails to *spawn* with a confusing "agent definition not found" error.
// This test pins that (undesired) behavior so a fix is a visible, intentional
// test change rather than a silent flip. Do not "fix" this test without
// fixing runConsult/ConsultHost itself (e.g. by spawning under def.ProjectID
// or by loading the prompt from req.Def directly instead of re-querying by
// req.ProjectID).
func TestConsultHost_GlobalOnlyDef_FindsButFailsToSpawn(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupConsultTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('__global__', 'Global', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert __global__ project: %v", err)
	}
	if _, err := env.database.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		VALUES ('__global__', 'feature', '', 'ticket', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert __global__ workflow: %v", err)
	}
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, created_at, updated_at)
		VALUES ('global-consultant', '__global__', 'feature', 'sonnet-5', 30, '# Answer: ${CONSULT_QUESTION}', 'api', 'findings_add,agent_finished', 0, 1, ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert global consultant agent_def: %v", err)
	}
	// The spawn scopes to the caller's own project (env.projectID), not
	// '__global__' — only the consultant definition itself is looked up in
	// the global namespace by FindConsultant.
	seedActiveProjectWorkflowInstance(t, env, env.projectID, "feature")

	sp := buildConsultSpawner(t, env, mock.New(consultMockScripts("the global answer")...))

	_, err := sp.ConsultHost(context.Background(), env.projectID, "global-consultant", "how?")
	if err == nil {
		t.Fatal("ConsultHost: got nil error, want the known template-resolution failure (see be_production_bugs) — if this now passes, replace this test with the intended happy-path assertions")
	}
	if !strings.Contains(err.Error(), "agent definition not found") {
		t.Errorf("ConsultHost error = %v, want the loadPromptContent not-found mismatch", err)
	}
}
