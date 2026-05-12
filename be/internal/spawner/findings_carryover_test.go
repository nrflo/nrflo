package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// createNamedSession inserts a session row with the given ID and returns it.
func (env *testEnv) createNamedSession(t *testing.T, sessionID, modelID string) *model.AgentSession {
	t.Helper()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("createNamedSession: failed to create session %s: %v", sessionID, err)
	}
	return session
}

// setSessionFindings sets the findings JSON on the given session row.
func setSessionFindings(t *testing.T, env *testEnv, sessionID string, findings map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("setSessionFindings: marshal: %v", err)
	}
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.UpdateFindings(sessionID, string(b)); err != nil {
		t.Fatalf("setSessionFindings: UpdateFindings: %v", err)
	}
}

// getSessionFindings loads a session and returns its findings map.
func getSessionFindings(t *testing.T, env *testEnv, sessionID string) map[string]interface{} {
	t.Helper()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session, err := sessionRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("getSessionFindings: Get(%s): %v", sessionID, err)
	}
	return session.GetFindings()
}

// TestCopyFindingsForContinuation_EmptyTarget verifies that findings from the old
// session are copied to a new session whose findings map is empty.
func TestCopyFindingsForContinuation_EmptyTarget(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	oldFindings := map[string]interface{}{
		"summary":   "abc",
		"files":     []interface{}{"a.go"},
		"to_resume": "resume blob",
	}
	setSessionFindings(t, env, oldID, oldFindings)

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	got := getSessionFindings(t, env, newID)

	if got["summary"] != "abc" {
		t.Errorf("summary: got %v, want %q", got["summary"], "abc")
	}
	if got["to_resume"] != "resume blob" {
		t.Errorf("to_resume: got %v, want %q", got["to_resume"], "resume blob")
	}
	files, ok := got["files"]
	if !ok {
		t.Fatalf("files key missing from new session findings")
	}
	wantFiles := []interface{}{"a.go"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("files: got %v, want %v", files, wantFiles)
	}
}

// TestCopyFindingsForContinuation_PrePopulatedTarget verifies that when the new
// session already has a key, the new-session value wins; absent keys are copied.
func TestCopyFindingsForContinuation_PrePopulatedTarget(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	oldFindings := map[string]interface{}{
		"summary":   "old summary",
		"files":     []interface{}{"a.go"},
		"to_resume": "resume blob",
	}
	setSessionFindings(t, env, oldID, oldFindings)

	newFindings := map[string]interface{}{
		"summary": "new summary",
	}
	setSessionFindings(t, env, newID, newFindings)

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	got := getSessionFindings(t, env, newID)

	// New-session key wins on conflict.
	if got["summary"] != "new summary" {
		t.Errorf("summary: got %v, want %q (new session should win)", got["summary"], "new summary")
	}
	// Absent keys are carried over from old session.
	if got["to_resume"] != "resume blob" {
		t.Errorf("to_resume: got %v, want %q", got["to_resume"], "resume blob")
	}
	files, ok := got["files"]
	if !ok {
		t.Fatalf("files key missing; should be copied from old session")
	}
	wantFiles := []interface{}{"a.go"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("files: got %v, want %v", files, wantFiles)
	}
}

// TestCopyFindingsForContinuation_OldEmpty verifies that when the old session has
// no findings, the new session's findings are left untouched.
func TestCopyFindingsForContinuation_OldEmpty(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	// Old session: empty findings (no UpdateFindings call).
	// New session: has a pre-existing key.
	newFindings := map[string]interface{}{
		"x": "1",
	}
	setSessionFindings(t, env, newID, newFindings)

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	got := getSessionFindings(t, env, newID)
	if got["x"] != "1" {
		t.Errorf("x: got %v, want %q; new session should be unchanged when old is empty", got["x"], "1")
	}
	if len(got) != 1 {
		t.Errorf("unexpected extra keys in new session findings: %v", got)
	}
}

// TestCopyFindingsForContinuation_OldEmptyJSON verifies the same no-op behaviour
// when the old session has an explicit empty-object findings value.
func TestCopyFindingsForContinuation_OldEmptyJSON(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	setSessionFindings(t, env, oldID, map[string]interface{}{})

	newFindings := map[string]interface{}{
		"x": "1",
	}
	setSessionFindings(t, env, newID, newFindings)

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	got := getSessionFindings(t, env, newID)
	if got["x"] != "1" {
		t.Errorf("x: got %v, want %q; new session should be unchanged when old is empty", got["x"], "1")
	}
	if len(got) != 1 {
		t.Errorf("unexpected extra keys in new session findings: %v", got)
	}
}

// TestCopyFindingsForContinuation_OldMissing verifies that passing a non-existent
// old session ID does not return an error and leaves the new session unchanged.
func TestCopyFindingsForContinuation_OldMissing(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	nonExistentOldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, newID, "claude:sonnet")
	setSessionFindings(t, env, newID, map[string]interface{}{"x": "1"})

	// Should not panic; all errors are logged as warnings.
	env.spawner.copyFindingsForContinuation(context.Background(), nonExistentOldID, newID)

	got := getSessionFindings(t, env, newID)
	if got["x"] != "1" {
		t.Errorf("x: got %v, want %q; new session should be unchanged when old is missing", got["x"], "1")
	}
}

// TestCopyFindingsForContinuation_MultipleKeys verifies that all keys from the old
// session (summary, files slice, to_resume, and an extra custom key) are carried
// over correctly to an empty new session.
func TestCopyFindingsForContinuation_MultipleKeys(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	oldID := uuid.New().String()
	newID := uuid.New().String()

	env.createNamedSession(t, oldID, "claude:sonnet")
	env.createNamedSession(t, newID, "claude:sonnet")

	oldFindings := map[string]interface{}{
		"summary":     "done",
		"files":       []interface{}{"main.go", "util.go"},
		"to_resume":   "state blob",
		"custom_note": "carry me",
	}
	setSessionFindings(t, env, oldID, oldFindings)

	env.spawner.copyFindingsForContinuation(context.Background(), oldID, newID)

	got := getSessionFindings(t, env, newID)

	for _, k := range []string{"summary", "to_resume", "custom_note"} {
		if _, ok := got[k]; !ok {
			t.Errorf("key %q missing from new session findings", k)
		}
	}
	if got["summary"] != "done" {
		t.Errorf("summary: got %v, want %q", got["summary"], "done")
	}
	if got["to_resume"] != "state blob" {
		t.Errorf("to_resume: got %v, want %q", got["to_resume"], "state blob")
	}
	if got["custom_note"] != "carry me" {
		t.Errorf("custom_note: got %v, want %q", got["custom_note"], "carry me")
	}
	wantFiles := []interface{}{"main.go", "util.go"}
	if !reflect.DeepEqual(got["files"], wantFiles) {
		t.Errorf("files: got %v, want %v", got["files"], wantFiles)
	}
}
