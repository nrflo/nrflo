package spawner

import (
	"testing"
)

// coerceExecutionModeForTest mirrors the ProviderModes coercion block in prepareSpawn,
// enabling table-driven unit tests without spawning real agents.
func coerceExecutionModeForTest(providerModes map[string][]string, cliName, mode string) string {
	if mode != "cli" && mode != "cli_interactive" {
		return mode
	}
	if providerModes == nil {
		return mode
	}
	allowed, ok := providerModes[cliName]
	if !ok || len(allowed) == 0 {
		return mode
	}
	for _, m := range allowed {
		if m == mode {
			return mode
		}
	}
	return allowed[0]
}

func TestProviderModes_Coercion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		providerModes map[string][]string
		cliName       string
		mode          string
		want          string
	}{
		{
			name:          "nil map — no coercion",
			providerModes: nil,
			cliName:       "claude",
			mode:          "cli",
			want:          "cli",
		},
		{
			name:          "provider not in map — no coercion",
			providerModes: map[string][]string{"opencode": {"cli"}},
			cliName:       "claude",
			mode:          "cli_interactive",
			want:          "cli_interactive",
		},
		{
			name:          "cli in allowlist [cli cli_interactive] — unchanged",
			providerModes: map[string][]string{"claude": {"cli", "cli_interactive"}},
			cliName:       "claude",
			mode:          "cli",
			want:          "cli",
		},
		{
			name:          "cli_interactive in allowlist — unchanged",
			providerModes: map[string][]string{"claude": {"cli", "cli_interactive"}},
			cliName:       "claude",
			mode:          "cli_interactive",
			want:          "cli_interactive",
		},
		{
			name:          "cli not in allowlist [cli_interactive] — coerced to cli_interactive",
			providerModes: map[string][]string{"claude": {"cli_interactive"}},
			cliName:       "claude",
			mode:          "cli",
			want:          "cli_interactive",
		},
		{
			name:          "cli_interactive not in allowlist [cli] — coerced to cli",
			providerModes: map[string][]string{"claude": {"cli"}},
			cliName:       "claude",
			mode:          "cli_interactive",
			want:          "cli",
		},
		{
			name:          "api mode bypasses allowlist check",
			providerModes: map[string][]string{"claude": {"cli_interactive"}},
			cliName:       "claude",
			mode:          "api",
			want:          "api",
		},
		{
			name:          "script mode bypasses allowlist check",
			providerModes: map[string][]string{"claude": {"cli_interactive"}},
			cliName:       "claude",
			mode:          "script",
			want:          "script",
		},
		{
			name:          "opencode cli not in allowlist — coerced",
			providerModes: map[string][]string{"opencode": {"cli_interactive"}},
			cliName:       "opencode",
			mode:          "cli",
			want:          "cli_interactive",
		},
		{
			name:          "codex cli_interactive in allowlist — unchanged",
			providerModes: map[string][]string{"codex": {"cli", "cli_interactive"}},
			cliName:       "codex",
			mode:          "cli_interactive",
			want:          "cli_interactive",
		},
		{
			name:          "empty allowlist — no coercion",
			providerModes: map[string][]string{"claude": {}},
			cliName:       "claude",
			mode:          "cli",
			want:          "cli",
		},
		{
			name:          "opencode cli in allowlist [cli] — unchanged",
			providerModes: map[string][]string{"opencode": {"cli"}},
			cliName:       "opencode",
			mode:          "cli",
			want:          "cli",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := coerceExecutionModeForTest(tc.providerModes, tc.cliName, tc.mode)
			if got != tc.want {
				t.Errorf("coerceExecutionMode(%v, %q, %q) = %q, want %q",
					tc.providerModes, tc.cliName, tc.mode, got, tc.want)
			}
		})
	}
}
