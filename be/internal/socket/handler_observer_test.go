package socket

import (
	"database/sql"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// seedObserverSession creates an observer agent_session row via the repo.
// Pass wfiID="" for global-scoped observers (stored as NULL in DB).
func seedObserverSession(t *testing.T, env *handlerTestEnv, id, scope, projectID, wfiID string) {
	t.Helper()
	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	sess := &model.AgentSession{
		ID:                 id,
		ProjectID:          projectID,
		WorkflowInstanceID: wfiID,
		AgentType:          "_observer",
		Phase:              "observer",
		Status:             model.AgentSessionRunning,
		Kind:               "observer",
		ObserverScope:      sql.NullString{String: scope, Valid: scope != ""},
	}
	if err := asRepo.Create(sess); err != nil {
		t.Fatalf("seedObserverSession id=%s scope=%s: %v", id, scope, err)
	}
}

// enableObserver sets experimental_observer_enabled=true in the test DB.
func enableObserver(t *testing.T, env *handlerTestEnv) {
	t.Helper()
	svc := service.NewGlobalSettingsService(env.pool, clock.Real())
	if err := svc.SetExperimentalObserverEnabled(true); err != nil {
		t.Fatalf("enableObserver: %v", err)
	}
}

// wfiIDForTicket queries the workflow_instance ID for a ticket seeded via createTicketAndWorkflow.
func wfiIDForTicket(t *testing.T, env *handlerTestEnv, ticketID string) string {
	t.Helper()
	var id string
	err := env.pool.QueryRow(
		`SELECT id FROM workflow_instances WHERE LOWER(project_id)=LOWER(?) AND LOWER(ticket_id)=LOWER(?) AND LOWER(workflow_id)=LOWER(?)`,
		env.project, ticketID, "test",
	).Scan(&id)
	if err != nil {
		t.Fatalf("wfiIDForTicket ticket=%s: %v", ticketID, err)
	}
	return id
}

// obsReq builds a socket Request for an observer.* method.
func obsReq(method string, params map[string]interface{}) Request {
	data, _ := json.Marshal(params)
	return Request{ID: "req-1", Method: method, Params: data}
}

// TestObserverAuth_MissingSessionID verifies validation error when session_id is empty.
func TestObserverAuth_MissingSessionID(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	resp := env.handler.Handle(obsReq("observer.global.health", map[string]interface{}{}))
	if resp.Error == nil {
		t.Fatal("expected error for missing session_id, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
}

// TestObserverAuth_UnknownSession verifies not-found error for a non-existent session_id.
func TestObserverAuth_UnknownSession(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	resp := env.handler.Handle(obsReq("observer.global.health", map[string]interface{}{
		"session_id": "does-not-exist",
	}))
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d (not found)", resp.Error.Code, ErrCodeNotFound)
	}
}

// TestObserverAuth_NotObserverSession verifies kind=workflow_agent is rejected when
// calling any observer.* method.
func TestObserverAuth_NotObserverSession(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")
	wfiID := wfiIDForTicket(t, env, "TEST-1")

	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	if err := asRepo.Create(&model.AgentSession{
		ID:                 "wf-agent-sess",
		ProjectID:          env.project,
		WorkflowInstanceID: wfiID,
		AgentType:          "implementor",
		Phase:              "impl",
		Status:             model.AgentSessionRunning,
		// Kind empty → defaults to "workflow_agent" in repo
	}); err != nil {
		t.Fatalf("seed workflow_agent session: %v", err)
	}

	resp := env.handler.Handle(obsReq("observer.global.health", map[string]interface{}{
		"session_id": "wf-agent-sess",
	}))
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	const want = "permission denied: not an observer session"
	if resp.Error.Message != want {
		t.Errorf("message = %q, want %q", resp.Error.Message, want)
	}
}

// TestObserverAuth_ScopeTooNarrow verifies out-of-scope errors for all escalation paths.
func TestObserverAuth_ScopeTooNarrow(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")
	wfiID := wfiIDForTicket(t, env, "TEST-1")

	seedObserverSession(t, env, "obs-wf", "workflow", env.project, wfiID)
	seedObserverSession(t, env, "obs-proj", "project", env.project, wfiID)

	cases := []struct {
		name      string
		sessionID string
		method    string
	}{
		{"workflow→project", "obs-wf", "observer.project.workflows"},
		{"workflow→global", "obs-wf", "observer.global.health"},
		{"project→global", "obs-proj", "observer.global.health"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := env.handler.Handle(obsReq(tc.method, map[string]interface{}{
				"session_id": tc.sessionID,
			}))
			if resp.Error == nil {
				t.Fatal("expected out-of-scope error, got nil")
			}
			if resp.Error.Code != ErrCodeValidation {
				t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
			}
			const want = "permission denied: out-of-scope call"
			if resp.Error.Message != want {
				t.Errorf("message = %q, want %q", resp.Error.Message, want)
			}
		})
	}
}

// TestObserverAuth_ProjectIDMismatch verifies project-scoped observer with a different project_id.
func TestObserverAuth_ProjectIDMismatch(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")
	wfiID := wfiIDForTicket(t, env, "TEST-1")

	seedObserverSession(t, env, "obs-proj-mismatch", "project", env.project, wfiID)

	resp := env.handler.Handle(obsReq("observer.project.workflows", map[string]interface{}{
		"session_id": "obs-proj-mismatch",
		"project_id": "completely-different-project",
	}))
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	const want = "permission denied: project_id mismatch"
	if resp.Error.Message != want {
		t.Errorf("message = %q, want %q", resp.Error.Message, want)
	}
}

// TestObserverAuth_WorkflowIDMismatch verifies workflow-scoped observer with the wrong workflow_id.
func TestObserverAuth_WorkflowIDMismatch(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")
	wfiID := wfiIDForTicket(t, env, "TEST-1")

	seedObserverSession(t, env, "obs-wf-mismatch", "workflow", env.project, wfiID)

	resp := env.handler.Handle(obsReq("observer.workflow.show", map[string]interface{}{
		"session_id":  "obs-wf-mismatch",
		"workflow_id": "some-other-workflow-id",
	}))
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
	}
	const want = "permission denied: workflow_id mismatch"
	if resp.Error.Message != want {
		t.Errorf("message = %q, want %q", resp.Error.Message, want)
	}
}

// TestObserverAuth_MutateRequiresObserverEnabled verifies that mutating methods are
// rejected when experimental_observer_enabled=false (the default).
func TestObserverAuth_MutateRequiresObserverEnabled(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)
	// Observer feature disabled by default — no call to enableObserver.
	seedObserverSession(t, env, "obs-global-no-enable", "global", env.project, "")

	cases := []struct {
		method string
		params map[string]interface{}
	}{
		{"observer.workflow.trigger", map[string]interface{}{
			"session_id": "obs-global-no-enable", "project_id": env.project, "workflow_id": "test",
		}},
		{"observer.project.env.set", map[string]interface{}{
			"session_id": "obs-global-no-enable", "name": "X", "value": "1",
		}},
		{"observer.global.project.create", map[string]interface{}{
			"session_id": "obs-global-no-enable", "project_id": "new-p",
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			resp := env.handler.Handle(obsReq(tc.method, tc.params))
			if resp.Error == nil {
				t.Fatal("expected error for disabled observer feature, got nil")
			}
			if resp.Error.Code != ErrCodeValidation {
				t.Errorf("code = %d, want %d (validation)", resp.Error.Code, ErrCodeValidation)
			}
			const want = "permission denied: observer feature is disabled"
			if resp.Error.Message != want {
				t.Errorf("message = %q, want %q", resp.Error.Message, want)
			}
		})
	}
}

// TestObserverAuth_UnknownMethod verifies method-not-found for unknown observer actions.
// Unknown methods are detected before authorization, so no session is required.
func TestObserverAuth_UnknownMethod(t *testing.T) {
	t.Parallel()
	env := newHandlerTestEnv(t)

	cases := []string{
		"observer.global.nonexistent",
		"observer.workflow.badaction",
		"observer.project.unknown",
		"observer.badnamespace.health",
	}
	for _, method := range cases {
		method := method
		t.Run(method, func(t *testing.T) {
			resp := env.handler.Handle(obsReq(method, map[string]interface{}{"session_id": "any"}))
			if resp.Error == nil {
				t.Fatalf("expected method-not-found for %s, got nil", method)
			}
			if resp.Error.Code != ErrCodeMethodNotFound {
				t.Errorf("%s: code = %d, want %d", method, resp.Error.Code, ErrCodeMethodNotFound)
			}
		})
	}
}
