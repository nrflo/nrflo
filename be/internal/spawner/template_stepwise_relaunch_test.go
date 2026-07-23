package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"

	"github.com/google/uuid"
)

// cleanupPreparedSpawn removes the promptFile/suffixFile a prepareSpawn call
// wrote, mirroring TestPrepareSpawn_Stepwise_CreatesCursorIdempotently.
func cleanupPreparedSpawn(t *testing.T, prep *prepResult) {
	t.Helper()
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}
}

// TestPrepareSpawn_Stepwise_RelaunchReusesCursorAcrossTeardown is case 19:
// a real CAS-guarded Advance moves the cursor one step, then the
// session-keyed in-memory teardowns a relaunch calls on the old session are
// invoked; a second prepareSpawn for the same (wfiID, phase) must leave the
// cursor byte-identical and render step 2's own instruction, never step 3's.
func TestPrepareSpawn_Stepwise_RelaunchReusesCursorAcrossTeardown(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	createStepwiseAgentDefInContextEnv(t, env, "impl", "sonnet-5", threeSteps())

	sp := stepwisePrepareSpawner(env)
	req := SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature",
		WorkflowInstanceID: env.wfiID, TicketID: env.ticketID,
	}

	_, prep, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("first prepareSpawn() error: %v", err)
	}
	cleanupPreparedSpawn(t, prep)

	// Real CAS-guarded advance: revision 1 -> 2, current_index 0 -> 1.
	cursorRepo := repo.NewAgentStepCursorRepo(env.database, clock.Real())
	completedJSON := `[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`
	ok, advErr := cursorRepo.Advance(env.wfiID, "impl", 1, 0, completedJSON)
	if advErr != nil {
		t.Fatalf("Advance: %v", advErr)
	}
	if !ok {
		t.Fatal("Advance() = false, want true (CAS should succeed on a fresh cursor)")
	}

	oldSessionID := uuid.New().String()
	// Session-keyed in-memory teardowns a relaunch calls on the old session —
	// none of these touch the DB cursor (cursor key is (wfiID, nodeID), not
	// session).
	DropProactiveRestartState(oldSessionID)
	FinalizeSessionCost(oldSessionID)
	globalLedgerStore.drop(oldSessionID)

	revBefore, idxBefore, completedBefore, rejectionsBefore := readCursorRow(t, env.database, env.wfiID, "impl")

	_, prep2, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("second prepareSpawn() error: %v", err)
	}
	cleanupPreparedSpawn(t, prep2)

	revAfter, idxAfter, completedAfter, rejectionsAfter := readCursorRow(t, env.database, env.wfiID, "impl")
	if revAfter != revBefore || idxAfter != idxBefore || completedAfter != completedBefore || rejectionsAfter != rejectionsBefore {
		t.Errorf("cursor changed across relaunch: before=(%d,%d,%q,%q) after=(%d,%d,%q,%q)",
			revBefore, idxBefore, completedBefore, rejectionsBefore, revAfter, idxAfter, completedAfter, rejectionsAfter)
	}
	if revAfter != 2 || idxAfter != 1 {
		t.Errorf("cursor after relaunch = revision=%d current_index=%d, want 2/1", revAfter, idxAfter)
	}

	if !strings.Contains(prep2.prompt, "step 2 of") {
		t.Errorf("second prepareSpawn's prompt does not show step 2: %s", prep2.prompt)
	}
	if !strings.Contains(prep2.prompt, "Instruction body two.") {
		t.Errorf("second prepareSpawn's prompt is missing step-two's instruction: %s", prep2.prompt)
	}
	if strings.Contains(prep2.prompt, "Instruction body three.") {
		t.Errorf("second prepareSpawn's prompt leaked step-three's instruction (future step must never render): %s", prep2.prompt)
	}
}

// readCursorRow reads the (revision, current_index, completed, rejections)
// tuple for (wfiID, nodeID) directly.
func readCursorRow(t *testing.T, database interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}, wfiID, nodeID string) (revision, currentIndex int, completed, rejections string) {
	t.Helper()
	if err := database.QueryRow(
		`SELECT revision, current_index, completed, rejections FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		wfiID, nodeID).Scan(&revision, &currentIndex, &completed, &rejections); err != nil {
		t.Fatalf("readCursorRow: %v", err)
	}
	return revision, currentIndex, completed, rejections
}

// insertContinuedSession writes a status='continued' agent_sessions row
// matching (workflow_instance_id, agent_type, model_id, node_id) — the
// lookup fetchPreviousDataAndReason performs to find the prior run.
func insertContinuedSession(t *testing.T, env *contextSaveTestEnv, sessionID, agentType, modelID, nodeID, resultReason string, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, node_id, agent_type, model_id, status, result, result_reason, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'continued', 'continue', ?, ?, ?, ?, ?)`,
		sessionID, env.projectID, env.ticketID, env.wfiID, nodeID, nodeID, agentType, modelID,
		resultReason, startedAt.UTC().Format(time.RFC3339Nano), now, now, now); err != nil {
		t.Fatalf("insertContinuedSession: %v", err)
	}
}

// TestPrepareSpawn_Stepwise_ResumeShowsCompletedStepsForPlainFailRestart is
// case 20: with a prior continued session matching (wfi, agent, model,
// node), the prepended low-context block on the next render contains the
// "## Completed Steps (verified)" header for the step the cursor has
// already recorded as done — proving the P3 resume body composes on a
// plain fail-restart, not only on a rotation.
func TestPrepareSpawn_Stepwise_ResumeShowsCompletedStepsForPlainFailRestart(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	createStepwiseAgentDefInContextEnv(t, env, "impl", "sonnet-5", threeSteps())

	sp := stepwisePrepareSpawner(env)
	req := SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature",
		WorkflowInstanceID: env.wfiID, TicketID: env.ticketID,
	}

	// First spawn creates the cursor.
	_, prep, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("first prepareSpawn() error: %v", err)
	}
	cleanupPreparedSpawn(t, prep)

	// Step one completes.
	cursorRepo := repo.NewAgentStepCursorRepo(env.database, clock.Real())
	completedJSON := `[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`
	if ok, advErr := cursorRepo.Advance(env.wfiID, "impl", 1, 0, completedJSON); advErr != nil || !ok {
		t.Fatalf("Advance: ok=%v err=%v", ok, advErr)
	}

	// A prior session ended (fail-restart), status='continued', matching the
	// upcoming relaunch's (agent_type, model_id, node_id).
	insertContinuedSession(t, env, uuid.New().String(), "impl", "claude:sonnet-5", "impl", "exit_code", time.Now().Add(-time.Minute))

	_, prep2, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("second prepareSpawn() error: %v", err)
	}
	cleanupPreparedSpawn(t, prep2)

	if !strings.Contains(prep2.prompt, "## Completed Steps (verified)") {
		t.Errorf("resume render missing '## Completed Steps (verified)' header: %s", prep2.prompt)
	}
	if !strings.Contains(prep2.prompt, "step-one") {
		t.Errorf("resume render does not name the completed step-one: %s", prep2.prompt)
	}
}

// TestFetchPreviousDataAndReason_StepwiseZeroCompleted_NeverFallsToToResume
// is case 21: a stepwise def with zero completed steps must take the
// stepwise branch (empty/digest-only) and never fall through to the
// to_resume finding, even when one is planted on the matching continued
// session.
func TestFetchPreviousDataAndReason_StepwiseZeroCompleted_NeverFallsToToResume(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	createStepwiseAgentDefInContextEnv(t, env, "impl", "sonnet-5", threeSteps())

	sp := stepwisePrepareSpawner(env)
	req := SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature",
		WorkflowInstanceID: env.wfiID, TicketID: env.ticketID,
	}
	_, prep, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	cleanupPreparedSpawn(t, prep)
	// Cursor left at current_index=0 — zero completed steps.

	sessionID := uuid.New().String()
	insertContinuedSession(t, env, sessionID, "impl", "claude:sonnet-5", "impl", "exit_code", time.Now().Add(-time.Minute))

	// Plant a to_resume finding on that session — must NOT surface.
	toResume, marshalErr := json.Marshal("DO NOT SHOW THIS NARRATIVE")
	if marshalErr != nil {
		t.Fatalf("marshal to_resume: %v", marshalErr)
	}
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	if err := findingRepo.Upsert("session", sessionID, "to_resume", json.RawMessage(toResume),
		repo.Denorm{ProjectID: env.projectID, WorkflowInstanceID: env.wfiID, AgentType: "impl", ModelID: "claude:sonnet-5"},
		repo.Actor{Source: "system", ID: "test"}); err != nil {
		t.Fatalf("plant to_resume finding: %v", err)
	}

	data, reason := sp.fetchPreviousDataAndReason(env.projectID, env.ticketID, "feature", "impl", "claude:sonnet-5", "impl", env.wfiID)
	if reason != "exit_code" {
		t.Errorf("reason = %q, want exit_code (reason must still surface)", reason)
	}
	if strings.Contains(data, "DO NOT SHOW THIS NARRATIVE") {
		t.Errorf("stepwise zero-completed data leaked the to_resume finding: %q", data)
	}
	if strings.Contains(data, "## Completed Steps (verified)") {
		t.Errorf("data has a Completed Steps header with zero completed steps: %q", data)
	}
}
