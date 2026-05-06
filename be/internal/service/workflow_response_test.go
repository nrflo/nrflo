package service

import (
	"testing"

	"be/internal/db"
)

// insertSessionMode sets effective_mode on a session already inserted via insertSession.
// Pass mode="" to leave effective_mode as NULL (legacy row).
func insertSessionMode(t *testing.T, pool *db.Pool, id, mode string) {
	t.Helper()
	if mode == "" {
		return
	}
	if _, err := pool.Exec(`UPDATE agent_sessions SET effective_mode = ? WHERE id = ?`, mode, id); err != nil {
		t.Fatalf("insertSessionMode(%s, %q): %v", id, mode, err)
	}
}

// getAgentEntry retrieves an entry from the active-agents result map as map[string]interface{}.
func getAgentEntry(t *testing.T, result map[string]interface{}, agentType string) map[string]interface{} {
	t.Helper()
	entry, ok := result[agentType]
	if !ok {
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		t.Fatalf("buildActiveAgentsMap: key %q not found; got keys: %v", agentType, keys)
	}
	m, ok := entry.(map[string]interface{})
	if !ok {
		t.Fatalf("buildActiveAgentsMap[%q] = %T, want map[string]interface{}", agentType, entry)
	}
	return m
}

// checkEffectiveMode asserts the effective_mode field is present (or absent) in m.
// want="" means the field must be absent from the map.
func checkEffectiveMode(t *testing.T, m map[string]interface{}, want string) {
	t.Helper()
	got, present := m["effective_mode"]
	if want == "" {
		if present {
			t.Errorf("effective_mode = %v, want absent for legacy NULL/empty row", got)
		}
		return
	}
	if !present {
		t.Errorf("effective_mode absent, want %q", want)
		return
	}
	if got != want {
		t.Errorf("effective_mode = %v, want %q", got, want)
	}
}

// TestBuildActiveAgentsMap_EffectiveMode verifies that effective_mode is correctly
// surfaced in the active-agents map for each supported value, and absent for NULL rows.
func TestBuildActiveAgentsMap_EffectiveMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string // empty → NULL (legacy)
	}{
		{"cli", "cli"},
		{"cli_interactive", "cli_interactive"},
		{"api", "api"},
		{"script", "script"},
		{"legacy_null_absent", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			insertSession(t, pool, "s1", wfiID, "em-agent", "running", "pass", "")
			insertSessionMode(t, pool, "s1", tc.mode)

			result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
			m := getAgentEntry(t, result, "em-agent")
			checkEffectiveMode(t, m, tc.mode)
		})
	}
}

// TestBuildActiveAgentsMap_EmptyStringOmitted verifies that an explicit empty-string
// effective_mode is treated the same as NULL — the field must be absent.
func TestBuildActiveAgentsMap_EmptyStringOmitted(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "s1", wfiID, "em-agent", "running", "", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET effective_mode = '' WHERE id = 's1'`); err != nil {
		t.Fatalf("set empty effective_mode: %v", err)
	}

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	m := getAgentEntry(t, result, "em-agent")
	checkEffectiveMode(t, m, "")
}

// TestBuildActiveAgentsMap_AllModes verifies all four effective_mode values and one
// legacy NULL row appear correctly in the same map.
func TestBuildActiveAgentsMap_AllModes(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	sessions := []struct{ agentType, mode string }{
		{"em-cli", "cli"},
		{"em-cli-int", "cli_interactive"},
		{"em-api", "api"},
		{"em-script", "script"},
		{"em-legacy", ""},
	}
	for _, s := range sessions {
		insertSession(t, pool, "s-"+s.agentType, wfiID, s.agentType, "running", "", "")
		insertSessionMode(t, pool, "s-"+s.agentType, s.mode)
	}

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if len(result) != len(sessions) {
		t.Errorf("buildActiveAgentsMap len = %d, want %d", len(result), len(sessions))
	}
	for _, s := range sessions {
		m := getAgentEntry(t, result, s.agentType)
		checkEffectiveMode(t, m, s.mode)
	}
}

// TestBuildActiveAgentsMap_RunningOnlyIncluded verifies that only running sessions
// appear in the active-agents map.
func TestBuildActiveAgentsMap_RunningOnlyIncluded(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s-done", wfiID, "am-done", "completed", "pass", "")
	insertSessionMode(t, pool, "s-done", "api")
	insertSession(t, pool, "s-running", wfiID, "am-running", "running", "", "")
	insertSessionMode(t, pool, "s-running", "cli")

	result := svc.buildActiveAgentsMap(wfiID, map[string][]RestartDetail{})
	if _, ok := result["am-done"]; ok {
		t.Error("completed session must not appear in buildActiveAgentsMap")
	}
	m := getAgentEntry(t, result, "am-running")
	checkEffectiveMode(t, m, "cli")
}

// TestBuildAgentHistory_EffectiveMode verifies effective_mode in agent history entries
// for each supported value, and absent for NULL rows.
func TestBuildAgentHistory_EffectiveMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode string // empty → NULL (legacy)
	}{
		{"cli", "cli"},
		{"cli_interactive", "cli_interactive"},
		{"api", "api"},
		{"script", "script"},
		{"legacy_null_absent", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, wfiID := setupDeriveTestEnv(t)
			insertSession(t, pool, "sh1", wfiID, "h-agent", "completed", "pass", "")
			insertSessionMode(t, pool, "sh1", tc.mode)

			history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
			if len(history) != 1 {
				t.Fatalf("buildAgentHistory len = %d, want 1", len(history))
			}
			entry, ok := history[0].(map[string]interface{})
			if !ok {
				t.Fatalf("buildAgentHistory[0] = %T, want map", history[0])
			}
			checkEffectiveMode(t, entry, tc.mode)
		})
	}
}

// TestBuildAgentHistory_EmptyStringOmitted verifies that an explicit empty-string
// effective_mode is absent from the history entry.
func TestBuildAgentHistory_EmptyStringOmitted(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)
	insertSession(t, pool, "sh1", wfiID, "h-agent", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET effective_mode = '' WHERE id = 'sh1'`); err != nil {
		t.Fatalf("set empty effective_mode: %v", err)
	}

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1", len(history))
	}
	entry := history[0].(map[string]interface{})
	checkEffectiveMode(t, entry, "")
}

// TestBuildAgentHistory_RunningAndContinuedExcluded verifies that running and
// continued sessions are excluded from agent history.
func TestBuildAgentHistory_RunningAndContinuedExcluded(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "sh-running", wfiID, "h-running", "running", "", "")
	insertSessionMode(t, pool, "sh-running", "api")
	insertSession(t, pool, "sh-cont", wfiID, "h-cont", "continued", "", "")
	insertSessionMode(t, pool, "sh-cont", "cli")
	insertSession(t, pool, "sh-done", wfiID, "h-done", "completed", "pass", "")
	insertSessionMode(t, pool, "sh-done", "cli_interactive")

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != 1 {
		t.Fatalf("buildAgentHistory len = %d, want 1 (running/continued excluded)", len(history))
	}
	entry := history[0].(map[string]interface{})
	if entry["agent_type"] != "h-done" {
		t.Errorf("agent_type = %v, want h-done", entry["agent_type"])
	}
	checkEffectiveMode(t, entry, "cli_interactive")
}

// TestBuildAgentHistory_AllModes verifies all four effective_mode values and one
// legacy NULL row appear in the correct order in agent history.
func TestBuildAgentHistory_AllModes(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	times := []string{
		"2025-01-01T00:00:01Z",
		"2025-01-01T00:00:02Z",
		"2025-01-01T00:00:03Z",
		"2025-01-01T00:00:04Z",
		"2025-01-01T00:00:05Z",
	}
	sessions := []struct{ agentType, mode string }{
		{"h-cli", "cli"},
		{"h-cli-int", "cli_interactive"},
		{"h-api", "api"},
		{"h-script", "script"},
		{"h-legacy", ""},
	}
	for i, s := range sessions {
		insertSession(t, pool, "sh-"+s.agentType, wfiID, s.agentType, "completed", "pass", times[i])
		insertSessionMode(t, pool, "sh-"+s.agentType, s.mode)
	}

	history := svc.buildAgentHistory(wfiID, map[string][]RestartDetail{})
	if len(history) != len(sessions) {
		t.Fatalf("buildAgentHistory len = %d, want %d", len(history), len(sessions))
	}
	for i, s := range sessions {
		entry, ok := history[i].(map[string]interface{})
		if !ok {
			t.Fatalf("history[%d] = %T, want map", i, history[i])
		}
		if entry["agent_type"] != s.agentType {
			t.Errorf("history[%d].agent_type = %v, want %q", i, entry["agent_type"], s.agentType)
		}
		checkEffectiveMode(t, entry, s.mode)
	}
}
