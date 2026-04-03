package spawner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockErrorRecorder captures RecordError calls for assertion in spawner tests.
type mockErrorRecorder struct {
	mu    sync.Mutex
	calls []errorCall
}

type errorCall struct {
	projectID  string
	errorType  string
	instanceID string
	message    string
}

func (m *mockErrorRecorder) RecordError(projectID, errorType, instanceID, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, errorCall{
		projectID:  projectID,
		errorType:  errorType,
		instanceID: instanceID,
		message:    message,
	})
	return nil
}

func (m *mockErrorRecorder) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockErrorRecorder) getCall(i int) errorCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[i]
}

// withMockErrorSvc sets a new mockErrorRecorder on the spawner and returns it.
func withMockErrorSvc(env *testEnv) *mockErrorRecorder {
	mock := &mockErrorRecorder{}
	env.spawner.config.ErrorSvc = mock
	return mock
}

// TestCheckInstantStall_RecordsErrorOnRestart verifies that RecordError is called
// when an instant stall restart occurs (budget not exhausted).
func TestCheckInstantStall_RecordsErrorOnRestart(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	mock := withMockErrorSvc(env)

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Fatalf("finalStatus = %q, want CONTINUE — stall not triggered", proc.finalStatus)
	}

	if got := mock.callCount(); got != 1 {
		t.Fatalf("RecordError calls = %d, want 1", got)
	}

	call := mock.getCall(0)
	wantMsg := fmt.Sprintf("implementor: instant_stall (restart 1/%d)", maxStallRestarts)
	if call.message != wantMsg {
		t.Errorf("message = %q, want %q", call.message, wantMsg)
	}
	if call.errorType != "agent" {
		t.Errorf("errorType = %q, want %q", call.errorType, "agent")
	}
	if call.projectID != env.projectID {
		t.Errorf("projectID = %q, want %q", call.projectID, env.projectID)
	}
	if call.instanceID != env.sessionID {
		t.Errorf("instanceID = %q, want %q", call.instanceID, env.sessionID)
	}
}

// TestCheckInstantStall_RecordsError_NthRestart verifies the message counter is
// 1-indexed and matches the post-increment stallRestartCount.
func TestCheckInstantStall_RecordsError_NthRestart(t *testing.T) {
	tests := []struct {
		name              string
		stallRestartCount int // before the call
		wantN             int // expected N in "restart N/6"
	}{
		{"first_restart", 0, 1},
		{"second_restart", 1, 2},
		{"third_restart", 2, 3},
		{"last_restart_before_exhaustion", maxStallRestarts - 1, maxStallRestarts},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()
			mock := withMockErrorSvc(env)

			env.createSession(t, "claude:sonnet")
			insertTestMessages(t, env, 1)

			proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, tt.stallRestartCount)
			env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

			if proc.finalStatus != "CONTINUE" {
				t.Fatalf("finalStatus = %q, want CONTINUE", proc.finalStatus)
			}
			if got := mock.callCount(); got != 1 {
				t.Fatalf("RecordError calls = %d, want 1", got)
			}

			wantMsg := fmt.Sprintf("implementor: instant_stall (restart %d/%d)", tt.wantN, maxStallRestarts)
			if got := mock.getCall(0).message; got != wantMsg {
				t.Errorf("message = %q, want %q", got, wantMsg)
			}
		})
	}
}

// TestCheckInstantStall_NoErrorWhenGuardSkips verifies RecordError is NOT called
// when any guard condition prevents stall detection.
func TestCheckInstantStall_NoErrorWhenGuardSkips(t *testing.T) {
	tests := []struct {
		name     string
		modelID  string
		elapsed  time.Duration
		msgCount int
		noOp     bool
	}{
		{"non_claude_opencode", "opencode:opencode_gpt_normal", 15 * time.Second, 1, false},
		{"non_claude_codex", "codex:codex_gpt_normal", 15 * time.Second, 1, false},
		{"elapsed_at_boundary", "claude:sonnet", 1 * time.Minute, 1, false},
		{"elapsed_above_boundary", "claude:sonnet", 2 * time.Minute, 1, false},
		{"msg_count_4", "claude:sonnet", 15 * time.Second, 4, false},
		{"msg_count_5", "claude:sonnet", 15 * time.Second, 5, false},
		{"no_op_finding", "claude:sonnet", 15 * time.Second, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestEnv(t)
			defer env.cleanup()
			mock := withMockErrorSvc(env)

			env.createSession(t, tt.modelID)
			insertTestMessages(t, env, tt.msgCount)
			if tt.noOp {
				setSessionFindings(t, env, map[string]interface{}{"no-op": "no-op"})
			}

			proc := makeInstantStallProc(env, tt.modelID, tt.elapsed, 0)
			env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

			if proc.finalStatus != "PASS" {
				t.Errorf("finalStatus = %q, want PASS (guard should have blocked)", proc.finalStatus)
			}
			if got := mock.callCount(); got != 0 {
				t.Errorf("RecordError calls = %d, want 0 (guard skipped stall)", got)
			}
		})
	}
}

// TestCheckInstantStall_BudgetExhausted_RecordsError is a regression guard verifying
// that the budget-exhausted path still records "stall_budget_exhausted" (unchanged behavior).
func TestCheckInstantStall_BudgetExhausted_RecordsError(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	mock := withMockErrorSvc(env)

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts)
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "FAIL" {
		t.Fatalf("finalStatus = %q, want FAIL when budget exhausted", proc.finalStatus)
	}

	if got := mock.callCount(); got != 1 {
		t.Fatalf("RecordError calls = %d, want 1 for budget-exhausted path", got)
	}

	call := mock.getCall(0)
	wantMsg := "implementor: stall_budget_exhausted"
	if call.message != wantMsg {
		t.Errorf("message = %q, want %q", call.message, wantMsg)
	}
	if call.errorType != "agent" {
		t.Errorf("errorType = %q, want %q", call.errorType, "agent")
	}
	if call.projectID != env.projectID {
		t.Errorf("projectID = %q, want %q", call.projectID, env.projectID)
	}
}

// TestCheckInstantStall_NilErrorSvc_NoPanic verifies that nil ErrorSvc does not panic
// on instant stall restart. This is the default state of the spawner in most tests.
func TestCheckInstantStall_NilErrorSvc_NoPanic(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	// ErrorSvc is nil by default in setupTestEnv — no explicit nil assignment needed.

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, 0)
	// Must not panic.
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("finalStatus = %q, want CONTINUE", proc.finalStatus)
	}
}

// TestCheckInstantStall_NilErrorSvc_BudgetExhausted_NoPanic verifies that nil ErrorSvc
// does not panic on the budget-exhausted path either.
func TestCheckInstantStall_NilErrorSvc_BudgetExhausted_NoPanic(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	// ErrorSvc is nil by default — verifies nil-safe path in markInstantStallFailed.

	env.createSession(t, "claude:sonnet")
	insertTestMessages(t, env, 1)

	proc := makeInstantStallProc(env, "claude:sonnet", 15*time.Second, maxStallRestarts)
	// Must not panic.
	env.spawner.checkInstantStall(context.Background(), proc, makeInstantStallReq(env))

	if proc.finalStatus != "FAIL" {
		t.Errorf("finalStatus = %q, want FAIL", proc.finalStatus)
	}
}
