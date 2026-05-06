package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be/internal/db"
)

// insertASLogsSessWithMode inserts an agent_session row with an explicit effective_mode value.
func insertASLogsSessWithMode(t *testing.T, pool *db.Pool, id, wfiID, agentType, status, effectiveMode string, endedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := endedAt.Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)
	ended := endedAt.UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, findings, started_at, ended_at, effective_mode, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, 'ph', ?, ?, '', ?, ?, ?, ?, ?)`,
		id, wfiID, agentType, status, started, ended, effectiveMode, now, now,
	)
	if err != nil {
		t.Fatalf("insertASLogsSessWithMode(%s): %v", id, err)
	}
}

// TestHandleListAgentSessionLogs_EffectiveModeOverridesJoin verifies that when
// agent_sessions.effective_mode is set, it takes precedence over the JOINed
// agent_definitions.execution_mode in the API response.
func TestHandleListAgentSessionLogs_EffectiveModeOverridesJoin(t *testing.T) {
	s, pool := newASLogsServer(t)
	base := time.Date(2025, 9, 1, 12, 0, 0, 0, time.UTC)

	// agent-api agent_def has execution_mode='api'; effective_mode='cli_interactive' must win.
	insertASLogsSessWithMode(t, pool, "sess-override", "test-wfi", "agent-api", "completed", "cli_interactive", base)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Sessions []struct {
			SessionID     string `json:"session_id"`
			ExecutionMode string `json:"execution_mode"`
		} `json:"sessions"`
		Total int `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Total)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(resp.Sessions))
	}
	sess := resp.Sessions[0]
	if sess.SessionID != "sess-override" {
		t.Errorf("session_id = %q, want sess-override", sess.SessionID)
	}
	// effective_mode column wins over the JOINed agent_definitions.execution_mode='api'.
	if sess.ExecutionMode != "cli_interactive" {
		t.Errorf("execution_mode = %q, want cli_interactive (effective_mode beats JOIN fallback)", sess.ExecutionMode)
	}
}

// TestHandleListAgentSessionLogs_LegacyNullFallbackToAgentDef verifies that when
// agent_sessions.effective_mode is NULL (legacy row), the response falls back to
// agent_definitions.execution_mode.
func TestHandleListAgentSessionLogs_LegacyNullFallbackToAgentDef(t *testing.T) {
	s, pool := newASLogsServer(t)
	base := time.Date(2025, 9, 2, 12, 0, 0, 0, time.UTC)

	// Insert without effective_mode → NULL in DB; agent_def has execution_mode='api'.
	insertASLogsSess(t, pool, "sess-legacy-null", "test-wfi", "agent-api", "completed", "", base)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Sessions []struct {
			SessionID     string `json:"session_id"`
			ExecutionMode string `json:"execution_mode"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(resp.Sessions))
	}
	// NULL effective_mode → falls back to agent_def execution_mode='api'.
	if resp.Sessions[0].ExecutionMode != "api" {
		t.Errorf("execution_mode = %q, want api (legacy fallback to agent_def)", resp.Sessions[0].ExecutionMode)
	}
}

// TestHandleListAgentSessionLogs_AllEffectiveModes verifies that all four valid
// effective_mode values (cli, cli_interactive, api, script) are surfaced correctly
// in the API response, each overriding the JOINed agent_def value.
func TestHandleListAgentSessionLogs_AllEffectiveModes(t *testing.T) {
	modes := []string{"cli", "cli_interactive", "api", "script"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			s, pool := newASLogsServer(t)
			base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			sessID := "sess-mode-" + mode

			// agent-api has execution_mode='api'; effective_mode should override it.
			insertASLogsSessWithMode(t, pool, sessID, "test-wfi", "agent-api", "completed", mode, base)

			req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
			rr := httptest.NewRecorder()
			s.handleListAgentSessionLogs(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("[%s] status = %d, want 200; body: %s", mode, rr.Code, rr.Body.String())
			}

			var resp struct {
				Sessions []struct {
					ExecutionMode string `json:"execution_mode"`
				} `json:"sessions"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("[%s] decode: %v", mode, err)
			}
			if len(resp.Sessions) != 1 {
				t.Fatalf("[%s] sessions count = %d, want 1", mode, len(resp.Sessions))
			}
			if resp.Sessions[0].ExecutionMode != mode {
				t.Errorf("[%s] execution_mode = %q, want %q", mode, resp.Sessions[0].ExecutionMode, mode)
			}
		})
	}
}

// TestHandleListAgentSessionLogs_NoAgentDefFallbackEmpty verifies that when
// effective_mode is NULL and there is no matching agent_definition row, the
// execution_mode field is empty in the response.
func TestHandleListAgentSessionLogs_NoAgentDefFallbackEmpty(t *testing.T) {
	s, pool := newASLogsServer(t)
	base := time.Date(2025, 9, 3, 12, 0, 0, 0, time.UTC)

	// 'unknown-agent' has no matching agent_def row → NULL JOIN → empty execution_mode.
	insertASLogsSess(t, pool, "sess-no-def", "test-wfi", "unknown-agent", "failed", "", base)

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Sessions []struct {
			ExecutionMode string `json:"execution_mode"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(resp.Sessions))
	}
	// Both effective_mode (NULL) and agent_def.execution_mode (no match) are absent.
	if resp.Sessions[0].ExecutionMode != "" {
		t.Errorf("execution_mode = %q, want empty (no agent_def, no effective_mode)", resp.Sessions[0].ExecutionMode)
	}
}
