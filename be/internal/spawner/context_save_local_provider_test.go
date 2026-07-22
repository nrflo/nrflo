package spawner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// registerLocalCtxSaverProvider wires up a custom_providers row pointed at
// the stub, repoints context-saver-api's model at a ModelConfigs slug naming
// that row, and returns the ModelConfigs map + BuildAPIProvider closure the
// one-off context-saver spawn needs to resolve service.BuildAPIProvider ->
// custom.New -> openaichat against it.
func registerLocalCtxSaverProvider(t *testing.T, env *contextSaveTestEnv, clk clock.Clock, stubURL string) (map[string]ModelConfig, func(context.Context, string, string) (provider.Provider, error)) {
	t.Helper()
	pool := env.spawner.config.Pool
	cpSvc := service.NewCustomProviderService(pool, clk)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name:    "local-ollama-ctxsave",
		BaseURL: stubURL,
		APIWire: "chat_completions",
	}); err != nil {
		t.Fatalf("create custom provider: %v", err)
	}
	if _, err := env.database.Exec(
		`UPDATE system_agent_definitions SET model = ? WHERE id = 'context-saver-api'`,
		"local-ollama-ctxsave-model",
	); err != nil {
		t.Fatalf("repoint context-saver-api model: %v", err)
	}
	modelConfigs := map[string]ModelConfig{
		"local-ollama-ctxsave-model": {
			Provider:      "local-ollama-ctxsave",
			APIModel:      "qwen-test",
			APIContext:    128000,
			APIEfforts:    []string{"low", "medium", "high"},
			DefaultEffort: "low",
		},
	}
	buildProvider := func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
		return service.BuildAPIProvider(ctx, pool, clk, providerName, projectID)
	}
	return modelConfigs, buildProvider
}

// TestSpawnContextSaver_LocalProviderStub_APIModePropagated verifies the
// regression fix for a production defect found while verifying the
// local-provider path: spawnContextSaver's one-off Spawner
// (be/internal/spawner/context_save.go) previously never set APIMode on the
// Config it constructs — unlike its siblings delegate.go and consult_run.go,
// which both explicitly set APIMode: true for their equivalent one-off
// spawners. That meant execution_mode="api" context-saver spawns were always
// rejected at the prepareSpawn gate with "api_mode_disabled", regardless of a
// fully working custom provider. context_save.go now sets APIMode: true; this
// test asserts the spawn succeeds and actually reaches the stub.
func TestSpawnContextSaver_LocalProviderStub_APIModePropagated(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "implementing feature X")

	addInput, _ := json.Marshal(map[string]string{"key": "to_resume", "value": "progress summary"})
	stub := newLocalProviderStub(t, []string{
		sseToolCallScript("tu-1", "findings_add", addInput),
		sseTextScript("Saved."),
	})

	clk := clock.Real()
	modelConfigs, buildProvider := registerLocalCtxSaverProvider(t, env, clk, stub.URL())

	env.spawner.config.ModelConfigs = modelConfigs
	env.spawner.config.BuildAPIProvider = buildProvider
	env.spawner.config.AgentSvc = &noopAgentSvc{}
	env.spawner.config.FindingsSvc = service.NewFindingsService(env.spawner.config.Pool, clk)
	env.spawner.config.ProjectFindingsSvc = service.NewProjectFindingsService(env.spawner.config.Pool, clk)
	env.spawner.config.AgentSvcReal = service.NewAgentService(env.spawner.config.Pool, clk)
	env.spawner.config.WorkflowSvc = service.NewWorkflowService(env.spawner.config.Pool, clk)
	env.spawner.config.WSHub = ws.NewHub(clk)

	proc := &processInfo{
		sessionID: sessionID,
		agentType: "implementor",
		backend:   &mockBackend{name: "api"},
	}

	got := env.spawner.spawnContextSaver(env.ctx, proc, SpawnRequest{
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowName:       "feature",
		WorkflowInstanceID: env.wfiID,
	})
	if !got {
		t.Fatal("spawnContextSaver() = false; want true now that context_save.go forwards APIMode")
	}
	if stub.requestCount() == 0 {
		t.Error("stub requestCount() = 0, want > 0 (spawn should have reached the provider)")
	}
}

// TestContextSaverAPI_LocalProviderStub_WritesToResumeFinding verifies the
// context-saver-api system agent (selected by GetForBackend("context-saver",
// "api") — see spawn_context_saver_backend_test.go) actually writes a
// to_resume finding when driven through service.BuildAPIProvider ->
// custom.New -> openaichat against a localhost stub standing in for Ollama.
// It reproduces spawnContextSaver's own one-off-spawner construction
// (context_save.go) field-for-field but adds the APIMode: true that function
// currently omits (see the KnownBug test above) — isolating the local-provider
// wiring itself (registry resolution + real HTTP wire) from that unrelated
// defect.
func TestContextSaverAPI_LocalProviderStub_WritesToResumeFinding(t *testing.T) {
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	env.insertAgentMessage(t, sessionID, "implementing feature X: created foo.go, wired it into bar.go, tests pass")

	addInput, _ := json.Marshal(map[string]string{
		"key":   "to_resume",
		"value": "Implemented foo.go and wired it into bar.go; tests pass. Remaining: docs.",
	})
	stub := newLocalProviderStub(t, []string{
		sseToolCallScript("tu-1", "findings_add", addInput),
		sseTextScript("Saved."),
	})

	clk := clock.Real()
	modelConfigs, buildProvider := registerLocalCtxSaverProvider(t, env, clk, stub.URL())

	pool := env.spawner.config.Pool
	sysDef, err := service.NewSystemAgentDefinitionService(pool, clk, service.NewModelService(pool, clk)).
		GetForBackend("context-saver", "api")
	if err != nil {
		t.Fatalf("GetForBackend(context-saver, api): %v", err)
	}
	if sysDef.Model != "local-ollama-ctxsave-model" {
		t.Fatalf("sysDef.Model = %q, want local-ollama-ctxsave-model (repoint did not take)", sysDef.Model)
	}

	msgRepo := repo.NewAgentMessageRepo(env.database, clk)
	messages, err := msgRepo.GetBySession(sessionID)
	if err != nil {
		t.Fatalf("GetBySession: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("no messages found for session; insertAgentMessage did not take")
	}
	formatted := formatMessagesForSave(messages, maxMessageChars)

	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			"_context_save": {
				Phases: []PhaseDef{{NodeID: "context-saver", Agent: "context-saver", Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			"context-saver": {
				Model:            sysDef.Model,
				Timeout:          sysDef.Timeout,
				ExecutionMode:    sysDef.ExecutionMode,
				Tools:            sysDef.Tools,
				APIMaxIterations: sysDef.APIMaxIterations,
				APIMaxTokens:     sysDef.APIMaxTokens,
			},
		},
		DataPath:           env.dbPath,
		Pool:               pool,
		Clock:              clk,
		APIMode:            true,
		ModelConfigs:       modelConfigs,
		BuildAPIProvider:   buildProvider,
		AgentSvc:           &noopAgentSvc{},
		FindingsSvc:        service.NewFindingsService(pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, clk),
		AgentSvcReal:       service.NewAgentService(pool, clk),
		WorkflowSvc:        service.NewWorkflowService(pool, clk),
		WSHub:              ws.NewHub(clk),
	})

	spawnErr := sp.Spawn(env.ctx, SpawnRequest{
		AgentType:          "context-saver",
		NodeID:             "context-saver",
		TicketID:           env.ticketID,
		ProjectID:          env.projectID,
		WorkflowName:       "_context_save",
		WorkflowInstanceID: env.wfiID,
		ExtraVars: map[string]string{
			"AGENT_TYPE":        "implementor",
			"AGENT_MESSAGES":    formatted,
			"TARGET_SESSION_ID": sessionID,
			"WORKFLOW":          "feature",
			"TICKET_ID":         env.ticketID,
		},
	})
	if spawnErr != nil {
		t.Fatalf("Spawn(context-saver): %v", spawnErr)
	}

	if got := stub.requestCount(); got != 2 {
		t.Errorf("stub requestCount() = %d, want 2 (tool-call turn + closing text turn)", got)
	}
	for i, auth := range stub.authHeaders() {
		if auth != "" {
			t.Errorf("request #%d carried Authorization=%q, want empty (blank api_key, no bearer)", i+1, auth)
		}
	}

	// findings_add scopes to the CURRENT session — the context-saver's own
	// freshly spawned session, not TARGET_SESSION_ID (migration 000063's
	// comment flags cross-session write as a follow-up concern, not yet
	// implemented). So the to_resume finding lands on that child session,
	// which we resolve by agent_type within the shared workflow instance
	// rather than on the original target session directly.
	var childSessionID string
	if err := env.database.QueryRow(
		`SELECT id FROM agent_sessions WHERE workflow_instance_id = ? AND agent_type = 'context-saver'`,
		env.wfiID,
	).Scan(&childSessionID); err != nil {
		t.Fatalf("resolve context-saver child session: %v", err)
	}

	findRepo := repo.NewFindingRepo(env.database, clk)
	findings, err := findRepo.GetOwn("session", childSessionID)
	if err != nil {
		t.Fatalf("GetOwn findings: %v", err)
	}
	var toResume string
	if err := json.Unmarshal(findings["to_resume"], &toResume); err != nil {
		t.Fatalf("unmarshal to_resume: %v", err)
	}
	if !strings.Contains(toResume, "foo.go") {
		t.Errorf("to_resume = %q, want it to contain the stub's summary text", toResume)
	}
}
