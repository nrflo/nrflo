package socket

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/ws"
)

// globalTestClient registers a client that captures all global WS events.
func globalTestClient(t *testing.T, hub *ws.Hub, id string) <-chan []byte {
	t.Helper()
	client, sendCh := ws.NewTestClient(hub, id)
	hub.Register(client)
	return sendCh
}

// TestHandleGlobal_ClaudeLimitsUpdate_HappyPath_BothPcts verifies the happy path:
// both pcts present → service persists values, BroadcastGlobal fires with correct event.
func TestHandleGlobal_ClaudeLimitsUpdate_HappyPath_BothPcts(t *testing.T) {
	env := newHandlerTestEnv(t)
	sendCh := globalTestClient(t, env.hub, "global-listener-both")

	fivePct := 42.5
	sevenPct := 80.0
	params := map[string]interface{}{
		"five_hour_pct":       fivePct,
		"five_hour_resets_at": "2026-05-11T05:00:00Z",
		"seven_day_pct":       sevenPct,
		"seven_day_resets_at": "2026-05-18T05:00:00Z",
	}
	paramsData, _ := json.Marshal(params)

	req := Request{
		ID:     "req-global-1",
		Method: "global.claude_limits_update",
		Params: paramsData,
	}

	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result["status"] != "updated" {
		t.Errorf("status = %q, want %q", result["status"], "updated")
	}

	// Verify global broadcast is emitted.
	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventGlobalClaudeLimitsUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventGlobalClaudeLimitsUpdated)
		}
		// Verify snake_case payload fields are present.
		if _, ok := event.Data["five_hour_pct"]; !ok {
			t.Error("event.Data missing five_hour_pct")
		}
		if _, ok := event.Data["seven_day_pct"]; !ok {
			t.Error("event.Data missing seven_day_pct")
		}
		if _, ok := event.Data["five_hour_resets_at"]; !ok {
			t.Error("event.Data missing five_hour_resets_at")
		}
		if _, ok := event.Data["seven_day_resets_at"]; !ok {
			t.Error("event.Data missing seven_day_resets_at")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for global.claude_limits_updated broadcast")
	}
}

// TestHandleGlobal_ClaudeLimitsUpdate_OnlyFiveHourPct verifies that providing only
// five_hour_pct is valid and broadcasts without seven_day_pct.
func TestHandleGlobal_ClaudeLimitsUpdate_OnlyFiveHourPct(t *testing.T) {
	env := newHandlerTestEnv(t)
	sendCh := globalTestClient(t, env.hub, "global-listener-5h")

	params := map[string]interface{}{
		"five_hour_pct":       33.3,
		"five_hour_resets_at": "2026-05-11T05:00:00Z",
		"seven_day_resets_at": "",
	}
	paramsData, _ := json.Marshal(params)

	req := Request{ID: "req-5h", Method: "global.claude_limits_update", Params: paramsData}
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error with only five_hour_pct, got: %v", resp.Error)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventGlobalClaudeLimitsUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventGlobalClaudeLimitsUpdated)
		}
		if _, ok := event.Data["five_hour_pct"]; !ok {
			t.Error("event.Data missing five_hour_pct when only five_hour_pct provided")
		}
		if _, ok := event.Data["seven_day_pct"]; ok {
			t.Error("event.Data should NOT contain seven_day_pct when not provided")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

// TestHandleGlobal_ClaudeLimitsUpdate_OnlySevenDayPct verifies that providing only
// seven_day_pct is valid.
func TestHandleGlobal_ClaudeLimitsUpdate_OnlySevenDayPct(t *testing.T) {
	env := newHandlerTestEnv(t)
	sendCh := globalTestClient(t, env.hub, "global-listener-7d")

	params := map[string]interface{}{
		"seven_day_pct":       65.0,
		"seven_day_resets_at": "2026-05-18T05:00:00Z",
	}
	paramsData, _ := json.Marshal(params)

	req := Request{ID: "req-7d", Method: "global.claude_limits_update", Params: paramsData}
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("expected no error with only seven_day_pct, got: %v", resp.Error)
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}
		if event.Type != ws.EventGlobalClaudeLimitsUpdated {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventGlobalClaudeLimitsUpdated)
		}
		if _, ok := event.Data["seven_day_pct"]; !ok {
			t.Error("event.Data missing seven_day_pct when only seven_day_pct provided")
		}
		if _, ok := event.Data["five_hour_pct"]; ok {
			t.Error("event.Data should NOT contain five_hour_pct when not provided")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

// TestHandleGlobal_ClaudeLimitsUpdate_MissingBothPcts verifies validation error when
// neither pct is provided.
func TestHandleGlobal_ClaudeLimitsUpdate_MissingBothPcts(t *testing.T) {
	env := newHandlerTestEnv(t)

	params := map[string]interface{}{
		"five_hour_resets_at": "2026-05-11T05:00:00Z",
		"seven_day_resets_at": "2026-05-18T05:00:00Z",
	}
	paramsData, _ := json.Marshal(params)

	req := Request{ID: "req-noboth", Method: "global.claude_limits_update", Params: paramsData}
	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected validation error when both pcts missing, got success")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %d, want %d (ErrCodeValidation)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestHandleGlobal_ClaudeLimitsUpdate_ServicePersists verifies values are written to DB.
func TestHandleGlobal_ClaudeLimitsUpdate_ServicePersists(t *testing.T) {
	env := newHandlerTestEnv(t)

	params := map[string]interface{}{
		"five_hour_pct":       55.0,
		"five_hour_resets_at": "2026-05-11T10:00:00Z",
		"seven_day_pct":       77.5,
		"seven_day_resets_at": "2026-05-18T10:00:00Z",
	}
	paramsData, _ := json.Marshal(params)
	req := Request{ID: "req-persist", Method: "global.claude_limits_update", Params: paramsData}
	resp := env.handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("handle error: %v", resp.Error)
	}

	// Verify via DB directly.
	var val string
	if err := env.pool.QueryRow(`SELECT value FROM config WHERE key = 'claude_5h_used_pct'`).Scan(&val); err != nil {
		t.Fatalf("query claude_5h_used_pct: %v", err)
	}
	if val == "" {
		t.Error("claude_5h_used_pct not persisted after handle")
	}

	if err := env.pool.QueryRow(`SELECT value FROM config WHERE key = 'claude_weekly_used_pct'`).Scan(&val); err != nil {
		t.Fatalf("query claude_weekly_used_pct: %v", err)
	}
	if val == "" {
		t.Error("claude_weekly_used_pct not persisted after handle")
	}
}

// TestHandleGlobal_UnknownAction verifies unknown global actions return MethodNotFound.
func TestHandleGlobal_UnknownAction(t *testing.T) {
	env := newHandlerTestEnv(t)
	req := Request{
		ID:     "req-unknown",
		Method: "global.nonexistent_action",
		Params: []byte("{}"),
	}
	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown global action")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("code = %d, want %d (MethodNotFound)", resp.Error.Code, ErrCodeMethodNotFound)
	}
}
