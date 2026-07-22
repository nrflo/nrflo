package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// TestDelegate_LocalProviderStub_ExtractorEndToEnd drives a real T2 extractor
// delegate worker through service.BuildAPIProvider -> custom.New ->
// openaichat against a localhost stub standing in for Ollama: registers a
// custom_providers row pointed at the stub (api_wire=chat_completions, blank
// api_key), repoints _t2_extractor's model at a ModelConfigs slug whose
// Provider names that row, and asserts the delegation completes with the
// worker's _delegate_findings answer — with the stub as the only endpoint
// contacted and no bearer token sent.
func TestDelegate_LocalProviderStub_ExtractorEndToEnd(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	findingJSON, _ := json.Marshal(map[string]string{"answer": "v1.2.3"})
	addInput, _ := json.Marshal(map[string]string{"key": "_delegate_findings", "value": string(findingJSON)})

	stub := newLocalProviderStub(t, []string{
		sseToolCallScript("tu-1", "findings_add", addInput),
		sseToolCallScript("tu-2", "agent_finished", json.RawMessage(`{}`)),
	})

	clk := clock.Real()
	cpSvc := service.NewCustomProviderService(env.pool, clk)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name:    "local-ollama-delegate",
		BaseURL: stub.URL(),
		APIWire: "chat_completions",
	}); err != nil {
		t.Fatalf("create custom provider: %v", err)
	}

	if _, err := env.database.Exec(
		`UPDATE system_agent_definitions SET model = ? WHERE id = '_t2_extractor'`,
		"local-ollama-extractor-model",
	); err != nil {
		t.Fatalf("repoint _t2_extractor model: %v", err)
	}

	// ResolveAgentChain's override branch validates the repointed model
	// against the live `models` table (service.ModelService), not just the
	// spawner's runtime ModelConfigs map — mirroring doc/local-providers.md's
	// "Add a model row" step for a custom-provider (API-only, no cli_model).
	modelSvc := service.NewModelService(env.pool, clk)
	if _, err := modelSvc.Create(types.ModelCreateRequest{
		ID:            "local-ollama-extractor-model",
		Provider:      "local-ollama-delegate",
		DisplayName:   "local-ollama-extractor-model",
		APIModel:      "qwen-test",
		APIContext:    128000,
		APIEfforts:    []string{"low", "medium", "high"},
		DefaultEffort: "low",
	}); err != nil {
		t.Fatalf("create local-ollama-extractor-model row: %v", err)
	}

	modelConfigs := delegateModelConfigs()
	modelConfigs["local-ollama-extractor-model"] = ModelConfig{
		Provider:      "local-ollama-delegate",
		APIModel:      "qwen-test",
		APIContext:    128000,
		APIEfforts:    []string{"low", "medium", "high"},
		DefaultEffort: "low",
	}

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.pool,
		Clock:    clk,
		WSHub:    ws.NewHub(clk),
		APIMode:  true,
		BuildAPIProvider: func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
			return service.BuildAPIProvider(ctx, env.pool, clk, providerName, projectID)
		},
		ModelConfigs:       modelConfigs,
		AgentSvc:           &noopAgentSvc{},
		FindingsSvc:        service.NewFindingsService(env.pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(env.pool, clk),
		AgentSvcReal:       service.NewAgentService(env.pool, clk),
		WorkflowSvc:        service.NewWorkflowService(env.pool, clk),
	})

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "extract the version number",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal Delegate result: %v", err)
	}
	delegationID, _ := start["delegation_id"].(string)
	if delegationID == "" {
		t.Fatal("delegation_id is empty")
	}

	final := waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
	if final["status"] != "completed" {
		t.Errorf("final status = %v, want completed", final["status"])
	}
	results, ok := final["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("results = %v, want a single-element array", final["results"])
	}
	entry := results[0].(map[string]interface{})
	if entry["status"] != "completed" {
		t.Errorf("worker status = %v, want completed", entry["status"])
	}
	if entry["findings"] == nil {
		t.Errorf("worker entry missing findings: %+v", entry)
	}

	if got := stub.requestCount(); got != 2 {
		t.Errorf("stub requestCount() = %d, want 2 (one per turn)", got)
	}
	for i, auth := range stub.authHeaders() {
		if auth != "" {
			t.Errorf("request #%d carried Authorization=%q, want empty (blank api_key, no bearer)", i+1, auth)
		}
	}
}
