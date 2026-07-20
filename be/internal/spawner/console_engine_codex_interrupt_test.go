package spawner

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCodexEngine_InterruptTurn_UsesThreadAndTurnIDs(t *testing.T) {
	eng, fake := startTestCodexEngine(t, &testSink{}, EngineSpec{})
	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "work"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	paramsCh := make(chan json.RawMessage, 1)
	fake.setOverride("turn/interrupt", func(fake *fakeAppServer, env rpcEnvelope) {
		paramsCh <- env.Params
		fake.replyResult(*env.ID, `{}`)
	})
	if err := eng.InterruptTurn(context.Background()); err != nil {
		t.Fatalf("InterruptTurn: %v", err)
	}
	params := mustRecvParams(t, paramsCh)
	var got struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	if err := json.Unmarshal(params, &got); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got.ThreadID == "" || got.TurnID != "turn-test-1" {
		t.Fatalf("interrupt params = %+v", got)
	}
}

func TestCodexEngine_InterruptTurn_Idle(t *testing.T) {
	eng, _ := startTestCodexEngine(t, &testSink{}, EngineSpec{})
	if err := eng.InterruptTurn(context.Background()); err != ErrNoActiveTurn {
		t.Fatalf("InterruptTurn = %v, want ErrNoActiveTurn", err)
	}
}
