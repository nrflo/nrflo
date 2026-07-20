package spawner

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// rateLimitOnceProvider fails the first Run with a 429 sdk error, then
// answers every later Run with end_turn text.
type rateLimitOnceProvider struct {
	mu   sync.Mutex
	runs int
}

func (p *rateLimitOnceProvider) Name() string                { return "rl-once" }
func (p *rateLimitOnceProvider) MaxContext(model string) int { return 200000 }

func (p *rateLimitOnceProvider) Run(_ context.Context, _ provider.Request, _ provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	p.runs++
	first := p.runs == 1
	p.mu.Unlock()
	if first {
		req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
		return nil, &sdk.Error{StatusCode: 429, Request: req, Response: &http.Response{StatusCode: 429}}
	}
	return &provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "recovered"}},
	}, nil
}

// TestAPIConsoleEngine_RateLimitedTurn_RetriesAndCompletes verifies the
// bounded in-turn retry: a 429 on the first provider call emits a system
// retry row and the turn still ends in EventTurnCompleted after the resumed
// call succeeds — instead of surfacing an EventError like other failures.
func TestAPIConsoleEngine_RateLimitedTurn_RetriesAndCompletes(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := &rateLimitOnceProvider{}
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	eng.rateLimitBackoff = []time.Duration{time.Millisecond}
	if err := eng.Start(context.Background(), apiTestSpec("sess-rl-retry")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), UserTurn{Text: "hello"}); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	waitForEventType(t, eng.Events(), EventTurnCompleted, 5*time.Second)

	prov.mu.Lock()
	runs := prov.runs
	prov.mu.Unlock()
	if runs != 2 {
		t.Errorf("provider runs = %d, want 2 (initial + one retry)", runs)
	}

	sink.mu.Lock()
	msgs := append([]recordedMsg(nil), sink.recordedMsgs...)
	sink.mu.Unlock()
	foundRetry := false
	for _, m := range msgs {
		if m.category == "system" && strings.Contains(m.content, "retrying in") {
			foundRetry = true
		}
	}
	if !foundRetry {
		t.Errorf("expected a system 'retrying in' row, got %+v", msgs)
	}
}
