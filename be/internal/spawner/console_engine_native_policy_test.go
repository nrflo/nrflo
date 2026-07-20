package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/model"
)

// TestClaudeEngine_Start_NativeToolsNone_EmitsEmptyToolsFlag verifies a
// console.Profile with NativeToolPolicy="none" (nativeToolFieldsForPolicy
// maps it to model.NativeToolsNone) reaches the claude engine's argv as
// `--tools ""` — MCP-only, mirroring cli_adapter_claude.go's autonomous-spawn
// precedent.
func TestClaudeEngine_Start_NativeToolsNone_EmitsEmptyToolsFlag(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{
		SessionID:      "sess-native-none",
		NativeToolsCSV: model.NativeToolsNone,
	})
	launch := mgr.registeredLaunches[e.spec.SessionID]
	pos := findArgElement(launch.Args, "--tools")
	if pos == -1 || pos+1 >= len(launch.Args) {
		t.Fatalf("argv %v missing --tools flag", launch.Args)
	}
	if launch.Args[pos+1] != "" {
		t.Errorf("--tools value = %q, want empty (MCP-only)", launch.Args[pos+1])
	}
}

// TestClaudeEngine_Start_NoNativeToolsCSV_OmitsToolsFlag is the
// byte-identical regression: a chat with no profile (NativeToolsCSV empty)
// must not carry a --tools flag at all.
func TestClaudeEngine_Start_NoNativeToolsCSV_OmitsToolsFlag(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-native-unset"})
	launch := mgr.registeredLaunches[e.spec.SessionID]
	if pos := findArgElement(launch.Args, "--tools"); pos != -1 {
		t.Errorf("argv %v unexpectedly contains --tools with no profile set", launch.Args)
	}
}

// TestCodexEngine_Start_NativeToolsNone_UsesReadOnlySandbox verifies a
// console.Profile with NativeToolPolicy="none" maps to spec.Sandbox ==
// model.SandboxReadOnly (nativeToolFieldsForPolicy), which the codex engine
// threads verbatim into thread/start's sandbox param.
func TestCodexEngine_Start_NativeToolsNone_UsesReadOnlySandbox(t *testing.T) {
	sink := &testSink{}
	paramsCh := make(chan json.RawMessage, 1)
	t.Setenv("HOME", t.TempDir())
	f := newFakeAppServer(t)
	f.install(t)
	f.setOverride("thread/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"thread":{"id":"T-native-none"}}`)
	})
	eng := newCodexEngine(sink)
	spec := EngineSpec{SessionID: "sess-codex-native-none", WorkDir: t.TempDir(), Sandbox: model.SandboxReadOnly}
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	params := mustRecvParams(t, paramsCh)
	var p struct {
		Sandbox string `json:"sandbox"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		t.Fatalf("unmarshal thread/start params: %v", err)
	}
	if p.Sandbox != model.SandboxReadOnly {
		t.Errorf("thread/start sandbox = %q, want %q", p.Sandbox, model.SandboxReadOnly)
	}
}
