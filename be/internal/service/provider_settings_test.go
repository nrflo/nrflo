package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func setupProviderSettingsTestEnv(t *testing.T) *ProviderSettingsService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "provider_settings_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	gs := NewGlobalSettingsService(pool, clock.Real())
	return NewProviderSettingsService(gs)
}

func TestProviderSettings_GetModes_DefaultWhenAbsent(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	cases := []struct {
		provider    string
		wantModes   []string
	}{
		{"claude", []string{"cli", "cli_interactive"}},
		{"codex", []string{"cli", "cli_interactive"}},
		{"opencode", []string{"cli"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			modes, err := svc.GetModes(tc.provider)
			if err != nil {
				t.Fatalf("GetModes(%q): %v", tc.provider, err)
			}
			if len(modes) != len(tc.wantModes) {
				t.Fatalf("GetModes(%q) = %v, want %v", tc.provider, modes, tc.wantModes)
			}
			for i, m := range tc.wantModes {
				if modes[i] != m {
					t.Errorf("GetModes(%q)[%d] = %q, want %q", tc.provider, i, modes[i], m)
				}
			}
		})
	}
}

func TestProviderSettings_SetGetModes_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		provider string
		modes    []string
	}{
		{"claude", []string{"cli"}},
		{"codex", []string{"cli_interactive"}},
		{"opencode", []string{"cli"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			svc := setupProviderSettingsTestEnv(t)
			if err := svc.SetModes(tc.provider, tc.modes); err != nil {
				t.Fatalf("SetModes(%q, %v): %v", tc.provider, tc.modes, err)
			}
			got, err := svc.GetModes(tc.provider)
			if err != nil {
				t.Fatalf("GetModes(%q): %v", tc.provider, err)
			}
			if len(got) != len(tc.modes) {
				t.Fatalf("GetModes(%q) len = %d, want %d", tc.provider, len(got), len(tc.modes))
			}
			for i, m := range tc.modes {
				if got[i] != m {
					t.Errorf("GetModes(%q)[%d] = %q, want %q", tc.provider, i, got[i], m)
				}
			}
		})
	}
}

func TestProviderSettings_SetModes_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		provider string
		modes    []string
		wantErr  string
	}{
		{
			name:     "unknown provider",
			provider: "gpt4",
			modes:    []string{"cli"},
			wantErr:  "invalid provider",
		},
		{
			name:     "empty modes slice",
			provider: "claude",
			modes:    []string{},
			wantErr:  "must not be empty",
		},
		{
			name:     "nil modes",
			provider: "claude",
			modes:    nil,
			wantErr:  "must not be empty",
		},
		{
			name:     "unknown mode string",
			provider: "claude",
			modes:    []string{"api"},
			wantErr:  "invalid mode",
		},
		{
			name:     "script mode rejected",
			provider: "codex",
			modes:    []string{"script"},
			wantErr:  "invalid mode",
		},
		{
			name:     "valid mixed with invalid mode",
			provider: "opencode",
			modes:    []string{"cli", "api"},
			wantErr:  "invalid mode",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := setupProviderSettingsTestEnv(t)
			err := svc.SetModes(tc.provider, tc.modes)
			if err == nil {
				t.Fatalf("SetModes(%q, %v) = nil, want error containing %q", tc.provider, tc.modes, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("SetModes(%q, %v) error = %q, want to contain %q", tc.provider, tc.modes, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestProviderSettings_SetModes_Dedupe(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	if err := svc.SetModes("claude", []string{"cli", "cli", "cli_interactive", "cli"}); err != nil {
		t.Fatalf("SetModes: %v", err)
	}
	got, err := svc.GetModes("claude")
	if err != nil {
		t.Fatalf("GetModes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetModes after dedupe len = %d, want 2; got %v", len(got), got)
	}
	if got[0] != "cli" || got[1] != "cli_interactive" {
		t.Errorf("GetModes after dedupe = %v, want [cli cli_interactive]", got)
	}
}

func TestProviderSettings_SetModes_DedupeToSingle(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	if err := svc.SetModes("codex", []string{"cli_interactive", "cli_interactive"}); err != nil {
		t.Fatalf("SetModes: %v", err)
	}
	got, err := svc.GetModes("codex")
	if err != nil {
		t.Fatalf("GetModes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetModes after dedupe len = %d, want 1; got %v", len(got), got)
	}
	if got[0] != "cli_interactive" {
		t.Errorf("GetModes after dedupe = %v, want [cli_interactive]", got)
	}
}

func TestProviderSettings_GetAll_DefaultsAllThree(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	all, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != len(AllProviders) {
		t.Fatalf("GetAll len = %d, want %d", len(all), len(AllProviders))
	}
	wantDefaults := map[string][]string{
		"claude":   {"cli", "cli_interactive"},
		"codex":    {"cli", "cli_interactive"},
		"opencode": {"cli"},
	}
	for p, want := range wantDefaults {
		modes, ok := all[p]
		if !ok {
			t.Errorf("GetAll missing provider %q", p)
			continue
		}
		if len(modes) != len(want) {
			t.Errorf("GetAll[%q] = %v, want %v", p, modes, want)
			continue
		}
		for i, m := range want {
			if modes[i] != m {
				t.Errorf("GetAll[%q][%d] = %q, want %q", p, i, modes[i], m)
			}
		}
	}
}

func TestProviderSettings_GetAll_ReflectsSetModes(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	if err := svc.SetModes("claude", []string{"cli"}); err != nil {
		t.Fatalf("SetModes claude: %v", err)
	}
	// opencode only accepts cli; cli_interactive is rejected.
	if err := svc.SetModes("opencode", []string{"cli"}); err != nil {
		t.Fatalf("SetModes opencode: %v", err)
	}

	all, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	if got := all["claude"]; len(got) != 1 || got[0] != "cli" {
		t.Errorf("GetAll[claude] = %v, want [cli]", got)
	}
	if got := all["opencode"]; len(got) != 1 || got[0] != "cli" {
		t.Errorf("GetAll[opencode] = %v, want [cli]", got)
	}
	if got := all["codex"]; len(got) != 2 || got[0] != "cli" || got[1] != "cli_interactive" {
		t.Errorf("GetAll[codex] = %v, want [cli cli_interactive] (unchanged default)", got)
	}
}

// Opencode-specific mode enforcement tests are in provider_settings_opencode_test.go.
