package spawner

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// recordingSpawnerProvider wraps the scripted mock provider and captures
// every Request so tests can assert the replayed tool specs/results.
type recordingSpawnerProvider struct {
	inner provider.Provider
	mu    sync.Mutex
	reqs  []provider.Request
}

func newRecordingSpawnerProvider(scripts ...mock.Script) *recordingSpawnerProvider {
	return &recordingSpawnerProvider{inner: mock.New(scripts...)}
}

func (p *recordingSpawnerProvider) Name() string                { return p.inner.Name() }
func (p *recordingSpawnerProvider) MaxContext(model string) int { return p.inner.MaxContext(model) }

func (p *recordingSpawnerProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	return p.inner.Run(ctx, req, sink)
}

func (p *recordingSpawnerProvider) Requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.Request, len(p.reqs))
	copy(out, p.reqs)
	return out
}

func fsTestSpec(sessionID, workDir string) EngineSpec {
	spec := apiTestSpec(sessionID)
	spec.WorkDir = workDir
	return spec
}

func bashToolUse(id, command string) provider.ContentBlock {
	input, _ := json.Marshal(map[string]string{"command": command})
	return provider.ContentBlock{Type: "tool_use", ToolUseID: id, ToolName: "bash", Input: input}
}

// TestAPIConsoleEngine_FSTools_ApprovalRoundtrip drives a full turn where the
// model calls bash: the injected fs tools must surface an approval request,
// an allow must execute the command, and the turn must complete. Requires
// api_native_tools_enabled — the whole feature is gated on it.
func TestAPIConsoleEngine_FSTools_ApprovalRoundtrip(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("api_native_tools_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{bashToolUse("tu-fs-1", "echo approved-run")},
		}},
		mock.Script{Final: provider.FinalResponse{
			StopReason: "end_turn",
			Content:    []provider.ContentBlock{{Type: "text", Text: "done"}},
		}},
	)
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{Sink: sink, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), fsTestSpec("sess-fs-appr", t.TempDir())); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "run echo"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	req := waitForEventType(t, eng.Events(), EventApprovalRequest, 5*time.Second)
	if req.Approval == nil || req.ToolName != "bash" {
		t.Fatalf("approval request = %+v, want bash", req)
	}
	if err := eng.ReplyApproval(req.Approval.ID, ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	resolved := waitForEventType(t, eng.Events(), EventApprovalResolved, 5*time.Second)
	if resolved.Decision != ApprovalApprove {
		t.Errorf("resolved decision = %q, want approve", resolved.Decision)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	// The tool actually ran: the tool_result replayed to the provider carries
	// the command's stdout.
	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if len(last.Content) != 1 || !strings.Contains(last.Content[0].Output, "approved-run") || last.Content[0].IsError {
		t.Errorf("tool_result = %+v, want bash stdout 'approved-run'", last.Content)
	}
}

// TestAPIConsoleEngine_FSTools_DenyBlocksExecution verifies a denied bash
// call returns a tool error result without executing, and that the fs tools
// are absent entirely when the gate is off.
func TestAPIConsoleEngine_FSTools_DenyBlocksExecution(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("api_native_tools_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	marker := t.TempDir() + "/marker"
	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{
			StopReason: "tool_use",
			Content:    []provider.ContentBlock{bashToolUse("tu-fs-2", "touch "+marker)},
		}},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), fsTestSpec("sess-fs-deny", t.TempDir())); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "touch it"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	req := waitForEventType(t, eng.Events(), EventApprovalRequest, 5*time.Second)
	if err := eng.ReplyApproval(req.Approval.ID, ApprovalDeny); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	reqs := prov.Requests()
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if !last.Content[0].IsError || !strings.Contains(last.Content[0].Output, "denied by user") {
		t.Errorf("tool_result = %+v, want denied-by-user error", last.Content)
	}
}

// TestAPIConsoleEngine_FSTools_AbsentWhenGateOff: with the global setting
// unset, the engine must not offer read_file/edit_file/bash at all.
func TestAPIConsoleEngine_FSTools_AbsentWhenGateOff(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	)
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), fsTestSpec("sess-fs-off", t.TempDir())); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hello"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	for _, spec := range prov.Requests()[0].Tools {
		if spec.Name == "bash" || spec.Name == "edit_file" || spec.Name == "read_file" {
			t.Errorf("tool %q offered while api_native_tools_enabled is off", spec.Name)
		}
	}
}

// TestAPIConsoleEngine_FSTools_PolicyNone_OverridesGlobalEnabled verifies a
// console.Profile's NativeToolPolicy="none" (e.g. t0-decider) refuses fs
// tools even when the api_native_tools_enabled global is on — the profile's
// no-fs/bash invariant must not be bypassable through that global.
func TestAPIConsoleEngine_FSTools_PolicyNone_OverridesGlobalEnabled(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	settings := service.NewGlobalSettingsService(pool, clk)
	if err := settings.Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set("api_native_tools_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	)
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := fsTestSpec("sess-fs-policy-none", t.TempDir())
	spec.NativeToolPolicy = NativeToolPolicyNone
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hello"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	for _, spec := range prov.Requests()[0].Tools {
		if spec.Name == "bash" || spec.Name == "edit_file" || spec.Name == "read_file" {
			t.Errorf("tool %q offered under NativeToolPolicy=none despite api_native_tools_enabled=true", spec.Name)
		}
	}
}

// TestAPIConsoleEngine_FSTools_PolicyFull_AddsEvenWhenGlobalDisabled
// verifies NativeToolPolicy="full" (e.g. t0-hands) offers fs tools even when
// the api_native_tools_enabled global is off/unset.
func TestAPIConsoleEngine_FSTools_PolicyFull_AddsEvenWhenGlobalDisabled(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatal(err)
	}

	prov := newRecordingSpawnerProvider(
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	)
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := fsTestSpec("sess-fs-policy-full", t.TempDir())
	spec.NativeToolPolicy = NativeToolPolicyFull
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hello"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	found := false
	for _, spec := range prov.Requests()[0].Tools {
		if spec.Name == "bash" {
			found = true
		}
	}
	if !found {
		t.Error("bash tool not offered under NativeToolPolicy=full despite api_native_tools_enabled being off")
	}
}
