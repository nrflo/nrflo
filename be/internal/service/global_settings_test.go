package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func setupGlobalSettingsTestEnv(t *testing.T) *GlobalSettingsService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "global_settings_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewGlobalSettingsService(pool, clock.Real())
}

func TestGlobalSettings_Get_MissingKey(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	val, err := svc.Get("nonexistent_key")
	if err != nil {
		t.Fatalf("Get(%q) returned error: %v", "nonexistent_key", err)
	}
	if val != "" {
		t.Errorf("Get(%q) = %q, want empty string", "nonexistent_key", val)
	}
}

func TestGlobalSettings_Get_ReturnsSetValue(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	if err := svc.Set("my_key", "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := svc.Get("my_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get(%q) = %q, want %q", "my_key", val, "hello")
	}
}

func TestGlobalSettings_Set_Upsert(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	if err := svc.Set("upsert_key", "initial"); err != nil {
		t.Fatalf("Set initial: %v", err)
	}

	if err := svc.Set("upsert_key", "updated"); err != nil {
		t.Fatalf("Set updated: %v", err)
	}

	val, err := svc.Get("upsert_key")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if val != "updated" {
		t.Errorf("Get after upsert = %q, want %q", val, "updated")
	}
}

func TestGlobalSettings_MultipleKeys_Independent(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	keys := map[string]string{
		"key_alpha": "value_alpha",
		"key_beta":  "value_beta",
		"key_gamma": "value_gamma",
	}

	for k, v := range keys {
		if err := svc.Set(k, v); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}

	for k, want := range keys {
		got, err := svc.Get(k)
		if err != nil {
			t.Fatalf("Get(%q): %v", k, err)
		}
		if got != want {
			t.Errorf("Get(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestGlobalSettings_LowConsumptionMode_DefaultFalse(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	val, err := svc.Get("low_consumption_mode")
	if err != nil {
		t.Fatalf("Get low_consumption_mode: %v", err)
	}
	// Missing key returns "" which the API handler interprets as false.
	if val == "true" {
		t.Errorf("low_consumption_mode default = %q, want empty (false)", val)
	}
}

func TestGlobalSettings_LowConsumptionMode_SetAndGet(t *testing.T) {
	svc := setupGlobalSettingsTestEnv(t)

	// Enable
	if err := svc.Set("low_consumption_mode", "true"); err != nil {
		t.Fatalf("Set true: %v", err)
	}
	val, err := svc.Get("low_consumption_mode")
	if err != nil {
		t.Fatalf("Get after enable: %v", err)
	}
	if val != "true" {
		t.Errorf("low_consumption_mode after enable = %q, want %q", val, "true")
	}

	// Disable
	if err := svc.Set("low_consumption_mode", "false"); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	val, err = svc.Get("low_consumption_mode")
	if err != nil {
		t.Fatalf("Get after disable: %v", err)
	}
	if val != "false" {
		t.Errorf("low_consumption_mode after disable = %q, want %q", val, "false")
	}
}
