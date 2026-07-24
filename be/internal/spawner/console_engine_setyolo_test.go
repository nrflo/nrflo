package spawner

import (
	"context"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
)

// TestClaudeEngine_SetYolo_TogglesApprovalShortCircuit verifies SetYolo(true)
// makes RequestApproval short-circuit immediately, SetYolo(false) re-arms the
// blocking path on the next call, and Yolo() reflects the current state at
// every step.
func TestClaudeEngine_SetYolo_TogglesApprovalShortCircuit(t *testing.T) {
	sink := &testSink{}
	e, _ := startTestClaudeEngine(t, sink, nil, EngineSpec{})

	if e.Yolo() {
		t.Fatal("Yolo() = true on a fresh engine with Yolo unset, want false")
	}

	if err := e.SetYolo(true); err != nil {
		t.Fatalf("SetYolo(true): %v", err)
	}
	if !e.Yolo() {
		t.Error("Yolo() = false after SetYolo(true), want true")
	}

	resultCh := make(chan struct {
		decision, reason string
	}, 1)
	go func() {
		d, r := e.RequestApproval(context.Background(), "Bash", map[string]any{"command": "ls"}, "tu-set-yolo-1")
		resultCh <- struct{ decision, reason string }{d, r}
	}()
	select {
	case res := <-resultCh:
		if res.decision != "allow" || res.reason != "nrflo: yolo" {
			t.Errorf("RequestApproval after SetYolo(true) = %+v, want decision=allow reason=%q", res, "nrflo: yolo")
		}
	case <-time.After(time.Second):
		t.Fatal("RequestApproval did not return immediately after SetYolo(true)")
	}

	if err := e.SetYolo(false); err != nil {
		t.Fatalf("SetYolo(false): %v", err)
	}
	if e.Yolo() {
		t.Error("Yolo() = true after SetYolo(false), want false")
	}

	go func() {
		d, r := e.RequestApproval(context.Background(), "Bash", map[string]any{"command": "ls"}, "tu-set-yolo-2")
		resultCh <- struct{ decision, reason string }{d, r}
	}()
	// RequestApproval must now block on the real approval path — resolve it
	// via ReplyApproval instead of a yolo short-circuit.
	waitForEventType(t, e.Events(), EventApprovalRequest, time.Second)
	if err := e.ReplyApproval("tu-set-yolo-2", ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}
	select {
	case res := <-resultCh:
		if res.decision != "allow" {
			t.Errorf("RequestApproval after SetYolo(false) decision = %q, want allow", res.decision)
		}
		if res.reason == "nrflo: yolo" {
			t.Error("RequestApproval after SetYolo(false) still returned the yolo reason")
		}
	case <-time.After(time.Second):
		t.Fatal("RequestApproval did not resolve after ReplyApproval")
	}
}

// TestAPIConsoleEngine_SetYolo_TogglesToolApprovalShortCircuit mirrors the
// claude case against requestToolApproval.
func TestAPIConsoleEngine_SetYolo_TogglesToolApprovalShortCircuit(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	installFakeAPIProvider(t, mock.New(), nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{Sink: sink, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := fsTestSpec("sess-set-yolo-api", t.TempDir())
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if eng.Yolo() {
		t.Fatal("Yolo() = true on a fresh spec with Yolo unset, want false")
	}

	if err := eng.SetYolo(true); err != nil {
		t.Fatalf("SetYolo(true): %v", err)
	}
	if !eng.Yolo() {
		t.Error("Yolo() = false after SetYolo(true), want true")
	}
	if ok := eng.requestToolApproval(context.Background(), "bash", "run ls"); !ok {
		t.Error("requestToolApproval after SetYolo(true) = false, want true")
	}

	if err := eng.SetYolo(false); err != nil {
		t.Fatalf("SetYolo(false): %v", err)
	}
	if eng.Yolo() {
		t.Error("Yolo() = true after SetYolo(false), want false")
	}

	resultCh := make(chan bool, 1)
	go func() {
		resultCh <- eng.requestToolApproval(context.Background(), "bash", "run ls")
	}()
	req := waitForEventType(t, eng.Events(), EventApprovalRequest, time.Second)
	if err := eng.ReplyApproval(req.Approval.ID, ApprovalApprove); err != nil {
		t.Fatalf("ReplyApproval: %v", err)
	}
	select {
	case ok := <-resultCh:
		if !ok {
			t.Error("requestToolApproval after SetYolo(false)+approve = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for requestToolApproval to return after SetYolo(false)")
	}
}

// TestCodexEngine_SetYolo_UpdatesSpecReturnsNil verifies SetYolo persists
// onto the in-memory spec and never errors — codex's approvalPolicy is fixed
// at thread/start, so the effect only applies on the next rotation, but the
// method contract itself must not surface that as an error.
func TestCodexEngine_SetYolo_UpdatesSpecReturnsNil(t *testing.T) {
	sink := &testSink{}
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		f.replyResult(*env.ID, `{"thread":{"id":"T-set-yolo"}}`)
	})
	eng := newCodexEngine(sink)
	spec := EngineSpec{SessionID: "s1", WorkDir: t.TempDir()}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if eng.Yolo() {
		t.Fatal("Yolo() = true before SetYolo, want false")
	}
	if err := eng.SetYolo(true); err != nil {
		t.Fatalf("SetYolo(true): %v", err)
	}
	if !eng.Yolo() {
		t.Error("Yolo() = false after SetYolo(true), want true")
	}
	if err := eng.SetYolo(false); err != nil {
		t.Fatalf("SetYolo(false): %v", err)
	}
	if eng.Yolo() {
		t.Error("Yolo() = true after SetYolo(false), want false")
	}
}
