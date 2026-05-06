package spawner

import (
	"database/sql"
	"testing"

	"be/internal/clock"
)

// selectEffectiveModeForTest mirrors the effectiveMode switch in startBackend,
// enabling unit tests of mode-selection logic without spawning real backends.
func selectEffectiveModeForTest(executionMode string, interactiveCLI bool, adapter CLIAdapter) string {
	switch {
	case executionMode == "api":
		return "api"
	case executionMode == "script":
		return "script"
	case interactiveCLI && adapter != nil && adapter.SupportsInteractive():
		return "cli_interactive"
	default:
		return "cli"
	}
}

func TestEffectiveMode_SelectionMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		executionMode  string
		interactiveCLI bool
		adapter        CLIAdapter
		want           string
	}{
		{
			name:           "api + interactiveOff → api",
			executionMode:  "api",
			interactiveCLI: false,
			adapter:        &ClaudeAdapter{},
			want:           "api",
		},
		{
			name:           "api + interactiveOn → api (api wins over interactive)",
			executionMode:  "api",
			interactiveCLI: true,
			adapter:        &ClaudeAdapter{},
			want:           "api",
		},
		{
			name:           "script → script",
			executionMode:  "script",
			interactiveCLI: false,
			adapter:        nil,
			want:           "script",
		},
		{
			name:           "script + interactiveOn → script (script wins)",
			executionMode:  "script",
			interactiveCLI: true,
			adapter:        &ClaudeAdapter{},
			want:           "script",
		},
		{
			name:           "cli + interactiveOn + SupportsInteractive → cli_interactive",
			executionMode:  "cli",
			interactiveCLI: true,
			adapter:        &ClaudeAdapter{},
			want:           "cli_interactive",
		},
		{
			name:           "cli + interactiveOff + SupportsInteractive → cli",
			executionMode:  "cli",
			interactiveCLI: false,
			adapter:        &ClaudeAdapter{},
			want:           "cli",
		},
		{
			name:           "cli + interactiveOn + !SupportsInteractive → cli",
			executionMode:  "cli",
			interactiveCLI: true,
			adapter:        &mockNoInteractiveAdapter{},
			want:           "cli",
		},
		{
			name:           "cli + interactiveOn + nil adapter → cli",
			executionMode:  "cli",
			interactiveCLI: true,
			adapter:        nil,
			want:           "cli",
		},
		{
			name:           "empty executionMode → cli (default)",
			executionMode:  "",
			interactiveCLI: false,
			adapter:        &ClaudeAdapter{},
			want:           "cli",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := selectEffectiveModeForTest(tc.executionMode, tc.interactiveCLI, tc.adapter)
			if got != tc.want {
				t.Errorf("selectEffectiveModeForTest(%q, interactiveCLI=%v, adapter=%T) = %q, want %q",
					tc.executionMode, tc.interactiveCLI, tc.adapter, got, tc.want)
			}
		})
	}
}

// TestEffectiveMode_ConsistentWithBackendSelector verifies that the effectiveMode switch
// and the backend selector (tested in backend_interactive_selector_test.go) agree on
// which "bucket" a given combination falls into.
func TestEffectiveMode_ConsistentWithBackendSelector(t *testing.T) {
	t.Parallel()
	type pair struct {
		executionMode  string
		interactiveCLI bool
		adapter        CLIAdapter
		wantMode       string
		wantBackend    string
	}
	cases := []pair{
		{"api", false, &ClaudeAdapter{}, "api", "api"},
		{"api", true, &ClaudeAdapter{}, "api", "api"},
		{"cli", true, &ClaudeAdapter{}, "cli_interactive", "cli_interactive"},
		{"cli", false, &ClaudeAdapter{}, "cli", "cli"},
		{"cli", true, &mockNoInteractiveAdapter{}, "cli", "cli"},
	}
	for _, c := range cases {
		s := New(Config{Clock: clock.Real(), InteractiveCLIMode: c.interactiveCLI})
		gotMode := selectEffectiveModeForTest(c.executionMode, c.interactiveCLI, c.adapter)
		gotBackend := selectBackendForTest(s, c.executionMode, c.adapter)
		if gotMode != c.wantMode {
			t.Errorf("mode(%q, interactive=%v, %T) = %q, want %q",
				c.executionMode, c.interactiveCLI, c.adapter, gotMode, c.wantMode)
		}
		if gotBackend.Name() != c.wantBackend {
			t.Errorf("backend(%q, interactive=%v, %T) = %q, want %q",
				c.executionMode, c.interactiveCLI, c.adapter, gotBackend.Name(), c.wantBackend)
		}
	}
}

// TestRegisterAgentStart_EffectiveModePersisted verifies that registerAgentStart
// writes the effectiveMode parameter into agent_sessions.effective_mode.
func TestRegisterAgentStart_EffectiveModePersisted(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)
	s := New(Config{Pool: pool, Clock: clock.Real()})

	s.registerAgentStart(
		"proj", "T-1", "feature", "wfi-1",
		"agent-em", "implementor", 0,
		"sess-em-persist", "sonnet", "phase1",
		"", "", "", "", "tok-em-persist",
		"cli_interactive",
		0, 25,
	)

	var effectiveMode sql.NullString
	if err := pool.QueryRow(`SELECT effective_mode FROM agent_sessions WHERE id = ?`, "sess-em-persist").Scan(&effectiveMode); err != nil {
		t.Fatalf("query effective_mode: %v", err)
	}
	if !effectiveMode.Valid {
		t.Errorf("effective_mode is NULL, want cli_interactive")
	} else if effectiveMode.String != "cli_interactive" {
		t.Errorf("effective_mode = %q, want cli_interactive", effectiveMode.String)
	}
}

// TestRegisterAgentStart_AllEffectiveModes verifies all four mode values are
// persisted correctly by registerAgentStart.
func TestRegisterAgentStart_AllEffectiveModes(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)

	modes := []string{"cli", "cli_interactive", "api", "script"}
	for i, mode := range modes {
		sessionID := "sess-allmode-" + mode
		spawnToken := "tok-allmode-" + mode
		s := New(Config{Pool: pool, Clock: clock.Real()})
		s.registerAgentStart(
			"proj", "T-1", "feature", "wfi-1",
			"agent-"+mode, "implementor", 0,
			sessionID, "sonnet", "phase1",
			"", "", "", "", spawnToken,
			mode,
			i, 25,
		)

		var got sql.NullString
		if err := pool.QueryRow(`SELECT effective_mode FROM agent_sessions WHERE id = ?`, sessionID).Scan(&got); err != nil {
			t.Fatalf("mode=%q: query: %v", mode, err)
		}
		if !got.Valid {
			t.Errorf("mode=%q: effective_mode is NULL, want %q", mode, mode)
		} else if got.String != mode {
			t.Errorf("mode=%q: effective_mode = %q, want %q", mode, got.String, mode)
		}
	}
}
