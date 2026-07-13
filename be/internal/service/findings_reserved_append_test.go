package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/types"
)

// --- Append ---

func TestFindingsAppend_RejectsReservedKey(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	_, err := svc.Append(&types.FindingsAppendRequest{
		SessionID: sessionID,
		Key:       WorkflowPlanFindingKey,
		Value:     `{"version":1}`,
	})
	assertReservedRejection(t, pool, err)
}

func TestFindingsAppend_AllowsConsultAnswer(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.Append(&types.FindingsAppendRequest{
		SessionID: sessionID,
		Key:       "_consult_answer",
		Value:     "appended answer",
	}); err != nil {
		t.Fatalf("Append(_consult_answer) unexpectedly rejected: %v", err)
	}

	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	raw, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	val, ok := raw["_consult_answer"]
	if !ok {
		t.Fatal("_consult_answer not found after Append")
	}
	if string(val) != `"appended answer"` {
		t.Errorf("_consult_answer value = %s, want %q", val, `"appended answer"`)
	}
}

func TestFindingsAppend_AllowsNormalKey(t *testing.T) {
	t.Parallel()
	_, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.Append(&types.FindingsAppendRequest{
		SessionID: sessionID,
		Key:       "log_lines",
		Value:     "line one",
	}); err != nil {
		t.Fatalf("Append(normal key) unexpectedly rejected: %v", err)
	}
}

// --- AppendBulk ---

func TestFindingsAppendBulk_RejectsReservedKey(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	_, err := svc.AppendBulk(&types.FindingsAppendBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{
			WorkflowPlanFindingKey: `{"version":1}`,
			"other_key":            "value",
		},
	})
	assertReservedRejection(t, pool, err)
}

func TestFindingsAppendBulk_AllowsConsultAnswer(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.AppendBulk(&types.FindingsAppendBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{
			"_consult_answer": "bulk appended answer",
		},
	}); err != nil {
		t.Fatalf("AppendBulk(_consult_answer) unexpectedly rejected: %v", err)
	}

	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	raw, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	val, ok := raw["_consult_answer"]
	if !ok {
		t.Fatal("_consult_answer not found after AppendBulk")
	}
	if string(val) != `"bulk appended answer"` {
		t.Errorf("_consult_answer value = %s, want %q", val, `"bulk appended answer"`)
	}
}

func TestFindingsAppendBulk_AllowsNormalKey(t *testing.T) {
	t.Parallel()
	_, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.AppendBulk(&types.FindingsAppendBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{"k1": "v1"},
	}); err != nil {
		t.Fatalf("AppendBulk(normal key) unexpectedly rejected: %v", err)
	}
}
