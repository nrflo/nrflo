package spawner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadFixtureNotifications returns the server→client notifications (method set,
// no id) from a captured app-server JSONL stream.
func loadFixtureNotifications(t *testing.T, name string) []rpcEnvelope {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "codex_appserver", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	var out []rpcEnvelope
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var env rpcEnvelope
		if json.Unmarshal(line, &env) != nil {
			continue
		}
		if env.Method != "" && env.ID == nil {
			out = append(out, env)
		}
	}
	return out
}

// TestDispatchAppServer_FullTurnFixture replays a captured real turn and asserts
// the Sink receives the agent message, the tool call (+exit), context updates,
// and turn lifecycle signals.
func TestDispatchAppServer_FullTurnFixture(t *testing.T) {
	notifs := loadFixtureNotifications(t, "full_turn.jsonl")
	if len(notifs) == 0 {
		t.Fatal("no notifications loaded from fixture")
	}
	sink := &opencodeTestSink{}
	var sawTurnStarted, sawTurnCompleted bool
	for _, n := range notifs {
		sig := dispatchAppServerEvent("sess-1", n, sink, 200000)
		sawTurnStarted = sawTurnStarted || sig.turnStarted
		sawTurnCompleted = sawTurnCompleted || sig.turnCompleted
		if sig.rateLimited {
			t.Errorf("unexpected rateLimited on a clean turn (method=%s)", n.Method)
		}
		if sig.fatalErr != "" {
			t.Errorf("unexpected fatalErr on a clean turn: %q", sig.fatalErr)
		}
	}

	var texts, tools []string
	for _, m := range sink.recordedMsgs {
		switch m.category {
		case "text":
			texts = append(texts, m.content)
		case "tool":
			tools = append(tools, m.content)
		}
	}
	if len(texts) == 0 {
		t.Errorf("expected at least one agent text message, got none")
	}
	if len(tools) == 0 {
		t.Fatalf("expected a tool (commandExecution) message, got none")
	}
	if !strings.Contains(tools[0], "echo hi") || !strings.Contains(tools[0], "exit 0") {
		t.Errorf("tool message missing command/exit: %q", tools[0])
	}
	if len(sink.contextUpdates) == 0 {
		t.Errorf("expected context_left updates from thread/tokenUsage/updated, got none")
	}
	for _, pct := range sink.contextUpdates {
		if pct < 0 || pct > 100 {
			t.Errorf("context_left pct out of range: %d", pct)
		}
	}
	if !sawTurnStarted || !sawTurnCompleted {
		t.Errorf("turn lifecycle signals not seen: started=%v completed=%v", sawTurnStarted, sawTurnCompleted)
	}
	if sink.bumpCount == 0 {
		t.Errorf("expected heartbeat bumps, got 0")
	}
}

func TestDispatchAppServer_CommandFormatting(t *testing.T) {
	exit := 2
	it := appServerItem{Type: "commandExecution", Command: "/bin/zsh -lc 'ls'", AggregatedOutput: "a\nb\n", ExitCode: &exit}
	got := formatAppServerCommand(it)
	if !strings.HasPrefix(got, "[Bash] /bin/zsh -lc 'ls' (exit 2)") {
		t.Errorf("unexpected format: %q", got)
	}
	if !strings.Contains(got, "a\nb\n") {
		t.Errorf("output not included: %q", got)
	}
	// Truncation at 4KB.
	big := appServerItem{Type: "commandExecution", Command: "x", AggregatedOutput: strings.Repeat("z", 5000)}
	gotBig := formatAppServerCommand(big)
	if !strings.Contains(gotBig, "…(truncated)") || len(gotBig) > 4200 {
		t.Errorf("expected truncation, len=%d", len(gotBig))
	}
}

func TestDispatchAppServer_RateLimitTyped(t *testing.T) {
	sink := &opencodeTestSink{}
	// Routine usage update: rateLimitReachedType null → NOT rate-limited.
	clean := rpcEnvelope{Method: "account/rateLimits/updated", Params: json.RawMessage(`{"rateLimits":{"rateLimitReachedType":null,"primary":{"usedPercent":2}}}`)}
	if dispatchAppServerEvent("s", clean, sink, 200000).rateLimited {
		t.Error("usage update wrongly classified as rate-limited")
	}
	// Reached: rateLimitReachedType non-null → rate-limited.
	hit := rpcEnvelope{Method: "account/rateLimits/updated", Params: json.RawMessage(`{"rateLimits":{"rateLimitReachedType":"primary"}}`)}
	if !dispatchAppServerEvent("s", hit, sink, 200000).rateLimited {
		t.Error("reached limit not classified as rate-limited")
	}
}

func TestDispatchAppServer_ErrorClassification(t *testing.T) {
	sink := &opencodeTestSink{}
	limit := rpcEnvelope{Method: "error", Params: json.RawMessage(`{"error":{"message":"Rate limit exceeded, try later"}}`)}
	sig := dispatchAppServerEvent("s", limit, sink, 200000)
	if !sig.rateLimited {
		t.Errorf("limit-pattern error not classified as rate-limited: %+v", sig)
	}
	boom := rpcEnvelope{Method: "error", Params: json.RawMessage(`{"error":{"message":"internal boom"}}`)}
	sig = dispatchAppServerEvent("s", boom, sink, 200000)
	if sig.fatalErr != "internal boom" || sig.rateLimited {
		t.Errorf("generic error misclassified: %+v", sig)
	}
}

func TestDispatchAppServer_DeltaHeartbeatOnly(t *testing.T) {
	sink := &opencodeTestSink{}
	d := rpcEnvelope{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"itemId":"m1","delta":"hi"}`)}
	dispatchAppServerEvent("s", d, sink, 200000)
	if len(sink.recordedMsgs) != 0 {
		t.Errorf("delta should not create a message row, got %+v", sink.recordedMsgs)
	}
	if sink.bumpCount != 1 {
		t.Errorf("delta should bump heartbeat once, got %d", sink.bumpCount)
	}
}

func TestDispatchAppServer_TokenUsageContextLeft(t *testing.T) {
	sink := &opencodeTestSink{}
	// Multi-turn: total is cumulative (27279), last is current context (9115).
	// context_left must derive from `last`, not the cumulative `total`.
	n := rpcEnvelope{Method: "thread/tokenUsage/updated", Params: json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":27279},"last":{"inputTokens":9115},"modelContextWindow":258400}}`)}
	dispatchAppServerEvent("s", n, sink, 200000)
	if len(sink.contextUpdates) != 1 {
		t.Fatalf("expected 1 context update, got %d", len(sink.contextUpdates))
	}
	// From last=9115: 100 - 100*9115/258400 ≈ 96. (From total=27279 it would be ~89 — wrong.)
	if pct := sink.contextUpdates[0]; pct < 95 || pct > 97 {
		t.Errorf("context_left pct = %d, want ~96 (must use last, not cumulative total)", pct)
	}
}

func TestDispatchAppServer_TokenUsageSingleTurnFallback(t *testing.T) {
	sink := &opencodeTestSink{}
	// No `last` block → fall back to total (single-turn, where they coincide).
	n := rpcEnvelope{Method: "thread/tokenUsage/updated", Params: json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":9091},"modelContextWindow":258400}}`)}
	dispatchAppServerEvent("s", n, sink, 200000)
	if len(sink.contextUpdates) != 1 || sink.contextUpdates[0] < 95 || sink.contextUpdates[0] > 97 {
		t.Errorf("single-turn fallback context_left = %v, want ~96", sink.contextUpdates)
	}
}
