package service

import (
	"strings"
	"testing"
)

func TestProviderSettings_SetModes_OpencodeRejectsCLIInteractive(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	cases := []struct {
		name  string
		modes []string
	}{
		{"cli_interactive only", []string{"cli_interactive"}},
		{"cli and cli_interactive", []string{"cli", "cli_interactive"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := svc.SetModes("opencode", tc.modes)
			if err == nil {
				t.Fatalf("SetModes(opencode, %v) = nil, want error", tc.modes)
			}
			if !strings.Contains(err.Error(), "opencode does not support") {
				t.Errorf("SetModes(opencode, %v) error = %q, want to contain %q", tc.modes, err.Error(), "opencode does not support")
			}
		})
	}
}

func TestProviderSettings_SetModes_OpencodeCLISucceeds(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	if err := svc.SetModes("opencode", []string{"cli"}); err != nil {
		t.Fatalf("SetModes(opencode, [cli]) = %v, want nil", err)
	}
	got, err := svc.GetModes("opencode")
	if err != nil {
		t.Fatalf("GetModes(opencode): %v", err)
	}
	if len(got) != 1 || got[0] != "cli" {
		t.Errorf("GetModes(opencode) = %v, want [cli]", got)
	}
}

func TestProviderSettings_ClaudeCodexStillAcceptCLIInteractive(t *testing.T) {
	t.Parallel()
	svc := setupProviderSettingsTestEnv(t)

	for _, provider := range []string{"claude", "codex"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			if err := svc.SetModes(provider, []string{"cli", "cli_interactive"}); err != nil {
				t.Fatalf("SetModes(%q, [cli, cli_interactive]) = %v, want nil", provider, err)
			}
			got, err := svc.GetModes(provider)
			if err != nil {
				t.Fatalf("GetModes(%q): %v", provider, err)
			}
			if len(got) != 2 || got[0] != "cli" || got[1] != "cli_interactive" {
				t.Errorf("GetModes(%q) = %v, want [cli cli_interactive]", provider, got)
			}
		})
	}
}
