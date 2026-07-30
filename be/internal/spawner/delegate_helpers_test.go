package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
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

// itemRoutedProvider gives each fanout worker its own mock script pair,
// routed by which fanout item appears in the worker's prompt. A shared
// linear script queue would interleave across concurrent workers, letting
// one worker draw another's agent_finished turn — which the _delegate
// findings guard rejects, desyncing the queue.
type itemRoutedProvider struct {
	mu     sync.Mutex
	byItem map[string]provider.Provider
}

func newItemRoutedProvider(items []string, answer string) *itemRoutedProvider {
	p := &itemRoutedProvider{byItem: map[string]provider.Provider{}}
	for _, item := range items {
		p.byItem[item] = mock.New(delegateWorkerScripts(answer)...)
	}
	return p
}

func (p *itemRoutedProvider) Name() string                { return "mock-routed" }
func (p *itemRoutedProvider) MaxContext(model string) int { return 200000 }

func (p *itemRoutedProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	var blob strings.Builder
	for _, m := range req.Messages {
		for _, b := range m.Content {
			blob.WriteString(b.Text)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for item, inner := range p.byItem {
		if strings.Contains(blob.String(), item) {
			return inner.Run(ctx, req, sink)
		}
	}
	return nil, fmt.Errorf("itemRoutedProvider: no fanout item matched in prompt")
}

// seedDelegationRow inserts a durable delegations row (migration 000216)
// directly via DelegationRepo, replacing the old `_delegation_<id>` finding
// seed. workerSessionIDs/spawnErrors are index-aligned and written via
// SetWorkerSlot; fanoutDone marks the row done so GetDelegation evaluates
// worker results instead of reporting "running".
func seedDelegationRow(t *testing.T, env *delegateTestEnv, delegationID, tier string, workerSessionIDs, spawnErrors []string, fanoutDone bool) {
	t.Helper()
	delegationRepo := repo.NewDelegationRepo(env.pool, clock.Real())
	if err := delegationRepo.Create(&model.Delegation{
		ID:                 delegationID,
		CallerSessionID:    env.callerSessionID,
		WorkflowInstanceID: env.wfiID,
		ProjectID:          env.projectID,
		Tier:               tier,
		Fanout:             len(workerSessionIDs),
		Depth:              1,
	}); err != nil {
		t.Fatalf("seed delegation row: %v", err)
	}
	for i, sid := range workerSessionIDs {
		errMsg := ""
		if i < len(spawnErrors) {
			errMsg = spawnErrors[i]
		}
		if err := delegationRepo.SetWorkerSlot(delegationID, i, sid, errMsg); err != nil {
			t.Fatalf("SetWorkerSlot(%d): %v", i, err)
		}
	}
	if fanoutDone {
		if err := delegationRepo.MarkFanoutDone(delegationID); err != nil {
			t.Fatalf("MarkFanoutDone: %v", err)
		}
	}
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
