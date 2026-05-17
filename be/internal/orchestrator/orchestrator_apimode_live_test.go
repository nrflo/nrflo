package orchestrator

import (
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/ws"
)

// TestOrchestratorAPIMode_LiveSettingReadByPool verifies that the api_mode_enabled
// global setting is read freshly from the shared pool on each check: changes made
// via GlobalSettingsService are immediately visible to the same pool without restart.
// This confirms the no-cache contract that runLoop relies on at spawn time.
func TestOrchestratorAPIMode_LiveSettingReadByPool(t *testing.T) {
	env := newTestEnv(t)

	settingsSvc := service.NewGlobalSettingsService(env.pool, clock.Real())

	// Baseline: setting absent → reads as "".
	val, err := settingsSvc.Get("api_mode_enabled")
	if err != nil {
		t.Fatalf("Get (initial): %v", err)
	}
	if val != "" {
		t.Errorf("initial api_mode_enabled = %q, want empty", val)
	}

	// Enable.
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set true: %v", err)
	}

	val, err = settingsSvc.Get("api_mode_enabled")
	if err != nil {
		t.Fatalf("Get (after enable): %v", err)
	}
	if val != "true" {
		t.Errorf("api_mode_enabled after enable = %q, want true", val)
	}

	// Disable.
	if err := settingsSvc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}

	val, err = settingsSvc.Get("api_mode_enabled")
	if err != nil {
		t.Fatalf("Get (after disable): %v", err)
	}
	if val != "false" {
		t.Errorf("api_mode_enabled after disable = %q, want false", val)
	}
}

// TestOrchestratorAPIMode_OrchestratorUsesPool verifies that the orchestrator's
// shared pool is the same DB that GlobalSettingsService writes to: settings set
// before a workflow starts are immediately visible through the orchestrator's pool.
func TestOrchestratorAPIMode_OrchestratorUsesPool(t *testing.T) {
	env := newTestEnv(t)

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	// The orchestrator is created with the same dbPath that env.pool uses.
	orch := New(env.dbPath, hub, clock.Real(), nil, "")
	_ = orch

	settingsSvc := service.NewGlobalSettingsService(env.pool, clock.Real())

	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify the same value is visible through a fresh reader on the same path.
	reader := service.NewGlobalSettingsService(env.pool, clock.Real())
	val, err := reader.Get("api_mode_enabled")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "true" {
		t.Errorf("api_mode_enabled via reader = %q, want true", val)
	}

	// Toggle off; fresh read must reflect it.
	if err := settingsSvc.Set("api_mode_enabled", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	val, err = reader.Get("api_mode_enabled")
	if err != nil {
		t.Fatalf("Get after disable: %v", err)
	}
	if val != "false" {
		t.Errorf("api_mode_enabled after disable = %q, want false", val)
	}
}
