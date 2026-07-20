package spawner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// toolEnvTestHandler is a minimal apirun.ToolHandler double: records Invoke
// calls and returns a canned result.
type toolEnvTestHandler struct {
	name   string
	output string
}

func (h *toolEnvTestHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: h.name, InputSchema: json.RawMessage(`{}`)}
}

func (h *toolEnvTestHandler) Invoke(_ context.Context, _ apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	return h.output, false, nil
}

// TestAPIConsoleEngine_ToolUse_DispatchesHandlerAndPersistsToolUseIDPayload
// verifies a tool_use turn invokes the injected Registry handler and the
// invoke row's payload carries tool_use_id (asserted on testSink); the
// pre-seeded agent_messages row for the same tool_use_id gets its ended_at
// stamped via the real DB pool (CloseToolSpan, output_tool_span.go's shape).
func TestAPIConsoleEngine_ToolUse_DispatchesHandlerAndPersistsToolUseIDPayload(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	const sessionID = "sess-tool"
	seedAPIEngineSession(t, pool, clk, "p1", sessionID)

	// Seed the invoke row CloseToolSpan will close, mirroring the row
	// TrackToolInvoke would have persisted through a real (non-fake) sink.
	msgRepo := repo.NewAgentMessageRepo(pool, clk)
	if err := msgRepo.InsertBatch(sessionID, []repo.MessageEntry{
		{Content: "[my_tool] {}", Category: "tool", Payload: `{"tool_use_id":"tu_1"}`},
	}); err != nil {
		t.Fatalf("seed invoke row: %v", err)
	}

	handler := &toolEnvTestHandler{name: "my_tool", output: "tool ran"}
	prov := mock.New(
		mock.Script{
			Events: []mock.SinkEvent{
				{Kind: mock.EventToolUseStart, ToolUseID: "tu_1", ToolName: "my_tool"},
				{Kind: mock.EventToolUseStop, ToolUseID: "tu_1", FullInput: json.RawMessage(`{"arg":"val"}`)},
			},
			Final: provider.FinalResponse{
				StopReason: "tool_use",
				Content: []provider.ContentBlock{
					{Type: "tool_use", ToolUseID: "tu_1", ToolName: "my_tool", Input: json.RawMessage(`{"arg":"val"}`)},
				},
			},
		},
		mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
	)
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API: APIEngineDeps{
			Pool:     pool,
			Clock:    clk,
			Handlers: apirun.Registry{"my_tool": handler},
		},
	})
	if err := eng.Start(context.Background(), apiTestSpec(sessionID)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "use the tool"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	// The streaming sink's invoke row (through testSink) must carry the
	// tool_use_id in its payload.
	sink.mu.Lock()
	var foundPayload string
	for _, m := range sink.recordedMsgs {
		if m.category == "tool" && strings.Contains(m.payload, "tu_1") {
			foundPayload = m.payload
		}
	}
	sink.mu.Unlock()
	if foundPayload == "" {
		t.Fatalf("no tool-invoke row with tool_use_id in payload; recordedMsgs = %+v", sink.recordedMsgs)
	}
	var payloadObj map[string]any
	if err := json.Unmarshal([]byte(foundPayload), &payloadObj); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payloadObj["tool_use_id"] != "tu_1" {
		t.Errorf("payload tool_use_id = %v, want tu_1", payloadObj["tool_use_id"])
	}
	input, ok := payloadObj["input"].(map[string]any)
	if !ok || input["arg"] != "val" {
		t.Errorf("payload input = %v, want {arg: val}", payloadObj["input"])
	}

	// CloseToolSpan must have stamped ended_at on the pre-seeded DB row.
	rows, err := msgRepo.GetBySessionPaginatedFiltered(sessionID, "tool", 10, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginatedFiltered: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("tool rows = %d, want 1", len(rows))
	}
	if !strings.Contains(rows[0].Payload, "ended_at") {
		t.Errorf("row payload = %q, want ended_at stamped", rows[0].Payload)
	}
}
