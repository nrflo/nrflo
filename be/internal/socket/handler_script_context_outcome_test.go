package socket

import (
	"testing"
)

// TestScriptContext_Outcomes verifies the four outcome keys script.context adds
// (workflow_status, workflow_result, workflow_final_result, failure_reason).
// All scenarios share one handler env; each creates its own ticket/WFI so the
// expensive DB+migration setup runs once for the whole group.
func TestScriptContext_Outcomes(t *testing.T) {
	env := newHandlerTestEnv(t)

	setStatus := func(t *testing.T, wfiID, status string) {
		t.Helper()
		if _, err := env.pool.Exec(
			`UPDATE workflow_instances SET status = ? WHERE id = ?`, status, wfiID,
		); err != nil {
			t.Fatalf("update status: %v", err)
		}
	}

	// derived workflow_result/workflow_status for ticket-scoped WFIs.
	derivedCases := []struct {
		name       string
		ticketID   string
		sessionID  string
		status     string
		wantResult string
	}{
		{"completed->pass", "SC-COMP-1", "sess-sc-comp-1", "completed", "pass"},
		{"failed->fail", "SC-FAIL-1", "sess-sc-fail-1", "failed", "fail"},
	}
	for _, tc := range derivedCases {
		t.Run(tc.name, func(t *testing.T) {
			env.createTicketAndWorkflow(t, tc.ticketID)
			wfiID := queryWFIID(t, env, tc.ticketID)
			setStatus(t, wfiID, tc.status)
			insertAgentSession(t, env, tc.ticketID, tc.sessionID, wfiID)

			_, result := callScriptContext(t, env.handler, tc.sessionID)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if got, _ := result["workflow_result"].(string); got != tc.wantResult {
				t.Errorf("workflow_result = %q, want %q", got, tc.wantResult)
			}
			if got, _ := result["workflow_status"].(string); got != tc.status {
				t.Errorf("workflow_status = %q, want %q", got, tc.status)
			}
		})
	}

	// project_completed (project-scoped WFI, sessionless ticket) also maps to pass.
	t.Run("project_completed->pass", func(t *testing.T) {
		wfiID := insertProjectWFI(t, env, "proj-wfi-pc-1")
		setStatus(t, wfiID, "project_completed")

		sessionID := "sess-sc-pc-1"
		if _, err := env.pool.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
			VALUES (?, ?, '', ?, 'analyzer', 'test-agent', 'claude-sonnet-4', 'running', datetime('now'), datetime('now'))
		`, sessionID, env.project, wfiID); err != nil {
			t.Fatalf("insert session: %v", err)
		}

		_, result := callScriptContext(t, env.handler, sessionID)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if got, _ := result["workflow_result"].(string); got != "pass" {
			t.Errorf("workflow_result = %q, want %q (project_completed should map to pass)", got, "pass")
		}
		if got, _ := result["workflow_status"].(string); got != "project_completed" {
			t.Errorf("workflow_status = %q, want %q", got, "project_completed")
		}
	})

	// workflow_final_result is surfaced from a session-scope finding.
	t.Run("workflow_final_result", func(t *testing.T) {
		env.createTicketAndWorkflow(t, "SC-WFR-1")
		wfiID := queryWFIID(t, env, "SC-WFR-1")
		sessionID := "sess-sc-wfr-1"
		insertAgentSession(t, env, "SC-WFR-1", sessionID, wfiID)
		setSessionFindings(t, env, sessionID, map[string]interface{}{
			"workflow_final_result": "Summary of final outcome",
		})

		_, result := callScriptContext(t, env.handler, sessionID)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if got, _ := result["workflow_final_result"].(string); got != "Summary of final outcome" {
			t.Errorf("workflow_final_result = %q, want %q", got, "Summary of final outcome")
		}
	})

	// failure_reason is parsed from the _failure_reason WFI finding {"reason": "..."}.
	t.Run("failure_reason", func(t *testing.T) {
		env.createTicketAndWorkflow(t, "SC-FR-1")
		wfiID := queryWFIID(t, env, "SC-FR-1")
		setWFIFindings(t, env, wfiID, map[string]interface{}{
			"_failure_reason": map[string]interface{}{"reason": "boom"},
		})
		sessionID := "sess-sc-fr-1"
		insertAgentSession(t, env, "SC-FR-1", sessionID, wfiID)

		_, result := callScriptContext(t, env.handler, sessionID)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if got, _ := result["failure_reason"].(string); got != "boom" {
			t.Errorf("failure_reason = %q, want %q", got, "boom")
		}
	})

	// All 4 outcome keys present with zero values when WFI is active and no
	// findings are set.
	t.Run("absent->zero values", func(t *testing.T) {
		env.createTicketAndWorkflow(t, "SC-OKA-1")
		wfiID := queryWFIID(t, env, "SC-OKA-1")
		sessionID := "sess-sc-oka-1"
		insertAgentSession(t, env, "SC-OKA-1", sessionID, wfiID)

		_, result := callScriptContext(t, env.handler, sessionID)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if got, _ := result["workflow_result"].(string); got != "" {
			t.Errorf("workflow_result = %q, want empty string for active WFI", got)
		}
		if got, _ := result["workflow_status"].(string); got != "active" {
			t.Errorf("workflow_status = %q, want %q for active WFI", got, "active")
		}
		if got, _ := result["workflow_final_result"].(string); got != "" {
			t.Errorf("workflow_final_result = %q, want empty string when absent", got)
		}
		if got, _ := result["failure_reason"].(string); got != "" {
			t.Errorf("failure_reason = %q, want empty string when absent", got)
		}
	})
}
