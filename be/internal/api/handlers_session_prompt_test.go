package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

// newSessionPromptServer creates a Server with a temp DB for session prompt handler tests.
// Returns the server and an open DB connection for test data setup.
func newSessionPromptServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session_prompt_test.db")
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	s := &Server{pool: pool, clock: clock.Real()}
	return s, database
}

// insertSessionWithPrompt inserts an agent_session row with the given prompt_context value.
// Pass empty string for promptContext to store NULL (omits the column from INSERT).
func insertSessionWithPrompt(t *testing.T, database *db.DB, id, wfiID, projectID, promptContext string) {
	t.Helper()
	if promptContext == "" {
		_, err := database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at)
			VALUES (?, ?, 'TKT-1', ?, 'impl', 'implementor', 'sonnet', 'completed', datetime('now'), datetime('now'))`,
			id, projectID, wfiID)
		if err != nil {
			t.Fatalf("insertSessionWithPrompt(%s, null prompt): %v", id, err)
		}
	} else {
		_, err := database.Exec(`
			INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, prompt_context, created_at, updated_at)
			VALUES (?, ?, 'TKT-1', ?, 'impl', 'implementor', 'sonnet', 'completed', ?, datetime('now'), datetime('now'))`,
			id, projectID, wfiID, promptContext)
		if err != nil {
			t.Fatalf("insertSessionWithPrompt(%s): %v", id, err)
		}
	}
}

// TestHandleGetSessionPrompt_WithPrompt verifies 200 and correct JSON body when prompt_context is set.
func TestHandleGetSessionPrompt_WithPrompt(t *testing.T) {
	s, database := newSessionPromptServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-prompt", "Prompt Project")
	wantPrompt := "# System Prompt\n\nYou are an agent. Do the task.\n\n## Context\n\nSome context here."
	insertSessionWithPrompt(t, database, "sess-with-prompt", wfiID, "proj-prompt", wantPrompt)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-with-prompt/prompt", nil)
	req.SetPathValue("id", "sess-with-prompt")
	rr := httptest.NewRecorder()
	s.handleGetSessionPrompt(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	got, ok := resp["prompt_context"].(string)
	if !ok {
		t.Fatalf("prompt_context field missing or wrong type: %v", resp["prompt_context"])
	}
	if got != wantPrompt {
		t.Errorf("prompt_context = %q, want %q", got, wantPrompt)
	}

	// Verify Content-Type is JSON
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestHandleGetSessionPrompt_NullPrompt verifies 204 with no body when prompt_context is NULL.
func TestHandleGetSessionPrompt_NullPrompt(t *testing.T) {
	s, database := newSessionPromptServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-null", "Null Prompt Project")
	insertSessionWithPrompt(t, database, "sess-null-prompt", wfiID, "proj-null", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/sess-null-prompt/prompt", nil)
	req.SetPathValue("id", "sess-null-prompt")
	rr := httptest.NewRecorder()
	s.handleGetSessionPrompt(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); body != "" {
		t.Errorf("body = %q, want empty for 204", body)
	}
}

// TestHandleGetSessionPrompt_NotFound verifies 404 when the session ID does not exist.
func TestHandleGetSessionPrompt_NotFound(t *testing.T) {
	s, database := newSessionPromptServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/no-such-session/prompt", nil)
	req.SetPathValue("id", "no-such-session")
	rr := httptest.NewRecorder()
	s.handleGetSessionPrompt(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleGetSessionPrompt_EmptyID verifies 404 when session ID is an empty string.
func TestHandleGetSessionPrompt_EmptyID(t *testing.T) {
	s, database := newSessionPromptServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions//prompt", nil)
	// No SetPathValue — extractID returns ""
	rr := httptest.NewRecorder()
	s.handleGetSessionPrompt(rr, req)

	// Empty ID means lookup will fail with "not found"
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}

// TestHandleGetSessionPrompt_TableDriven runs all cases in a table for clarity.
func TestHandleGetSessionPrompt_TableDriven(t *testing.T) {
	s, database := newSessionPromptServer(t)
	defer database.Close()

	wfiID := seedProject(t, database, "proj-table", "Table Project")
	insertSessionWithPrompt(t, database, "sess-table-with", wfiID, "proj-table", "hello prompt")
	insertSessionWithPrompt(t, database, "sess-table-null", wfiID, "proj-table", "")

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
		wantKey    string // if non-empty, verify this key is present in JSON response
	}{
		{
			name:       "existing session with prompt",
			sessionID:  "sess-table-with",
			wantStatus: http.StatusOK,
			wantKey:    "prompt_context",
		},
		{
			name:       "existing session without prompt",
			sessionID:  "sess-table-null",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "non-existent session",
			sessionID:  "sess-table-missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+tc.sessionID+"/prompt", nil)
			req.SetPathValue("id", tc.sessionID)
			rr := httptest.NewRecorder()
			s.handleGetSessionPrompt(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantKey != "" {
				var resp map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if _, ok := resp[tc.wantKey]; !ok {
					t.Errorf("response missing key %q; got %v", tc.wantKey, resp)
				}
			}
		})
	}
}
