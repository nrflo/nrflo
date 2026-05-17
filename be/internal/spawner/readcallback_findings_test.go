package spawner

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

func setFindings(t *testing.T, env *testEnv, m map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal findings: %v", err)
	}
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateFindings(env.sessionID, string(b)); err != nil {
		t.Fatalf("UpdateFindings: %v", err)
	}
}

func TestReadCallbackFindings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		findings     map[string]interface{}
		wantMode     string
		wantLevel    int
		wantAgent    string
		wantChain    []string
	}{
		{
			name: "mode=layer explicit with level",
			findings: map[string]interface{}{
				"callback_mode":  "layer",
				"callback_level": float64(2),
			},
			wantMode:  "layer",
			wantLevel: 2,
		},
		{
			name: "mode=agent with target",
			findings: map[string]interface{}{
				"callback_mode":   "agent",
				"callback_target": "target-agent",
			},
			wantMode:  "agent",
			wantAgent: "target-agent",
		},
		{
			name: "mode=chain with JSON array",
			findings: map[string]interface{}{
				"callback_mode":  "chain",
				"callback_chain": []interface{}{"p1", "p2"},
			},
			wantMode:  "chain",
			wantChain: []string{"p1", "p2"},
		},
		{
			name: "comma-sep string chain without explicit mode defaults to layer",
			findings: map[string]interface{}{
				"callback_chain": "p1,p2",
			},
			wantMode:  "layer",
			wantChain: []string{"p1", "p2"},
		},
		{
			name:      "no callback_mode key defaults to layer",
			findings:  map[string]interface{}{"callback_level": float64(1)},
			wantMode:  "layer",
			wantLevel: 1,
		},
		{
			name:     "callback_level as float64 is converted correctly",
			findings: map[string]interface{}{"callback_mode": "layer", "callback_level": float64(5)},
			wantMode:  "layer",
			wantLevel: 5,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := setupTestEnv(t)
			defer env.cleanup()

			env.createSession(t, "claude:sonnet")
			setFindings(t, env, tc.findings)

			proc := &processInfo{sessionID: env.sessionID}
			cb := env.spawner.readCallbackFindings(proc)

			if cb.Mode != tc.wantMode {
				t.Errorf("Mode: got %q, want %q", cb.Mode, tc.wantMode)
			}
			if cb.Level != tc.wantLevel {
				t.Errorf("Level: got %d, want %d", cb.Level, tc.wantLevel)
			}
			if cb.TargetAgent != tc.wantAgent {
				t.Errorf("TargetAgent: got %q, want %q", cb.TargetAgent, tc.wantAgent)
			}
			if len(cb.Chain) != len(tc.wantChain) {
				t.Errorf("Chain len: got %d, want %d (%v)", len(cb.Chain), len(tc.wantChain), cb.Chain)
			} else {
				for i, v := range tc.wantChain {
					if cb.Chain[i] != v {
						t.Errorf("Chain[%d]: got %q, want %q", i, cb.Chain[i], v)
					}
				}
			}
		})
	}
}

func TestReadCallbackFindings_NilPool(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	// Remove the pool so the nil-pool branch is exercised.
	env.spawner.config.Pool = nil

	proc := &processInfo{sessionID: env.sessionID}
	cb := env.spawner.readCallbackFindings(proc)

	if cb == nil {
		t.Fatal("expected non-nil CallbackError")
	}
	if cb.Mode != "layer" {
		t.Errorf("Mode: got %q, want %q", cb.Mode, "layer")
	}
}
