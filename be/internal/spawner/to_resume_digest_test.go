package spawner

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestFetchPreviousDataAndReason_FreshDigest_WinsOverToResume verifies a
// fresh autonomous refinery slot digest takes priority over the to_resume
// finding for the same (workflow_instance_id, node_id) slot.
func TestFetchPreviousDataAndReason_FreshDigest_WinsOverToResume(t *testing.T) {
	t.Parallel()
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	prevStarted := time.Now().Add(-time.Hour)
	createContinuedSessionWithStart(t, env, sessionID,
		map[string]interface{}{"to_resume": "to_resume finding text"}, "low_context", prevStarted)

	digestRepo := repo.NewRefineryDigestRepo(env.database, clock.NewTest(prevStarted.Add(time.Minute)))
	if _, err := digestRepo.UpsertSlot(env.wfiID, "test-phase", env.projectID, "DIGEST-TEXT"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet-5", "test-phase", "")

	if !strings.Contains(data, "DIGEST-TEXT") {
		t.Errorf("data = %q, want to contain digest content %q", data, "DIGEST-TEXT")
	}
	if strings.Contains(data, "to_resume finding text") {
		t.Errorf("data = %q, fresh digest must win over to_resume — to_resume text must not appear", data)
	}
	if reason != "low_context" {
		t.Errorf("reason = %q, want %q", reason, "low_context")
	}
}

// TestFetchPreviousDataAndReason_StaleDigest_FallsBackToToResume verifies a
// digest folded before the prior session started (stale) is ignored in favor
// of the to_resume finding.
func TestFetchPreviousDataAndReason_StaleDigest_FallsBackToToResume(t *testing.T) {
	t.Parallel()
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	prevStarted := time.Now()
	createContinuedSessionWithStart(t, env, sessionID,
		map[string]interface{}{"to_resume": "to_resume finding text"}, "low_context", prevStarted)

	digestRepo := repo.NewRefineryDigestRepo(env.database, clock.NewTest(prevStarted.Add(-time.Nanosecond)))
	if _, err := digestRepo.UpsertSlot(env.wfiID, "test-phase", env.projectID, "STALE-DIGEST"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	data, _ := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet-5", "test-phase", "")

	if !strings.Contains(data, "to_resume finding text") {
		t.Errorf("data = %q, want to contain to_resume fallback %q", data, "to_resume finding text")
	}
}

// TestFetchPreviousDataAndReason_NoDigest_FallsBackToToResume verifies the
// existing to_resume path is unchanged when no digest slot exists at all.
func TestFetchPreviousDataAndReason_NoDigest_FallsBackToToResume(t *testing.T) {
	t.Parallel()
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	createContinuedSessionWithStart(t, env, sessionID,
		map[string]interface{}{"to_resume": "only to_resume available"}, "low_context", time.Now().Add(-time.Hour))

	data, _ := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet-5", "test-phase", "")

	if !strings.Contains(data, "only to_resume available") {
		t.Errorf("data = %q, want to contain to_resume fallback %q", data, "only to_resume available")
	}
}

// TestFetchPreviousDataAndReason_EmptyDigest_FallsBackToToResume verifies an
// existing but empty-content digest slot does not win over to_resume.
func TestFetchPreviousDataAndReason_EmptyDigest_FallsBackToToResume(t *testing.T) {
	t.Parallel()
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	prevStarted := time.Now().Add(-time.Hour)
	createContinuedSessionWithStart(t, env, sessionID,
		map[string]interface{}{"to_resume": "fallback text"}, "low_context", prevStarted)

	digestRepo := repo.NewRefineryDigestRepo(env.database, clock.NewTest(prevStarted.Add(time.Minute)))
	if _, err := digestRepo.UpsertSlot(env.wfiID, "test-phase", env.projectID, ""); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	data, _ := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet-5", "test-phase", "")

	if !strings.Contains(data, "fallback text") {
		t.Errorf("data = %q, want to contain to_resume fallback %q", data, "fallback text")
	}
}

// TestFetchPreviousDataAndReason_CrashPath_FreshDigestWinsWithNoToResume
// covers the crash/fail_restart shape: a continued session with
// result_reason=fail_restart, no to_resume finding at all, and a fresh
// digest — the digest must still be returned.
func TestFetchPreviousDataAndReason_CrashPath_FreshDigestWinsWithNoToResume(t *testing.T) {
	t.Parallel()
	env := setupToResumeTestEnv(t)
	defer env.cleanup()

	sessionID := uuid.New().String()
	prevStarted := time.Now().Add(-time.Hour)
	createContinuedSessionWithStart(t, env, sessionID, map[string]interface{}{}, "fail_restart", prevStarted)

	digestRepo := repo.NewRefineryDigestRepo(env.database, clock.NewTest(prevStarted.Add(time.Minute)))
	if _, err := digestRepo.UpsertSlot(env.wfiID, "test-phase", env.projectID, "CRASH-DIGEST"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	data, reason := env.spawner.fetchPreviousDataAndReason(
		env.projectID, env.ticketID, env.workflowID,
		"test-agent", "claude:sonnet-5", "test-phase", "")

	if !strings.Contains(data, "CRASH-DIGEST") {
		t.Errorf("data = %q, want to contain digest content %q", data, "CRASH-DIGEST")
	}
	if reason != "fail_restart" {
		t.Errorf("reason = %q, want %q", reason, "fail_restart")
	}
}

// createContinuedSessionWithStart mirrors createContinuedSessionWithReason
// (to_resume_integration_test.go) but with an explicit started_at, needed
// for freshness comparisons against a seeded digest's updated_at.
func createContinuedSessionWithStart(t *testing.T, env *toResumeTestEnv, sessionID string, findings map[string]interface{}, resultReason string, startedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		NodeID:             "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet-5", Valid: true},
		Status:             model.AgentSessionContinued,
		Result:             sql.NullString{String: "continue", Valid: true},
		ResultReason:       sql.NullString{String: resultReason, Valid: resultReason != ""},
		StartedAt:          sql.NullString{String: startedAt.UTC().Format(time.RFC3339Nano), Valid: true},
		EndedAt:            sql.NullString{String: now, Valid: true},
	}
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create continued session: %v", err)
	}

	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	denorm := repo.Denorm{ProjectID: env.projectID, WorkflowInstanceID: env.wfiID, AgentType: "test-agent", ModelID: "claude:sonnet-5"}
	actor := repo.Actor{Source: "agent"}
	for k, v := range findings {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("createContinuedSessionWithStart: marshal key %q: %v", k, err)
		}
		if err := findingRepo.Upsert("session", sessionID, k, json.RawMessage(b), denorm, actor); err != nil {
			t.Fatalf("createContinuedSessionWithStart: Upsert key %q: %v", k, err)
		}
	}
}
