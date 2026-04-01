package db

import (
	"path/filepath"
	"testing"
)

// TestPoolGetProjectConfig_NotFound returns empty string for missing key.
func TestPoolGetProjectConfig_NotFound(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	val, err := pool.GetProjectConfig("myproject", "nonexistent_key")
	if err != nil {
		t.Fatalf("GetProjectConfig() error: %v", err)
	}
	if val != "" {
		t.Errorf("GetProjectConfig(nonexistent) = %q, want empty string", val)
	}
}

// TestPoolSetProjectConfig_RoundTrip verifies set and get return the same value.
func TestPoolSetProjectConfig_RoundTrip(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const projectID = "test-project"
	const key = "claude_safety_hook"
	const value = `{"enabled":true,"hook":"/usr/local/bin/safety-check"}`

	if err := pool.SetProjectConfig(projectID, key, value); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	got, err := pool.GetProjectConfig(projectID, key)
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}
	if got != value {
		t.Errorf("GetProjectConfig(%q,%q) = %q, want %q", projectID, key, got, value)
	}
}

// TestPoolSetProjectConfig_UpdateExisting verifies INSERT OR REPLACE updates value.
func TestPoolSetProjectConfig_UpdateExisting(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.SetProjectConfig("proj", "key1", "original"); err != nil {
		t.Fatalf("SetProjectConfig original: %v", err)
	}
	if err := pool.SetProjectConfig("proj", "key1", "updated"); err != nil {
		t.Fatalf("SetProjectConfig updated: %v", err)
	}

	val, err := pool.GetProjectConfig("proj", "key1")
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}
	if val != "updated" {
		t.Errorf("GetProjectConfig after update = %q, want %q", val, "updated")
	}
}

// TestPoolGetProjectConfig_IsolatedFromGlobal verifies project config does not
// leak into global config and vice versa.
func TestPoolGetProjectConfig_IsolatedFromGlobal(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pool.SetConfig("shared_key", "global_value"); err != nil {
		t.Fatalf("SetConfig global: %v", err)
	}
	if err := pool.SetProjectConfig("myproj", "shared_key", "project_value"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}

	// Global read should not see project value.
	globalVal, err := pool.GetConfig("shared_key")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if globalVal != "global_value" {
		t.Errorf("GetConfig(shared_key) = %q, want %q", globalVal, "global_value")
	}

	// Project read should not see global value.
	projVal, err := pool.GetProjectConfig("myproj", "shared_key")
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}
	if projVal != "project_value" {
		t.Errorf("GetProjectConfig(shared_key) = %q, want %q", projVal, "project_value")
	}
}

// TestPoolGlobalConfig_BackwardCompat verifies existing SetConfig/GetConfig still
// work after the migration that added project_id to the config table.
func TestPoolGlobalConfig_BackwardCompat(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	cases := []struct{ key, value string }{
		{"low_consumption_mode", "false"},
		{"stall_start_timeout_sec", "90"},
		{"stall_running_timeout_sec", "300"},
	}
	for _, tc := range cases {
		if err := pool.SetConfig(tc.key, tc.value); err != nil {
			t.Fatalf("SetConfig(%q): %v", tc.key, err)
		}
	}
	for _, tc := range cases {
		got, err := pool.GetConfig(tc.key)
		if err != nil {
			t.Fatalf("GetConfig(%q): %v", tc.key, err)
		}
		if got != tc.value {
			t.Errorf("GetConfig(%q) = %q, want %q", tc.key, got, tc.value)
		}
	}
}

// TestPoolGetConfig_NotFound returns empty string for missing global key.
func TestPoolGetConfig_NotFound(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	val, err := pool.GetConfig("this_key_does_not_exist")
	if err != nil {
		t.Fatalf("GetConfig(missing) error: %v", err)
	}
	if val != "" {
		t.Errorf("GetConfig(missing) = %q, want empty string", val)
	}
}

// TestDBGetProjectConfig_NotFound returns empty string for missing key (DB type).
func TestDBGetProjectConfig_NotFound(t *testing.T) {
	database, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	val, err := database.GetProjectConfig("proj", "missing")
	if err != nil {
		t.Fatalf("GetProjectConfig() error: %v", err)
	}
	if val != "" {
		t.Errorf("GetProjectConfig(missing) = %q, want empty string", val)
	}
}

// TestDBSetProjectConfig_RoundTrip verifies DB.SetProjectConfig/GetProjectConfig.
func TestDBSetProjectConfig_RoundTrip(t *testing.T) {
	database, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.SetProjectConfig("projX", "hook_config", "json-data"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	val, err := database.GetProjectConfig("projX", "hook_config")
	if err != nil {
		t.Fatalf("GetProjectConfig: %v", err)
	}
	if val != "json-data" {
		t.Errorf("GetProjectConfig = %q, want %q", val, "json-data")
	}
}

// TestDBGlobalConfig_BackwardCompat verifies DB.SetConfig/GetConfig after migration.
func TestDBGlobalConfig_BackwardCompat(t *testing.T) {
	database, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.SetConfig("low_consumption_mode", "true"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	val, err := database.GetConfig("low_consumption_mode")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "true" {
		t.Errorf("GetConfig = %q, want %q", val, "true")
	}
}
