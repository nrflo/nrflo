package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"runtime"
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

// delegateModelConfigs supplies both tier models the delegate builtin
// resolves to; DefaultEffort matches each system_agent_definitions seed row
// (migration 000182) so resolveReasoningEffort's fallback validates cleanly.
func delegateModelConfigs() map[string]ModelConfig {
	return map[string]ModelConfig{
		"haiku-4-5": {Provider: "anthropic", APIModel: "claude-haiku-4-5", APIContext: 200000, APIEfforts: []string{"low", "medium", "high"}, DefaultEffort: "low"},
		"sonnet-5":  {Provider: "anthropic", APIModel: "claude-sonnet-5", APIContext: 200000, APIEfforts: []string{"low", "medium", "high"}, DefaultEffort: "medium"},
	}
}

// delegateTestEnv extends the shared context-save test env with a caller
// agent_session bound to the base workflow instance.
type delegateTestEnv struct {
	*contextSaveTestEnv
	pool            *db.Pool
	callerSessionID string
}

func setupDelegateTestEnv(t *testing.T) *delegateTestEnv {
	t.Helper()
	base := setupContextSaveTestEnv(t)
	pool := db.WrapAsPool(base.database)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	callerSID := "caller-session-delegate"
	if err := repo.NewAgentSessionRepo(base.database, clock.Real()).Create(&model.AgentSession{
		ID:                 callerSID,
		ProjectID:          base.projectID,
		TicketID:           base.ticketID,
		WorkflowInstanceID: base.wfiID,
		Phase:              "implementor",
		AgentType:          "implementor",
		ModelID:            sql.NullString{String: "claude:opus-4-7", Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create caller session: %v", err)
	}

	return &delegateTestEnv{contextSaveTestEnv: base, pool: pool, callerSessionID: callerSID}
}

func buildDelegateSpawner(t *testing.T, env *delegateTestEnv, prov provider.Provider) *Spawner {
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
		ModelConfigs:       delegateModelConfigs(),
		AgentSvc:           &noopAgentSvc{},
		FindingsSvc:        service.NewFindingsService(env.pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(env.pool, clk),
		AgentSvcReal:       service.NewAgentService(env.pool, clk),
		WorkflowSvc:        service.NewWorkflowService(env.pool, clk),
	})
}

// delegateWorkerScripts returns a two-turn script pair for one worker: turn 1
// writes _delegate_findings, turn 2 calls agent_finished. One pair is
// consumed per worker regardless of fanout ordering (mock's cursor is
// shared/racy across concurrent workers, but every worker follows the exact
// same two-script shape).
func delegateWorkerScripts(answer string) []mock.Script {
	// findings_add's "value" arg is a Go string (JSON schema type "string");
	// it must carry the finding's JSON payload *encoded as a string*, not a
	// raw nested object, or json.Unmarshal into the handler's args struct
	// fails silently (isError, no finding ever written).
	findingJSON, _ := json.Marshal(map[string]string{"answer": answer})
	addInput, _ := json.Marshal(map[string]string{"key": "_delegate_findings", "value": string(findingJSON)})
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

func manyDelegateWorkerScripts(n int, answer string) []mock.Script {
	var out []mock.Script
	for i := 0; i < n; i++ {
		out = append(out, delegateWorkerScripts(answer)...)
	}
	return out
}

// waitForDelegationDone polls GetDelegation until the delegation leaves the
// "running" state. Delegate seeds a done=false tracking record synchronously
// before returning, so GetDelegation resolves the delegation as "running" for
// the whole run (never "unknown delegation") and flips to completed/failed
// once runDelegateFanout rewrites it with done=true; the err branch below is
// pure defense against a transient DB read.
func waitForDelegationDone(t *testing.T, sp *Spawner, callerSessionID, delegationID string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := sp.GetDelegation(context.Background(), callerSessionID, delegationID)
		if err != nil {
			runtime.Gosched()
			continue
		}
		var out map[string]interface{}
		if jerr := json.Unmarshal([]byte(raw), &out); jerr != nil {
			t.Fatalf("unmarshal GetDelegation result: %v", jerr)
		}
		if out["status"] != "running" {
			return out
		}
		runtime.Gosched()
	}
	t.Fatal("delegation did not finish within timeout")
	return nil
}
