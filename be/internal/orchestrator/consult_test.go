package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// consultMockScripts returns two scripted turns: turn 1 writes _consult_answer,
// turn 2 calls agent_finished.
func consultMockScripts(answer string) []mock.Script {
	raw, _ := json.Marshal(answer)
	addInput := json.RawMessage(`{"key":"_consult_answer","value":` + string(raw) + `}`)
	return []mock.Script{
		{
			Events: []mock.SinkEvent{
				{Kind: mock.EventToolUseStart, ToolUseID: "tu-1", ToolName: "findings_add"},
				{Kind: mock.EventToolUseStop, ToolUseID: "tu-1", FullInput: addInput},
			},
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolUseID: "tu-1", ToolName: "findings_add", Input: addInput},
				},
			},
		},
		{
			Events: []mock.SinkEvent{
				{Kind: mock.EventToolUseStart, ToolUseID: "tu-2", ToolName: "agent_finished"},
				{Kind: mock.EventToolUseStop, ToolUseID: "tu-2", FullInput: json.RawMessage(`{}`)},
			},
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolUseID: "tu-2", ToolName: "agent_finished", Input: json.RawMessage(`{}`)},
				},
			},
		},
	}
}

// seedConsultDefs inserts a running caller agent_session (non-consultant) and
// a consultant agent_definition under the "test" workflow.
// Returns the caller session ID and the workflow instance ID.
func seedConsultDefs(t *testing.T, env *testEnv, ticketID string) (callerSID, wfiID string) {
	t.Helper()

	env.createTicket(t, ticketID, "Consult Test")
	wfiID = env.initWorkflow(t, ticketID)

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Caller session: non-consultant type ("implementor") so recursion guard skips.
	callerSID = "caller-consult-" + ticketID
	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	if err := asRepo.Create(&model.AgentSession{
		ID:                 callerSID,
		ProjectID:          env.project,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              "implementor",
		AgentType:          "implementor",
		ModelID:            sql.NullString{String: "claude:opus-4-7", Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create caller session: %v", err)
	}

	// Consultant def: consultant=1, execution_mode=api, in "test" workflow.
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, created_at, updated_at)
		 VALUES ('test-consultant', ?, 'test', 'sonnet-5', 30, '# Answer: ${CONSULT_QUESTION}', 'api', 'findings_add,agent_finished', 0, 1, ?, ?)`,
		env.project, now, now,
	); err != nil {
		t.Fatalf("insert consultant def: %v", err)
	}

	return callerSID, wfiID
}

// TestConsult_UnknownCallerSession verifies that Consult returns an error when
// the caller session ID does not exist in the database.
func TestConsult_UnknownCallerSession(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.orch.Consult(context.Background(), "nonexistent-session-xyz", "test-consultant", "q?")
	if err == nil {
		t.Fatal("Consult() = nil error; want error for unknown caller session")
	}
	if !strings.Contains(err.Error(), "unknown caller session") {
		t.Errorf("error = %q, want contains 'unknown caller session'", err.Error())
	}
}

// TestConsult_RecursionGuard verifies that Consult returns an error when the
// calling agent's own definition has consultant=true (socket-boundary guard).
func TestConsult_RecursionGuard(t *testing.T) {
	env := newTestEnv(t)

	const ticketID = "rg-ticket-001"
	env.createTicket(t, ticketID, "Recursion Guard Test")
	wfiID := env.initWorkflow(t, ticketID)

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insert a consultant def for the CALLER agent type.
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, layer, consultant, created_at, updated_at)
		 VALUES ('doc-consultant', ?, 'test', 'sonnet-5', 30, '# answer', 'api', 0, 1, ?, ?)`,
		env.project, now, now,
	); err != nil {
		t.Fatalf("insert caller consultant def: %v", err)
	}

	// Caller session whose AgentType matches the consultant def above.
	callerSID := "caller-rg-001"
	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	if err := asRepo.Create(&model.AgentSession{
		ID:                 callerSID,
		ProjectID:          env.project,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              "doc-consultant",
		AgentType:          "doc-consultant",
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create caller session: %v", err)
	}

	_, err := env.orch.Consult(context.Background(), callerSID, "some-consultant", "how?")
	if err == nil {
		t.Fatal("Consult() = nil error; want recursion guard error")
	}
	if !strings.Contains(err.Error(), "recursion guard") {
		t.Errorf("error = %q, want contains 'recursion guard'", err.Error())
	}
}

// TestConsult_TargetNotConsultant verifies that Consult propagates the spawner
// error when the target agent definition has consultant=false.
func TestConsult_TargetNotConsultant(t *testing.T) {
	env := newTestEnv(t)
	callerSID, _ := seedConsultDefs(t, env, "nc-ticket-001")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, layer, consultant, created_at, updated_at)
		 VALUES ('normal-agent', ?, 'test', 'sonnet-5', 30, '# impl', 'api', 0, 0, ?, ?)`,
		env.project, now, now,
	); err != nil {
		t.Fatalf("insert non-consultant def: %v", err)
	}

	_, err := env.orch.Consult(context.Background(), callerSID, "normal-agent", "q?")
	if err == nil {
		t.Fatal("Consult() = nil error; want error for non-consultant target")
	}
	if !strings.Contains(err.Error(), "not flagged as a consultant") {
		t.Errorf("error = %q, want contains 'not flagged as a consultant'", err.Error())
	}
}

// TestConsult_TargetNotAPIMode verifies that Consult propagates the spawner
// error when the target consultant has execution_mode != api.
func TestConsult_TargetNotAPIMode(t *testing.T) {
	env := newTestEnv(t)
	callerSID, _ := seedConsultDefs(t, env, "na-ticket-001")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, execution_mode, layer, consultant, created_at, updated_at)
		 VALUES ('cli-consultant', ?, 'test', 'sonnet-5', 30, '# impl', 'cli_interactive', 0, 1, ?, ?)`,
		env.project, now, now,
	); err != nil {
		t.Fatalf("insert cli consultant def: %v", err)
	}

	_, err := env.orch.Consult(context.Background(), callerSID, "cli-consultant", "q?")
	if err == nil {
		t.Fatal("Consult() = nil error; want error for non-api-mode consultant")
	}
	if !strings.Contains(err.Error(), "execution_mode=api") {
		t.Errorf("error = %q, want contains 'execution_mode=api'", err.Error())
	}
}

// TestConsult_HappyPath exercises the full consult flow with a mock API provider.
// The consultant writes _consult_answer and calls agent_finished; Consult must
// return the answer and delete the _consult_answer finding.
func TestConsult_HappyPath(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := newTestEnv(t)
	callerSID, _ := seedConsultDefs(t, env, "hp-ticket-001")

	// Inject mock provider via the test seam.
	orig := consultBuildAPIProvider
	consultBuildAPIProvider = func(_ context.Context, _ *db.Pool, _ clock.Clock, _, _ string) (provider.Provider, error) {
		return mock.New(consultMockScripts("the answer")...), nil
	}
	t.Cleanup(func() { consultBuildAPIProvider = orig })

	answer, err := env.orch.Consult(context.Background(), callerSID, "test-consultant", "how?")
	if err != nil {
		t.Fatalf("Consult() error: %v", err)
	}
	if answer != "the answer" {
		t.Errorf("answer = %q, want %q", answer, "the answer")
	}

	// _consult_answer finding must have been deleted by the time Consult returns.
	var count int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE key='_consult_answer'`).Scan(&count); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if count != 0 {
		t.Errorf("_consult_answer count = %d, want 0 (must be deleted after round-trip)", count)
	}
}
