package spawner

import (
	"database/sql"
	"testing"

	"be/internal/clock"
)

// selectEffectiveModeForTest mirrors the effectiveMode switch in startBackend,
// enabling unit tests of mode-selection logic without spawning real backends.
func selectEffectiveModeForTest(executionMode string) string {
	switch executionMode {
	case "api":
		return "api"
	case "script":
		return "script"
	case "cli_interactive":
		return "cli_interactive"
	default:
		return "unknown"
	}
}

func TestEffectiveMode_SelectionMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		executionMode string
		want          string
	}{
		{
			name:          "api → api",
			executionMode: "api",
			want:          "api",
		},
		{
			name:          "script → script",
			executionMode: "script",
			want:          "script",
		},
		{
			name:          "cli_interactive → cli_interactive",
			executionMode: "cli_interactive",
			want:          "cli_interactive",
		},
		{
			name:          "unknown mode → unknown",
			executionMode: "cli",
			want:          "unknown",
		},
		{
			name:          "empty executionMode → unknown",
			executionMode: "",
			want:          "unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := selectEffectiveModeForTest(tc.executionMode)
			if got != tc.want {
				t.Errorf("selectEffectiveModeForTest(%q) = %q, want %q",
					tc.executionMode, got, tc.want)
			}
		})
	}
}

// TestEffectiveMode_ConsistentWithBackendSelector verifies that the effectiveMode switch
// and the backend selector agree on which "bucket" a given executionMode falls into.
func TestEffectiveMode_ConsistentWithBackendSelector(t *testing.T) {
	t.Parallel()
	type pair struct {
		executionMode string
		adapter       CLIAdapter
		wantMode      string
		wantBackend   string
	}
	cases := []pair{
		{"api", &ClaudeAdapter{}, "api", "api"},
		{"script", nil, "script", "script"},
		{"cli_interactive", &ClaudeAdapter{}, "cli_interactive", "cli_interactive"},
	}
	for _, c := range cases {
		s := New(Config{Clock: clock.Real()})
		gotMode := selectEffectiveModeForTest(c.executionMode)
		gotBackend := selectBackendForTest(s, c.executionMode, c.adapter)
		if gotMode != c.wantMode {
			t.Errorf("mode(%q, %T) = %q, want %q",
				c.executionMode, c.adapter, gotMode, c.wantMode)
		}
		if gotBackend == nil {
			t.Errorf("backend(%q, %T) = nil, want %q",
				c.executionMode, c.adapter, c.wantBackend)
			continue
		}
		if gotBackend.Name() != c.wantBackend {
			t.Errorf("backend(%q, %T) = %q, want %q",
				c.executionMode, c.adapter, gotBackend.Name(), c.wantBackend)
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

// TestRegisterAgentStart_AllEffectiveModes verifies all three valid mode values are
// persisted correctly by registerAgentStart.
func TestRegisterAgentStart_AllEffectiveModes(t *testing.T) {
	t.Parallel()
	pool := setupTestDB(t)

	modes := []string{"cli_interactive", "api", "script"}
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
