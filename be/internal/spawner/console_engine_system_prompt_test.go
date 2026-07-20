package spawner

// Tests for EngineSpec.SystemPrompt delivery across the three console engines:
// claude writes it to a temp file and passes --system-prompt-file; codex
// prepends it to the first turn only (never a later one); api uses it
// directly as Conversation.System, bypassing the api-console-system-prompt
// injectable render entirely. Empty SystemPrompt must be byte-identical to
// each engine's pre-feature default behavior.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/service"
)

// ── claude console engine ────────────────────────────────────────────────────

// TestClaudeEngine_Start_SystemPromptFile_WritesFileAndFlag verifies a
// non-empty spec.SystemPrompt is written to <tempDir>/system-prompt.md and
// --system-prompt-file is passed pointing at it.
func TestClaudeEngine_Start_SystemPromptFile_WritesFileAndFlag(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-sysprompt-1", SystemPrompt: "TIER-T2 ROLE TEXT"})

	launch := mgr.registeredLaunches[e.spec.SessionID]
	pos := findArgElement(launch.Args, "--system-prompt-file")
	if pos == -1 || pos+1 >= len(launch.Args) {
		t.Fatalf("argv %v missing --system-prompt-file", launch.Args)
	}
	path := launch.Args[pos+1]
	wantPath := filepath.Join(e.tempDir, "system-prompt.md")
	if path != wantPath {
		t.Errorf("--system-prompt-file path = %q, want %q", path, wantPath)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(got) != "TIER-T2 ROLE TEXT" {
		t.Errorf("system-prompt.md content = %q, want %q", string(got), "TIER-T2 ROLE TEXT")
	}
}

// TestClaudeEngine_Start_EmptySystemPrompt_NoFlagByteIdentical verifies that
// with SystemPrompt empty, --system-prompt-file is never emitted — identical
// to the pre-feature claude console argv.
func TestClaudeEngine_Start_EmptySystemPrompt_NoFlagByteIdentical(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-sysprompt-2"})

	launch := mgr.registeredLaunches[e.spec.SessionID]
	if findArgElement(launch.Args, "--system-prompt-file") != -1 {
		t.Errorf("argv %v should not contain --system-prompt-file when SystemPrompt is empty", launch.Args)
	}
	if e.tempDir == "" {
		t.Fatal("tempDir not set after Start")
	}
	if _, err := os.Stat(filepath.Join(e.tempDir, "system-prompt.md")); !os.IsNotExist(err) {
		t.Errorf("system-prompt.md should not exist when SystemPrompt is empty, stat err=%v", err)
	}
}

// ── codex console engine ─────────────────────────────────────────────────────

// turnStartInputText decodes a turn/start rpc envelope's single text input
// block, mirroring TestCodexEngine_SendUserTurn_WireAndPersistence's shape.
func turnStartInputText(t *testing.T, params json.RawMessage) string {
	t.Helper()
	var p struct {
		Input []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		t.Fatalf("unmarshal turn/start params: %v", err)
	}
	if len(p.Input) != 1 {
		t.Fatalf("turn/start input = %+v, want exactly one block", p.Input)
	}
	return p.Input[0].Text
}

// TestCodexEngine_SendUserTurn_SystemPromptPrependedFirstTurnOnly verifies the
// system prompt is prepended to only the first turn/start text (wire), while
// the persisted user_input row keeps the original unprefixed text, and a
// second turn on the same engine carries no prepend at all.
func TestCodexEngine_SendUserTurn_SystemPromptPrependedFirstTurnOnly(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{SystemPrompt: "TIER-T1 ROLE TEXT"})

	firstParamsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		firstParamsCh <- env.Params
		f.replyResult(*env.ID, `{"turn":{"id":"turn-1"}}`)
	})
	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "first message"}); err != nil {
		t.Fatalf("SendUserTurn (first): %v", err)
	}
	firstText := turnStartInputText(t, mustRecvParams(t, firstParamsCh))
	if firstText != "TIER-T1 ROLE TEXT\n\nfirst message" {
		t.Errorf("first turn wire text = %q, want system prompt prepended", firstText)
	}
	if n := countCategory(sink, "user_input"); n != 1 {
		t.Fatalf("user_input rows after first turn = %d, want 1", n)
	}

	// Complete the first turn so turnActive resets, then send a second one.
	f.feed(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"completed"}}}`)
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	secondParamsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		secondParamsCh <- env.Params
		f.replyResult(*env.ID, `{"turn":{"id":"turn-2"}}`)
	})
	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "second message"}); err != nil {
		t.Fatalf("SendUserTurn (second): %v", err)
	}
	secondText := turnStartInputText(t, mustRecvParams(t, secondParamsCh))
	if secondText != "second message" {
		t.Errorf("second turn wire text = %q, want unprefixed %q (no prepend after the first turn)", secondText, "second message")
	}
}

// TestCodexEngine_SendUserTurn_EmptySystemPrompt_ByteIdentical verifies an
// empty SystemPrompt leaves the turn/start wire text byte-identical to the
// pre-feature codex console engine (mirrors
// TestCodexEngine_SendUserTurn_WireAndPersistence's assertion).
func TestCodexEngine_SendUserTurn_EmptySystemPrompt_ByteIdentical(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	paramsCh := make(chan json.RawMessage, 1)
	f.setOverride("turn/start", func(f *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		f.replyResult(*env.ID, `{"turn":{"id":"turn-1"}}`)
	})
	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "plain message"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	got := turnStartInputText(t, mustRecvParams(t, paramsCh))
	if got != "plain message" {
		t.Errorf("turn/start wire text = %q, want byte-identical unprefixed %q", got, "plain message")
	}
}

// ── api console engine ───────────────────────────────────────────────────────

// TestAPIConsoleEngine_Start_SystemPromptFromSpec_SkipsInjectableRender
// verifies a non-empty spec.SystemPrompt is used directly as Conversation
// System, bypassing the api-console-system-prompt injectable/fallback render
// entirely (even when a custom row is seeded, proving spec wins).
func TestAPIConsoleEngine_Start_SystemPromptFromSpec_SkipsInjectableRender(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at)
		VALUES ('api-console-system-prompt', 'API console system prompt', 'ROW-SHOULD-NOT-WIN', 'ROW-SHOULD-NOT-WIN', 0, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed api-console-system-prompt: %v", err)
	}

	prov := &recordingAPIProvider{}
	installFakeAPIProvider(t, prov, nil)

	sessionID := "sess-sysprompt-fromspec"
	seedAPIEngineSession(t, pool, clk, "p1", sessionID)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := apiTestSpec(sessionID)
	spec.SystemPrompt = "TIER-T0 ROLE TEXT"
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hi"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	if prov.lastSystem() != "TIER-T0 ROLE TEXT" {
		t.Errorf("captured System = %q, want spec.SystemPrompt %q (row must not win)", prov.lastSystem(), "TIER-T0 ROLE TEXT")
	}
}

// TestAPIConsoleEngine_Start_EmptySystemPrompt_FallsBackToInjectable is the
// byte-identical regression: with spec.SystemPrompt empty, Start still renders
// the api-console-system-prompt injectable exactly as before this feature.
func TestAPIConsoleEngine_Start_EmptySystemPrompt_FallsBackToInjectable(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}

	prov := &recordingAPIProvider{}
	installFakeAPIProvider(t, prov, nil)

	sessionID := "sess-sysprompt-fallback"
	seedAPIEngineSession(t, pool, clk, "p1", sessionID)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := apiTestSpec(sessionID) // SystemPrompt left empty
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hi"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	if prov.lastSystem() != consoleAPISystem {
		t.Errorf("captured System = %q, want fallback consoleAPISystem %q", prov.lastSystem(), consoleAPISystem)
	}
}
