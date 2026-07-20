package console

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// newT0DeciderTestService mirrors newChatTestService but also wires
// ChatDeps.Tools (Pool/Clock/Delegator) so the fake engine's captured
// registry/ToolEnv can actually dispatch delegate/get_delegation against a
// real DB — the plain newChatTestService leaves Tools at its zero value,
// which is fine for tests that never invoke a captured tool handler.
func newT0DeciderTestService(t *testing.T, delegator apirun.Delegator) (*ChatService, *db.Pool, *ws.Hub, *fakeEngineFactory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chat_t0_test.db")
	if err := copyConsoleTemplateDB(dbPath); err != nil {
		t.Fatalf("copyConsoleTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		chatTestProjectID, chatTestProjectID, now, now)

	hub := ws.NewHub(clk)
	go hub.Run()
	t.Cleanup(hub.Stop)

	svc := NewChatService(ChatDeps{
		Pool:      pool,
		Clock:     clk,
		WSHub:     hub,
		PTY:       ptyPkg.NewManager(),
		Hub:       spawner.NewConsoleHub(),
		ServerURL: "http://127.0.0.1:6587",
		Tools:     Deps{Pool: pool, Clock: clk, WSHub: hub, Delegator: delegator},
	})
	factory := &fakeEngineFactory{}
	svc.SetEngineFactory(factory.factory)
	return svc, pool, hub, factory
}

// TestChatT0Decider_ExposesExactCatalogue_FSToolsStructurallyAbsent verifies
// a t0-decider chat's built EngineDeps.API exposes exactly the profile's
// catalogue: fs/bash tools are structurally absent from the captured
// Handlers/Tools (never composed into the registry at all), not merely
// refused at invoke time.
func TestChatT0Decider_ExposesExactCatalogue_FSToolsStructurallyAbsent(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newT0DeciderTestService(t, nil)

	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	profile, err := ProfileByName("t0-decider")
	if err != nil {
		t.Fatalf("ProfileByName: %v", err)
	}

	eng := factory.last()
	if eng == nil {
		t.Fatal("no fake engine constructed")
	}
	api := eng.apiDeps()
	if len(api.Handlers) != len(profile.Catalogue) {
		t.Fatalf("len(Handlers) = %d, want %d (exactly the catalogue)", len(api.Handlers), len(profile.Catalogue))
	}
	for _, name := range profile.Catalogue {
		if _, ok := api.Handlers[name]; !ok {
			t.Errorf("Handlers missing catalogued tool %q", name)
		}
	}
	for _, banned := range []string{"read_file", "edit_file", "write_file", "bash", "glob", "grep", "web_fetch"} {
		if _, ok := api.Handlers[banned]; ok {
			t.Errorf("Handlers unexpectedly contains %q", banned)
		}
		for _, spec := range api.Tools {
			if spec.Name == banned {
				t.Errorf("Tools unexpectedly advertises %q", banned)
			}
		}
	}
	if len(api.Tools) != len(profile.Catalogue) {
		t.Errorf("len(Tools) = %d, want %d", len(api.Tools), len(profile.Catalogue))
	}
	if api.ToolEnv.SessionID != sid {
		t.Errorf("ToolEnv.SessionID = %q, want %q", api.ToolEnv.SessionID, sid)
	}

	spec := eng.spec()
	if spec.NativeToolsCSV != "none" {
		t.Errorf("spec.NativeToolsCSV = %q, want none", spec.NativeToolsCSV)
	}
	if spec.NativeToolPolicy != NativeToolPolicyNone {
		t.Errorf("spec.NativeToolPolicy = %q, want %q", spec.NativeToolPolicy, NativeToolPolicyNone)
	}
	if spec.MaxContext != 200000 {
		t.Errorf("spec.MaxContext = %d, want 200000 (opus-4-8 CLIContext)", spec.MaxContext)
	}
	if spec.ContextBudgetTokens != 50000 {
		t.Errorf("spec.ContextBudgetTokens = %d, want 50000", spec.ContextBudgetTokens)
	}
}

// TestChatT0Decider_DelegationRoundTrip_FoldsIntoDigest drives delegate then
// get_delegation through the SAME registry/ToolEnv ChatService built into the
// live engine (not a freshly-built one), proving the t0-decider chat's own
// tool wiring — not just BuildRegistry in isolation — supports the
// delegate/get_delegation round trip. The worker's folded result is then
// written to the refinery digest table exactly as refinery.Manager's fold
// would (see be/internal/refinery, out of this package's scope) — this
// proves maybeRotate's digest read sees delegation-derived content.
func TestChatT0Decider_DelegationRoundTrip_FoldsIntoDigest(t *testing.T) {
	t.Parallel()
	fake := &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return `{"delegation_id":"wfi.worker1","status":"running"}`, nil
		},
		getDelegationFn: func(_ context.Context, _, delegationID string) (string, error) {
			if delegationID != "wfi.worker1" {
				t.Errorf("get_delegation delegationID = %q, want wfi.worker1", delegationID)
			}
			return `{"delegation_id":"wfi.worker1","status":"completed","results":[{"status":"completed","summary":"extracted the ticket requirements"}]}`, nil
		},
	}
	svc, pool, _, factory := newT0DeciderTestService(t, fake)

	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	eng := factory.last()
	api := eng.apiDeps()

	delegateOut, isErr, err := api.Handlers["delegate"].Invoke(context.Background(), api.ToolEnv,
		json.RawMessage(`{"tier":"extractor","brief":"extract the ticket requirements"}`))
	if err != nil || isErr {
		t.Fatalf("delegate: err=%v isErr=%v out=%s", err, isErr, delegateOut)
	}
	if !strings.Contains(delegateOut, "wfi.worker1") {
		t.Fatalf("delegate output = %q, want it to contain the delegation id", delegateOut)
	}

	getOut, isErr, err := api.Handlers["get_delegation"].Invoke(context.Background(), api.ToolEnv,
		json.RawMessage(`{"delegation_id":"wfi.worker1"}`))
	if err != nil || isErr {
		t.Fatalf("get_delegation: err=%v isErr=%v out=%s", err, isErr, getOut)
	}
	if !strings.Contains(getOut, "extracted the ticket requirements") {
		t.Fatalf("get_delegation output = %q, want the worker's result folded in", getOut)
	}

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "worker findings: extracted the ticket requirements"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}
	digest, err := digestRepo.Get(sid)
	if err != nil || digest == nil {
		t.Fatalf("digestRepo.Get after fold: digest=%v err=%v", digest, err)
	}
	if !strings.Contains(digest.Content, "extracted the ticket requirements") {
		t.Errorf("folded digest = %q, want it to carry the delegation result", digest.Content)
	}
}

// TestChatT0Decider_ModelSwitch_SpawnsSiblingLeavesOriginUntouched is the
// integration-flavored companion to chat_service_sibling_test.go's unit
// coverage: within a fuller t0-decider session (delegator wired, catalogue
// live), a model switch still opens a distinct sibling and never touches the
// origin's engine.
func TestChatT0Decider_ModelSwitch_SpawnsSiblingLeavesOriginUntouched(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newT0DeciderTestService(t, &fakeDelegator{})

	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create(t0-decider): %v", err)
	}
	originEngine := factory.last()

	siblingID, err := svc.SwitchModel(sid, "claude", "sonnet-5", "")
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	if siblingID == sid {
		t.Fatal("SwitchModel returned the origin's own session id")
	}
	if originEngine.isStopped() {
		t.Error("origin engine stopped by SwitchModel, want it left live")
	}
	if _, ok := svc.get(sid); !ok {
		t.Error("origin session removed from the map by SwitchModel")
	}
	if _, ok := svc.get(siblingID); !ok {
		t.Error("sibling session not found after SwitchModel")
	}
}
