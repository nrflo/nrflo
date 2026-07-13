package spawner

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
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// consultTestEnv extends the base context-save test env with caller session and
// a consultant agent_definition row.
type consultTestEnv struct {
	*contextSaveTestEnv
	pool            *db.Pool
	callerSessionID string
}

func setupConsultTestEnv(t *testing.T) *consultTestEnv {
	t.Helper()
	base := setupContextSaveTestEnv(t)
	pool := db.WrapAsPool(base.database)

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insert running caller agent_session.
	callerSID := "caller-session-consult"
	if err := repo.NewAgentSessionRepo(base.database, clock.Real()).Create(&model.AgentSession{
		ID:                 callerSID,
		ProjectID:          base.projectID,
		TicketID:           base.ticketID,
		WorkflowInstanceID: base.wfiID,
		Phase:              "implementor",
		AgentType:          "implementor",
		ModelID:            sql.NullString{String: "claude:opus_4_7", Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create caller session: %v", err)
	}

	// Insert consultant agent_definition (consultant=1, execution_mode=api).
	if _, err := base.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, tools, layer, consultant, created_at, updated_at)
		VALUES (?, ?, 'feature', 'sonnet', 30, '# Answer: ${CONSULT_QUESTION}', 'api', 'findings_add,agent_finished', 0, 1, ?, ?)`,
		"consultant", base.projectID, now, now,
	); err != nil {
		t.Fatalf("insert consultant agent_def: %v", err)
	}

	return &consultTestEnv{contextSaveTestEnv: base, pool: pool, callerSessionID: callerSID}
}

func buildConsultSpawner(t *testing.T, env *consultTestEnv, prov provider.Provider) *Spawner {
	t.Helper()
	clk := clock.Real()
	return New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clk,
		WSHub:    ws.NewHub(clk),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return prov, nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 200000},
		},
		AgentSvc:           &noopAgentSvc{},
		FindingsSvc:        service.NewFindingsService(env.pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(env.pool, clk),
		AgentSvcReal:       service.NewAgentService(env.pool, clk),
		WorkflowSvc:        service.NewWorkflowService(env.pool, clk),
	})
}

// consultMockScripts returns a pair of mock scripts: turn 1 writes _consult_answer,
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

func TestConsult_HappyPath_WritesAnswerAndCleansUp(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupConsultTestEnv(t)
	defer env.cleanup()

	sp := buildConsultSpawner(t, env, mock.New(consultMockScripts("the answer")...))

	answer, err := sp.Consult(context.Background(), env.callerSessionID, "consultant", "how?")
	if err != nil {
		t.Fatalf("Consult() error: %v", err)
	}
	if answer != "the answer" {
		t.Errorf("answer = %q, want %q", answer, "the answer")
	}

	// _consult_answer finding must be deleted after Consult returns.
	var count int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE key='_consult_answer'`).Scan(&count); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if count != 0 {
		t.Errorf("_consult_answer finding count = %d, want 0 (must be deleted)", count)
	}
}

func TestConsult_MissingDef_ReturnsError(t *testing.T) {
	env := setupConsultTestEnv(t)
	defer env.cleanup()

	sp := buildConsultSpawner(t, env, mock.New())

	_, err := sp.Consult(context.Background(), env.callerSessionID, "nonexistent-agent", "q")
	if err == nil {
		t.Fatal("Consult() returned nil error; want error for missing def")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want contains 'not found'", err.Error())
	}
}

func TestConsult_NotFlaggedAsConsultant_ReturnsError(t *testing.T) {
	env := setupConsultTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, layer, consultant, created_at, updated_at)
		VALUES ('normal-agent', ?, 'feature', 'sonnet', 30, '# Implement', 'api', 0, 0, ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert normal agent def: %v", err)
	}

	sp := buildConsultSpawner(t, env, mock.New())

	_, err := sp.Consult(context.Background(), env.callerSessionID, "normal-agent", "q")
	if err == nil {
		t.Fatal("Consult() returned nil error; want error for non-consultant def")
	}
	if !strings.Contains(err.Error(), "not flagged as a consultant") {
		t.Errorf("error = %q, want contains 'not flagged as a consultant'", err.Error())
	}
}

func TestConsult_NonAPIMode_ReturnsError(t *testing.T) {
	env := setupConsultTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, execution_mode, layer, consultant, created_at, updated_at)
		VALUES ('cli-consultant', ?, 'feature', 'sonnet', 30, '# Impl', 'cli_interactive', 0, 1, ?, ?)`,
		env.projectID, now, now,
	); err != nil {
		t.Fatalf("insert cli-interactive consultant def: %v", err)
	}

	sp := buildConsultSpawner(t, env, mock.New())

	_, err := sp.Consult(context.Background(), env.callerSessionID, "cli-consultant", "q")
	if err == nil {
		t.Fatal("Consult() returned nil error; want error for non-api execution_mode")
	}
	if !strings.Contains(err.Error(), "execution_mode=api") {
		t.Errorf("error = %q, want contains 'execution_mode=api'", err.Error())
	}
}

func TestConsult_NoAnswerWritten_ReturnsError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupConsultTestEnv(t)
	defer env.cleanup()

	// Consultant calls agent_finished without writing _consult_answer first.
	noAnswerScript := mock.Script{
		Events: []mock.SinkEvent{
			{Kind: mock.EventToolUseStart, ToolUseID: "tu-1", ToolName: "agent_finished"},
			{Kind: mock.EventToolUseStop, ToolUseID: "tu-1", FullInput: json.RawMessage(`{}`)},
		},
		Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content: []provider.ContentBlock{
				{Type: "tool_use", ToolUseID: "tu-1", ToolName: "agent_finished", Input: json.RawMessage(`{}`)},
			},
		},
	}

	sp := buildConsultSpawner(t, env, mock.New(noAnswerScript))

	_, err := sp.Consult(context.Background(), env.callerSessionID, "consultant", "q")
	if err == nil {
		t.Fatal("Consult() returned nil error; want error when _consult_answer not written")
	}
	if !strings.Contains(err.Error(), "_consult_answer") {
		t.Errorf("error = %q, want contains '_consult_answer'", err.Error())
	}
}

// TestConsultRecursionGuard verifies that prepareSpawn strips the "consult" tool
// from handlers and specs when the agent_definition has consultant=1.
func TestConsultRecursionGuard_ConsultExcludedForConsultantAgent(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupConsultTestEnv(t)
	defer env.cleanup()

	// Give the consultant def "consult" in its tools list.
	if _, err := env.database.Exec(
		`UPDATE agent_definitions SET tools='consult,findings_add,agent_finished' WHERE id='consultant' AND project_id=?`,
		env.projectID,
	); err != nil {
		t.Fatalf("update tools: %v", err)
	}

	// Build a spawner mirroring the child spawner inside Consult.
	clk := clock.Real()
	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clk,
		WSHub:    ws.NewHub(clk),
		APIMode:  true,
		BuildAPIProvider: func(_ context.Context, _, _ string) (provider.Provider, error) {
			return mock.New(), nil
		},
		APIModelConfigs: map[string]APIModelConfig{
			"sonnet": {Provider: "anthropic", MappedModel: "claude-sonnet-4-6", ContextLength: 200000},
		},
		AgentSvc: &noopAgentSvc{},
		Workflows: map[string]WorkflowDef{
			"feature": {
				Phases: []PhaseDef{{NodeID: "_consult", Agent: "consultant", Layer: 0}},
			},
		},
	})

	_, prep, err := sp.prepareSpawn(context.Background(), SpawnRequest{
		AgentType:          "consultant",
		ProjectID:          env.projectID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	}, "claude:sonnet", "_consult", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn: %v", err)
	}

	if _, ok := prep.apiHandlers["consult"]; ok {
		t.Error("consult handler must be excluded from consultant's registry (recursion guard)")
	}
	for _, spec := range prep.apiTools {
		if spec.Name == "consult" {
			t.Error("consult tool spec must be excluded from consultant's registry (recursion guard)")
		}
	}
}
