package spawner

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
)

// TestCodexEngine_Start_ThreadStartYolo asserts EngineSpec.Yolo=true drives
// thread/start with approvalPolicy="never" and sandbox="workspace-write".
func TestCodexEngine_Start_ThreadStartYolo(t *testing.T) {
	sink := &testSink{}
	paramsCh := make(chan json.RawMessage, 1)
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"T-yolo"}}`)
	})
	eng := newCodexEngine(sink)
	spec := EngineSpec{SessionID: "s1", WorkDir: t.TempDir(), Yolo: true}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	params := mustRecvParams(t, paramsCh)
	var p struct {
		Sandbox        string `json:"sandbox"`
		ApprovalPolicy string `json:"approvalPolicy"`
	}
	_ = json.Unmarshal(params, &p)
	if p.ApprovalPolicy != "never" {
		t.Errorf("thread/start approvalPolicy = %q, want never", p.ApprovalPolicy)
	}
	if p.Sandbox != "workspace-write" {
		t.Errorf("thread/start sandbox = %q, want workspace-write", p.Sandbox)
	}
}

// TestCodexEngine_Start_ThreadStartYolo_ReadOnlySandboxSurvives mirrors
// TestCodexEngine_Start_NativeToolsNone_UsesReadOnlySandbox: Yolo=true with an
// explicit read-only sandbox (NativeToolPolicy=none profile) must not
// override the read-only sandbox — only ApprovalPolicy is yolo-driven.
func TestCodexEngine_Start_ThreadStartYolo_ReadOnlySandboxSurvives(t *testing.T) {
	sink := &testSink{}
	paramsCh := make(chan json.RawMessage, 1)
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"T-yolo-ro"}}`)
	})
	eng := newCodexEngine(sink)
	spec := EngineSpec{SessionID: "s1", WorkDir: t.TempDir(), Yolo: true, Sandbox: model.SandboxReadOnly}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	params := mustRecvParams(t, paramsCh)
	var p struct {
		Sandbox        string `json:"sandbox"`
		ApprovalPolicy string `json:"approvalPolicy"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		t.Fatalf("unmarshal thread/start params: %v", err)
	}
	if p.Sandbox != model.SandboxReadOnly {
		t.Errorf("thread/start sandbox = %q, want %q (yolo must not touch an explicit sandbox)", p.Sandbox, model.SandboxReadOnly)
	}
	if p.ApprovalPolicy != "never" {
		t.Errorf("thread/start approvalPolicy = %q, want never", p.ApprovalPolicy)
	}
}

// TestClaudeEngine_RequestApproval_Yolo_AllowsWithoutRequest asserts
// spec.Yolo=true short-circuits RequestApproval to ("allow","nrflo: yolo")
// without emitting EventApprovalRequest or registering a pending approval,
// while EventToolInvoke is still emitted (the tool still streams).
func TestClaudeEngine_RequestApproval_Yolo_AllowsWithoutRequest(t *testing.T) {
	sink := &testSink{}
	e, _ := startTestClaudeEngine(t, sink, nil, EngineSpec{Yolo: true})

	resultCh := make(chan struct {
		decision, reason string
	}, 1)
	go func() {
		d, r := e.RequestApproval(context.Background(), "Bash", map[string]any{"command": "ls"}, "tu-yolo-1")
		resultCh <- struct{ decision, reason string }{d, r}
	}()

	invoke := waitForEventType(t, e.Events(), EventToolInvoke, time.Second)
	if invoke.ToolName != "Bash" {
		t.Errorf("tool_invoke ToolName = %q, want Bash", invoke.ToolName)
	}

	select {
	case res := <-resultCh:
		if res.decision != "allow" || res.reason != "nrflo: yolo" {
			t.Errorf("RequestApproval result = %+v, want decision=allow reason=%q", res, "nrflo: yolo")
		}
	case <-time.After(time.Second):
		t.Fatal("RequestApproval did not return immediately under yolo")
	}

	// No pending approval was ever registered — replying to it must error.
	if err := e.ReplyApproval("tu-yolo-1", ApprovalApprove); err == nil {
		t.Error("expected ReplyApproval to error: yolo must not register a pending approval")
	}

	// No EventApprovalRequest must have been emitted.
	select {
	case ev := <-e.Events():
		if ev.Type == EventApprovalRequest {
			t.Errorf("unexpected EventApprovalRequest under yolo: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// TestAPIConsoleEngine_RequestToolApproval_Yolo_ReturnsTrueImmediately
// asserts spec.Yolo=true short-circuits requestToolApproval to true without
// emitting EventApprovalRequest.
func TestAPIConsoleEngine_RequestToolApproval_Yolo_ReturnsTrueImmediately(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	installFakeAPIProvider(t, mock.New(), nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{Sink: sink, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := fsTestSpec("sess-yolo-tool", t.TempDir())
	spec.Yolo = true
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	ok := eng.requestToolApproval(context.Background(), "bash", "run ls")
	if !ok {
		t.Errorf("requestToolApproval under yolo = false, want true")
	}

	select {
	case ev := <-eng.Events():
		if ev.Type == EventApprovalRequest {
			t.Errorf("unexpected EventApprovalRequest under yolo: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

// TestAPIConsoleEngine_RequestToolApproval_NoYolo_StillGates asserts
// spec.Yolo=false (the zero value) keeps the blocking approval path.
func TestAPIConsoleEngine_RequestToolApproval_NoYolo_StillGates(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	installFakeAPIProvider(t, mock.New(), nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{Sink: sink, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := fsTestSpec("sess-noyolo-tool", t.TempDir())
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	resultCh := make(chan bool, 1)
	go func() {
		resultCh <- eng.requestToolApproval(context.Background(), "bash", "run ls")
	}()

	req := waitForEventType(t, eng.Events(), EventApprovalRequest, time.Second)
	if req.ToolName != "bash" {
		t.Errorf("approval request ToolName = %q, want bash", req.ToolName)
	}
	if err := eng.ReplyApproval(req.Approval.ID, ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}

	select {
	case ok := <-resultCh:
		if !ok {
			t.Errorf("requestToolApproval result = false, want true after approve")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requestToolApproval to return")
	}
}
