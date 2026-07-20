package spawner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"
)

// blockingProvider blocks Run() until release is closed, letting a test hold
// a turn "in flight" to exercise ErrTurnActive/Stop deterministically.
type blockingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingProvider() *blockingProvider {
	return &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
}

func (p *blockingProvider) Name() string                { return "blocking" }
func (p *blockingProvider) MaxContext(model string) int { return 200000 }
func (p *blockingProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	final := provider.FinalResponse{StopReason: "end_turn"}
	return &final, nil
}

// TestAPIConsoleEngine_SendUserTurn_WhileTurnActive_ReturnsErrTurnActive
// verifies a second SendUserTurn while a turn is in flight is rejected.
func TestAPIConsoleEngine_SendUserTurn_WhileTurnActive_ReturnsErrTurnActive(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := newBlockingProvider()
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-active")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "first"}); err != nil {
		t.Fatalf("first SendUserTurn: %v", err)
	}
	select {
	case <-prov.started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider never started")
	}

	err := eng.SendUserTurn(context.Background(), UserTurn{Text: "second"})
	if !errors.Is(err, ErrTurnActive) {
		t.Errorf("second SendUserTurn error = %v, want ErrTurnActive", err)
	}

	close(prov.release)
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)
}

// TestAPIConsoleEngine_SendUserTurn_AfterStop_ReturnsErrEngineStopped verifies
// a message racing Close/StopAll is rejected instead of starting a turn on a
// torn-down engine. Without the stopped guard this is a turnWG.Add concurrent
// with turnWG.Wait, and the turn_started emit sends on the closed events
// channel — a panic, not an error.
func TestAPIConsoleEngine_SendUserTurn_AfterStop_ReturnsErrEngineStopped(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	installFakeAPIProvider(t, newBlockingProvider(), nil)

	eng := newAPIConsoleEngine(EngineDeps{
		Sink: &testSink{},
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-after-stop")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	eng.Stop()

	err := eng.SendUserTurn(context.Background(), UserTurn{Text: "too late"})
	if !errors.Is(err, ErrEngineStopped) {
		t.Errorf("SendUserTurn after Stop = %v, want ErrEngineStopped", err)
	}
}

// TestAPIConsoleEngine_Stop_ClosesEventsAndCancelsInFlightTurn verifies Stop
// closes Events() and unblocks/cancels an in-flight turn without a
// send-on-closed-channel panic. Run with -race.
func TestAPIConsoleEngine_Stop_ClosesEventsAndCancelsInFlightTurn(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := newBlockingProvider()
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-stop")); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "in flight"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	select {
	case <-prov.started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider never started")
	}

	// A non-draining consumer: Stop is called with the queued turn_started
	// event still sitting in the buffer and never read until after Stop.
	stopDone := make(chan struct{})
	go func() {
		eng.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s (turn goroutine wedged on the undrained buffer?)")
	}

	// Draining to completion must terminate in a closed channel (no
	// send-on-closed-channel panic, and no event arrives after Stop finished
	// beyond whatever was already queued before it began).
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-eng.Events():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("Events() never closed after Stop")
		}
	}
}
