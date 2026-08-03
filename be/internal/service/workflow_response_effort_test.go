package service

import (
	"testing"

	"be/internal/db"
)

// insertSessionResolvedEffort sets resolved_effort on a session already
// inserted via insertSession. Pass effort="" to leave the column at its
// TEXT NOT NULL DEFAULT ” value (legacy row).
func insertSessionResolvedEffort(t *testing.T, pool *db.Pool, id, effort string) {
	t.Helper()
	if effort == "" {
		return
	}
	if _, err := pool.Exec(`UPDATE agent_sessions SET resolved_effort = ? WHERE id = ?`, effort, id); err != nil {
		t.Fatalf("insertSessionResolvedEffort(%s, %q): %v", id, effort, err)
	}
}

// checkResolvedEffort asserts the resolved_effort field is present (or
// absent) in m. want="" means the field must be absent from the map.
func checkResolvedEffort(t *testing.T, m map[string]interface{}, want string) {
	t.Helper()
	got, present := m["resolved_effort"]
	if want == "" {
		if present {
			t.Errorf("resolved_effort = %v, want absent for empty/default row", got)
		}
		return
	}
	if !present {
		t.Errorf("resolved_effort absent, want %q", want)
		return
	}
	if got != want {
		t.Errorf("resolved_effort = %v, want %q", got, want)
	}
}

// TestBuildActiveAgentsMap_ResolvedEffort verifies resolved_effort is
// surfaced for a running session when set, and omitted (not emitted as "")
// when the column holds its default empty-string value.
func TestBuildActiveAgentsMap_ResolvedEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		effort string // empty → leave column at default ''
	}{
		{"high", "high"},
		{"low", "low"},
		{"default_empty_absent", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			insertSession(t, pool, "s1", wfiID, "eff-agent", "running", "", "")
			insertSessionResolvedEffort(t, pool, "s1", tc.effort)

			result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
			m := getAgentEntry(t, result, "eff-agent")
			checkResolvedEffort(t, m, tc.effort)
		})
	}
}

// TestBuildAgentHistory_ResolvedEffort verifies resolved_effort is surfaced
// in a finished session's history entry when set, and omitted when the
// column holds its default empty-string value.
func TestBuildAgentHistory_ResolvedEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		effort string // empty → leave column at default ''
	}{
		{"high", "high"},
		{"low", "low"},
		{"default_empty_absent", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			insertSession(t, pool, "sh1", wfiID, "eff-h-agent", "completed", "pass", "")
			insertSessionResolvedEffort(t, pool, "sh1", tc.effort)

			history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
			if len(history) != 1 {
				t.Fatalf("buildAgentHistory len = %d, want 1", len(history))
			}
			entry, ok := history[0].(map[string]interface{})
			if !ok {
				t.Fatalf("buildAgentHistory[0] = %T, want map", history[0])
			}
			checkResolvedEffort(t, entry, tc.effort)
		})
	}
}
