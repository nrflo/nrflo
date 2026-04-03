package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeConfigFile writes cfg JSON to $tmpHome/.nrflow/config.json.
func writeConfigFile(t *testing.T, tmpHome string, content string) {
	t.Helper()
	dir := filepath.Join(tmpHome, DefaultConfigDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, DefaultConfigFile), []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// withTempHome redirects HOME to a fresh temp dir for the duration of the test.
// Uses t.Setenv so HOME is restored and the test cannot run in parallel.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	return tmpHome
}

// --- DefaultConfig tests ---

func TestDefaultConfig_HostIsLocalhost(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Host != DefaultHost {
		t.Errorf("DefaultConfig().Server.Host = %q, want %q", cfg.Server.Host, DefaultHost)
	}
}

func TestDefaultConfig_PortIsDefaultPort(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != DefaultPort {
		t.Errorf("DefaultConfig().Server.Port = %d, want %d", cfg.Server.Port, DefaultPort)
	}
}

func TestDefaultHostConstant(t *testing.T) {
	if DefaultHost != "127.0.0.1" {
		t.Errorf("DefaultHost = %q, want %q", DefaultHost, "127.0.0.1")
	}
}

func TestDefaultPortConstant(t *testing.T) {
	if DefaultPort != 6587 {
		t.Errorf("DefaultPort = %d, want 6587", DefaultPort)
	}
}

// --- Load() tests ---

func TestLoad_MissingFile_ReturnsDefault(t *testing.T) {
	withTempHome(t) // no config file written

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Server.Host = %q, want %q (default)", cfg.Server.Host, DefaultHost)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Server.Port = %d, want %d (default)", cfg.Server.Port, DefaultPort)
	}
}

func TestLoad_BackfillsEmptyHost(t *testing.T) {
	tmpHome := withTempHome(t)
	writeConfigFile(t, tmpHome, `{"server":{"host":"","port":8080}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Server.Host = %q, want %q after backfill", cfg.Server.Host, DefaultHost)
	}
}

func TestLoad_BackfillsZeroPort(t *testing.T) {
	tmpHome := withTempHome(t)
	writeConfigFile(t, tmpHome, `{"server":{"host":"10.0.0.1","port":0}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Server.Port = %d, want %d after backfill", cfg.Server.Port, DefaultPort)
	}
}

func TestLoad_PreservesCustomHost(t *testing.T) {
	cases := []struct {
		name string
		host string
	}{
		{"all interfaces", "0.0.0.0"},
		{"specific IP", "192.168.1.50"},
		{"localhost string", "localhost"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpHome := withTempHome(t)
			content, _ := json.Marshal(map[string]interface{}{
				"server": map[string]interface{}{"host": tc.host, "port": 6587},
			})
			writeConfigFile(t, tmpHome, string(content))

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Server.Host != tc.host {
				t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, tc.host)
			}
		})
	}
}

func TestLoad_PreservesCustomPort(t *testing.T) {
	tmpHome := withTempHome(t)
	writeConfigFile(t, tmpHome, `{"server":{"host":"127.0.0.1","port":9090}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoad_InvalidJSON_ReturnsError(t *testing.T) {
	tmpHome := withTempHome(t)
	writeConfigFile(t, tmpHome, `{not valid json`)

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for invalid JSON, got nil")
	}
}

func TestLoad_EmptyJSON_UsesDefaults(t *testing.T) {
	tmpHome := withTempHome(t)
	writeConfigFile(t, tmpHome, `{}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Server.Host = %q, want %q for empty JSON", cfg.Server.Host, DefaultHost)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("Server.Port = %d, want %d for empty JSON", cfg.Server.Port, DefaultPort)
	}
}

func TestLoad_ServerSectionOnly_BackfillsHost(t *testing.T) {
	tmpHome := withTempHome(t)
	// Only port set in server section, no host key at all
	writeConfigFile(t, tmpHome, `{"server":{"port":7777}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != DefaultHost {
		t.Errorf("Server.Host = %q, want %q when host key absent", cfg.Server.Host, DefaultHost)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port = %d, want 7777", cfg.Server.Port)
	}
}
