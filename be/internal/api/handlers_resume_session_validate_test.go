package api

import (
	"database/sql"
	"strings"
	"testing"

	"be/internal/model"
)

// --- validateResumeSession unit tests ---

func TestValidateResumeSession(t *testing.T) {
	cases := []struct {
		name        string
		session     *model.AgentSession
		wantErr     bool
		errContains string
	}{
		{
			name:        "null model_id",
			session:     &model.AgentSession{ModelID: sql.NullString{Valid: false}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "no model_id",
		},
		{
			name:        "empty model_id string",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "no model_id",
		},
		{
			name:        "codex model_id (no resume support)",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "codex:codex_gpt_high", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr:     true,
			errContains: "does not support resume",
		},
		{
			name:        "running session",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionRunning},
			wantErr:     true,
			errContains: "terminal state",
		},
		{
			name:        "user_interactive session",
			session:     &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionUserInteractive},
			wantErr:     true,
			errContains: "terminal state",
		},
		{
			name:    "completed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionCompleted},
			wantErr: false,
		},
		{
			name:    "failed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:opus", Valid: true}, Status: model.AgentSessionFailed},
			wantErr: false,
		},
		{
			name:    "timeout claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:haiku", Valid: true}, Status: model.AgentSessionTimeout},
			wantErr: false,
		},
		{
			name:    "interactive_completed claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionInteractiveCompleted},
			wantErr: false,
		},
		{
			name:    "skipped claude session",
			session: &model.AgentSession{ModelID: sql.NullString{String: "claude:sonnet", Valid: true}, Status: model.AgentSessionSkipped},
			wantErr: false,
		},
		{
			// A closed console session is interactive_completed; resuming it would
			// resurrect its dead bearer token. Only the kind guard blocks it.
			name: "console session is never resumable",
			session: &model.AgentSession{
				ModelID: sql.NullString{String: "claude:sonnet", Valid: true},
				Status:  model.AgentSessionInteractiveCompleted,
				Kind:    model.AgentSessionKindConsole,
			},
			wantErr:     true,
			errContains: "kind",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// agent_sessions.kind is NOT NULL DEFAULT 'workflow_agent', so a row
			// read from the DB always carries a kind; mirror that for fixtures
			// that don't set one explicitly.
			if tc.session.Kind == "" {
				tc.session.Kind = model.AgentSessionKindWorkflowAgent
			}
			err := validateResumeSession(tc.session)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateResumeSession() = nil, want error")
					return
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
			} else if err != nil {
				t.Errorf("validateResumeSession() = %v, want nil", err)
			}
		})
	}
}
