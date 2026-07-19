package spawner

// Tests that apiConsoleEngine.Start renders its conversation system prompt
// through the api-console-system-prompt injectable — a distinct, intentionally
// unseeded id (migration 000177 seeds only the worker "api-system-prompt"), so
// a fresh DB falls back to the console-specific consoleAPISystem/FSSystem
// constants. No t.Parallel() — these share the package-level
// newConsoleAPIProvider seam via installFakeAPIProvider.

import (
	"context"
	"sync"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun/provider"
)

// recordingAPIProvider records the System field of every Run() request so
// tests can assert what the engine actually sent, without a network call.
type recordingAPIProvider struct {
	mu      sync.Mutex
	systems []string
}

func (p *recordingAPIProvider) Name() string          { return "recording" }
func (p *recordingAPIProvider) MaxContext(string) int { return 1000 }

func (p *recordingAPIProvider) Run(_ context.Context, req provider.Request, _ provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	p.systems = append(p.systems, req.System)
	p.mu.Unlock()
	return &provider.FinalResponse{StopReason: "end_turn"}, nil
}

func (p *recordingAPIProvider) lastSystem() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.systems) == 0 {
		return ""
	}
	return p.systems[len(p.systems)-1]
}

// TestAPIConsoleEngine_SystemPrompt_RendersCustomInjectable verifies that
// after seeding a custom api-console-system-prompt row, Start renders it (with
// the engine's PROJECT_ID/MODEL/NODE_ID vars) and the provider receives it as
// Request.System.
func TestAPIConsoleEngine_SystemPrompt_RendersCustomInjectable(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	if _, err := pool.Exec(`INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at)
		VALUES ('api-console-system-prompt', 'API console system prompt', 'CUSTOM-CONSOLE-SYS', 'CUSTOM-CONSOLE-SYS', 0, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed custom api-console-system-prompt: %v", err)
	}

	prov := &recordingAPIProvider{}
	installFakeAPIProvider(t, prov, nil)

	sessionID := "sess-sysprompt-custom"
	seedAPIEngineSession(t, pool, clk, "p1", sessionID)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	spec := apiTestSpec(sessionID)
	if err := eng.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	rendered := renderAPISystemPrompt(context.Background(), pool, "api-console-system-prompt", map[string]string{
		"PROJECT_ID": spec.ProjectID,
		"MODEL":      spec.Model,
		"NODE_ID":    spec.SessionID,
	}, consoleAPISystem)

	if prov.lastSystem() != rendered {
		t.Errorf("captured System = %q, want rendered injectable %q", prov.lastSystem(), rendered)
	}
	if prov.lastSystem() != "CUSTOM-CONSOLE-SYS" {
		t.Errorf("captured System = %q, want exactly the seeded custom template", prov.lastSystem())
	}
}

// TestAPIConsoleEngine_SystemPrompt_FreshDBFallsBackToConsoleAPISystem is the
// regression guard for the shared-row trap: on a fresh DB the console's own
// api-console-system-prompt row does not exist (migration 000177 seeds only the
// worker api-system-prompt), so Start must fall back to the console-specific
// consoleAPISystem constant — NOT the seeded worker default.
func TestAPIConsoleEngine_SystemPrompt_FreshDBFallsBackToConsoleAPISystem(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}

	prov := &recordingAPIProvider{}
	installFakeAPIProvider(t, prov, nil)

	sessionID := "sess-sysprompt-freshdb"
	seedAPIEngineSession(t, pool, clk, "p1", sessionID)

	eng := newAPIConsoleEngine(EngineDeps{Sink: &testSink{}, API: APIEngineDeps{Pool: pool, Clock: clk}})
	if err := eng.Start(context.Background(), apiTestSpec(sessionID)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	waitForEventType(t, eng.Events(), EventTurnCompleted, 2*time.Second)

	if prov.lastSystem() != consoleAPISystem {
		t.Errorf("captured System = %q, want fallback consoleAPISystem %q", prov.lastSystem(), consoleAPISystem)
	}
	if prov.lastSystem() == defaultAPISystemPrompt {
		t.Errorf("console captured the seeded WORKER default %q instead of the console prompt", defaultAPISystemPrompt)
	}
}
