package spawner

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestDelegate_Depth_TopLevelCallerYieldsDepthOne verifies a fresh, non-worker
// caller seeds a delegations row at depth 1 (DepthForSession(callerSessionID)
// resolves 0 for a top-level caller, +1 for this delegation).
func TestDelegate_Depth_TopLevelCallerYieldsDepthOne(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("ok")...))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "top level",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.Depth != 1 {
		t.Errorf("row depth = %d, want 1 for a top-level caller", d.Depth)
	}

	waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
}

// TestDelegate_Depth_ExistingWorkerCallerYieldsDepthTwo verifies a caller that
// is itself tracked as a worker of a depth-1 delegation produces a depth-2
// row when it delegates further — DepthForSession resolves the caller's own
// depth from the row it's a worker of, and createDelegationRecord adds 1.
// This is the DB-derived depth that the worker's child Spawner carries as
// Config.DelegateDepth (delegate.go), replacing in-memory depth threading —
// it is what makes the console path (a fresh Spawner per call, always
// in-memory depth 0) resolve correctly too.
func TestDelegate_Depth_ExistingWorkerCallerYieldsDepthTwo(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	// Seed a depth-1 delegation whose sole worker slot is env.callerSessionID
	// itself, simulating that this session is currently running as a
	// delegate worker one level down.
	seedDelegationRow(t, env, env.wfiID+".parent01", "extractor", []string{env.callerSessionID}, nil, true)

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("nested")...))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "nested delegate",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.Depth != 2 {
		t.Errorf("row depth = %d, want 2 (caller's own depth 1 + 1)", d.Depth)
	}

	waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
}

// itemGatedProvider blocks Run() for one designated item (matched by
// substring against the rendered system prompt, which includes
// ${DELEGATE_ITEM}) until the test signals release, while every other item
// completes immediately. This lets a test observe a fanout worker's slot
// landing in the delegations row while a sibling worker is still running —
// no time.Sleep, pure channel synchronization.
type itemGatedProvider struct {
	gateOnSubstring string
	release         chan struct{}
	startedGated    chan struct{}
	mu              sync.Mutex
	gatedStarted    bool
}

// requestContains searches the system prompt and every text content block
// across all messages for substr — the rendered ${DELEGATE_ITEM} lands in
// the initial user message (loadTemplate's expansion), not the system
// prompt, so both must be checked.
func requestContains(req provider.Request, substr string) bool {
	if strings.Contains(req.System, substr) {
		return true
	}
	for _, m := range req.Messages {
		for _, c := range m.Content {
			if strings.Contains(c.Text, substr) {
				return true
			}
		}
	}
	return false
}

func (p *itemGatedProvider) Name() string          { return "mock" }
func (p *itemGatedProvider) MaxContext(string) int { return 200000 }
func (p *itemGatedProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	if requestContains(req, p.gateOnSubstring) {
		p.mu.Lock()
		if !p.gatedStarted {
			p.gatedStarted = true
			close(p.startedGated)
		}
		p.mu.Unlock()
		<-p.release
	}
	addInput, _ := json.Marshal(map[string]string{}) // agent_finished takes no args
	sink.OnToolUseStart("tu-1", "agent_finished")
	sink.OnToolUseStop("tu-1", json.RawMessage(`{}`))
	return &provider.FinalResponse{
		StopReason: "tool_use",
		Content: []provider.ContentBlock{
			{Type: "tool_use", ToolUseID: "tu-1", ToolName: "agent_finished", Input: addInput},
		},
	}, nil
}

// TestDelegate_WorkerSlot_WrittenIncrementally_BeforeFanoutWaitCompletes
// verifies recordWorkerSlot lands each worker's slot as soon as its own Spawn
// returns, not only after every worker in the fanout finishes: with one item
// held open, the other item's slot must already be populated in the
// delegations row while the held item is still running.
func TestDelegate_WorkerSlot_WrittenIncrementally_BeforeFanoutWaitCompletes(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	prov := &itemGatedProvider{
		gateOnSubstring: "held-item",
		release:         make(chan struct{}),
		startedGated:    make(chan struct{}),
	}
	sp := buildDelegateSpawner(t, env, prov)

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:   "executor",
		Brief:  "review ${DELEGATE_ITEM}",
		Fanout: []string{"fast-item", "held-item"},
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	// Wait until the held worker has actually started (so the fast worker has
	// had a fair chance to finish and record its slot), then check the row
	// while the held worker is still blocked in Run().
	<-prov.startedGated
	waitForSlotFilled(t, env, delegationID)

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.FanoutDone {
		t.Error("FanoutDone = true, want false while the held worker is still running")
	}
	filled := 0
	for _, sid := range d.WorkerSessionIDs {
		if sid != "" {
			filled++
		}
	}
	if filled != 1 {
		t.Errorf("filled worker slots = %d, want exactly 1 recorded while the other worker is still held", filled)
	}

	close(prov.release)
	waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
}

// waitForSlotFilled polls the delegations row until at least one worker slot
// is non-blank, yielding via runtime.Gosched (never time.Sleep, per Rule 4).
func waitForSlotFilled(t *testing.T, env *delegateTestEnv, delegationID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
		if err == nil {
			for _, sid := range d.WorkerSessionIDs {
				if sid != "" {
					return
				}
			}
		}
		runtime.Gosched()
	}
	t.Fatal("no worker slot filled within timeout")
}
