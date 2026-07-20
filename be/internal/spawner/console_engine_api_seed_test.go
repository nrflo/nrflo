package spawner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"
)

// recordingProvider implements provider.Provider and captures every Run
// call's Request so a test can inspect exactly what text reached the
// provider — the strongest feasible proof the rotation-carried digest
// (EngineSpec.SeededContext) reaches the api engine's first request. The
// integration package cannot reach this seam: apiConsoleEngine/
// newConsoleAPIProvider are spawner-private, so this acceptance test lives
// here instead of internal/integration.
type recordingProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

func (p *recordingProvider) Name() string          { return "recording" }
func (p *recordingProvider) MaxContext(string) int { return 100000 }

func (p *recordingProvider) Run(_ context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	sink.OnTextDelta("ok")
	sink.OnUsage(provider.Usage{InputTokens: 1, OutputTokens: 1})
	return &provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: "ok"}},
	}, nil
}

func (p *recordingProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.Request, len(p.reqs))
	copy(out, p.reqs)
	return out
}

// firstUserText returns the text of req's first "user" message's first text
// block, or "" when there is none.
func firstUserText(t *testing.T, req provider.Request) string {
	t.Helper()
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Content {
			if b.Type == "text" {
				return b.Text
			}
		}
	}
	return ""
}

// lastUserText returns the text of req's LAST "user" message's first text
// block — the newest turn appended by Conversation.SendTurn, as opposed to
// firstUserText which picks up earlier history still carried in req.Messages.
func lastUserText(t *testing.T, req provider.Request) string {
	t.Helper()
	var out string
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Content {
			if b.Type == "text" {
				out = b.Text
			}
		}
	}
	return out
}

// TestAPIConsoleEngine_SeededContext_ReachesFirstProviderRequestOnly is the
// rotation-digest-seeding acceptance test (fake provider, no real API): a
// rotated apiConsoleEngine's EngineSpec.SeededContext must reach the
// PROVIDER-visible text of the first SendUserTurn's request alongside the
// user's own text, and must NOT reappear on a later turn (consumed once).
func TestAPIConsoleEngine_SeededContext_ReachesFirstProviderRequestOnly(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := &recordingProvider{}
	installFakeAPIProvider(t, prov, nil)

	eng := newAPIConsoleEngine(EngineDeps{
		Sink: &testSink{},
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})

	spec := apiTestSpec("sess-seeded")
	spec.SeededContext = "CARRY-FWD-DIGEST-XYZ"
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), "user question"); err != nil {
		t.Fatalf("SendUserTurn 1: %v", err)
	}
	_ = waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	if err := eng.SendUserTurn(context.Background(), "second question"); err != nil {
		t.Fatalf("SendUserTurn 2: %v", err)
	}
	_ = waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	reqs := prov.requests()
	if len(reqs) != 2 {
		t.Fatalf("provider Run calls = %d, want 2", len(reqs))
	}

	firstText := firstUserText(t, reqs[0])
	if !strings.Contains(firstText, "CARRY-FWD-DIGEST-XYZ") {
		t.Errorf("first request user text = %q, want it to contain the seeded digest", firstText)
	}
	if !strings.Contains(firstText, "user question") {
		t.Errorf("first request user text = %q, want it to contain the user's turn text", firstText)
	}

	secondText := lastUserText(t, reqs[1])
	if strings.Contains(secondText, "CARRY-FWD-DIGEST-XYZ") {
		t.Errorf("second request user text = %q, must NOT re-contain the digest (consumed once)", secondText)
	}
	if !strings.Contains(secondText, "second question") {
		t.Errorf("second request user text = %q, want it to contain the new turn text", secondText)
	}
}
