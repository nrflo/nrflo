package spawner

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestHandleCompletion_ClassifyExitError_SetsHardProviderFail verifies the
// CLI/codex path: when ClassifyExit returns RetryClassError (e.g. a
// non-rate-limit auth/provider error pattern in the tail output),
// handleCompletion sets proc.hardProviderFail=true and resultReason
// "provider_error_pattern" — the signal shouldAdvanceChain consumes on a
// mid-work HARD failure.
func TestHandleCompletion_ClassifyExitError_SetsHardProviderFail(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")

	// Non-zero exit with no explicit agent-reported result, so handleCompletion
	// falls into "exit_code" and then consults the adapter's ClassifyExit.
	cmd := exec.Command("false")
	_ = cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet-5",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-2 * time.Second),
		adapter:            &ClaudeAdapter{},
	}
	proc.appendStderr("API Error: 401 Unauthorized")

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	if proc.finalStatus != "FAIL" {
		t.Errorf("proc.finalStatus = %q, want FAIL", proc.finalStatus)
	}
	if !proc.hardProviderFail {
		t.Error("proc.hardProviderFail = false, want true (RetryClassError from ClassifyExit)")
	}
}

// TestHandleCompletion_ClassifyExitRateLimit_NeverSetsHardProviderFail is the
// CLI-side guard mirroring the apirun-side rate-limit test: a rate-limit
// pattern match must never set hardProviderFail, since rate limits stay
// in-band via the existing retry dance and must never advance the chain.
func TestHandleCompletion_ClassifyExitRateLimit_NeverSetsHardProviderFail(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	defer env.cleanup()

	env.createSession(t, "claude:sonnet-5")

	cmd := exec.Command("false")
	_ = cmd.Run()

	proc := &processInfo{
		cmd:                cmd,
		sessionID:          env.sessionID,
		agentID:            "test-agent-id",
		modelID:            "claude:sonnet-5",
		workflowInstanceID: env.wfiID,
		projectID:          env.projectID,
		ticketID:           env.ticketID,
		workflowName:       env.workflowID,
		startTime:          time.Now().Add(-2 * time.Second),
		adapter:            &ClaudeAdapter{},
		// rateLimitConfig.Enabled left false, so handleRateLimitRetry is
		// skipped and we can observe finalStatus/hardProviderFail directly
		// without the retry side-effects.
	}
	proc.appendStderr("Overloaded")

	env.spawner.handleCompletion(context.Background(), proc, SpawnRequest{
		ProjectID:    env.projectID,
		TicketID:     env.ticketID,
		WorkflowName: env.workflowID,
		AgentType:    "test-agent",
	})

	if proc.hardProviderFail {
		t.Error("proc.hardProviderFail = true, want false (rate-limit pattern must never advance the chain)")
	}
}
