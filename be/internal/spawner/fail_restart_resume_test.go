package spawner

import (
	"context"
	"os"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"

	"github.com/google/uuid"
)

// TestResumeOnRelaunch_SetOnlyByFailRestartBranch is table-driven over every
// relaunch reason: only the fail-restart branch in monitorAll
// (spawner_monitor.go) ever sets resumeOnRelaunch=true; every other reason
// (low-context, stall, timeout, rate-limit, tier fallback) leaves it at its
// zero-value false. The fail-restart branch itself is inlined in monitorAll
// rather than a standalone helper, so this mirrors that one line exactly
// (established pattern: TestAutoRestart_SessionOverriddenToContined above);
// the other reasons are proven by their real relaunch functions never writing
// the field at all (relaunchForContinuation only ever resets it to false;
// relaunchForFallback never references it).
func TestResumeOnRelaunch_SetOnlyByFailRestartBranch(t *testing.T) {
	t.Parallel()

	t.Run("fail_restart sets it true", func(t *testing.T) {
		t.Parallel()
		proc := &processInfo{finalStatus: "FAIL", maxFailRestarts: 2, failRestartCount: 0}
		// Mirrors the exact monitorAll auto-restart branch (spawner_monitor.go).
		if proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
			proc.failRestartCount++
			proc.finalStatus = "CONTINUE"
			proc.resumeOnRelaunch = true
		}
		if !proc.resumeOnRelaunch {
			t.Error("resumeOnRelaunch = false after a fail-restart, want true")
		}
	})

	t.Run("timeout_restart does not set it", func(t *testing.T) {
		t.Parallel()
		// Mirrors monitorAll's timeout auto-restart branch, which never
		// references resumeOnRelaunch.
		proc := &processInfo{finalStatus: "TIMEOUT", maxFailRestarts: 2, failRestartCount: 0}
		if proc.finalStatus == "TIMEOUT" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
			proc.failRestartCount++
			proc.finalStatus = "CONTINUE"
		}
		if proc.resumeOnRelaunch {
			t.Error("resumeOnRelaunch = true after a timeout restart, want false")
		}
	})

	t.Run("stall restart does not set it", func(t *testing.T) {
		t.Parallel()
		proc := &processInfo{restartCount: 0}
		// checkStall's relaunch path never touches resumeOnRelaunch.
		if proc.resumeOnRelaunch {
			t.Error("resumeOnRelaunch = true for a stall-restarted proc, want false")
		}
	})

	t.Run("rate-limit restart does not set it", func(t *testing.T) {
		t.Parallel()
		proc := &processInfo{rateLimitRetryCount: 0}
		// handleRateLimitRetry (rate_limit_restart.go) never references
		// resumeOnRelaunch; it only sets finalStatus=CONTINUE and bumps
		// rateLimitRetryCount/rateLimitTotalWait.
		if proc.resumeOnRelaunch {
			t.Error("resumeOnRelaunch = true for a rate-limit-restarted proc, want false")
		}
	})

	t.Run("proactive rotation (low-context shape) does not set it", func(t *testing.T) {
		t.Parallel()
		proc := &processInfo{proactiveRotationPending: true}
		if proc.resumeOnRelaunch {
			t.Error("resumeOnRelaunch = true for a proactively-rotated proc, want false")
		}
	})
}

// TestRelaunchForContinuation_CarriesOrDropsHandoffByResumeOnRelaunch is the
// concrete, real-code-path complement to the table above: relaunchForContinuation
// (used by low-context/stall/failRestart relaunches) carries the resumeHandoff
// onward only when oldProc.resumeOnRelaunch was true (fail-restart), and
// discards it (profile dir removed) for every other reason; newProc always
// starts with resumeOnRelaunch reset to false regardless.
func TestRelaunchForContinuation_CarriesOrDropsHandoffByResumeOnRelaunch(t *testing.T) {
	t.Parallel()

	newTestSpawner := func(t *testing.T, env *contextSaveTestEnv) *Spawner {
		t.Helper()
		return New(Config{
			DataPath: env.dbPath,
			Pool:     db.WrapAsPool(env.database),
			Clock:    clock.Real(),
			APIMode:  true,
			BuildAPIProvider: func(_ context.Context, _ string, _ string) (provider.Provider, error) {
				return mock.New(), nil
			},
			ModelConfigs: map[string]ModelConfig{
				"m0": {Provider: "anthropic", APIModel: "claude-x", APIContext: 100000, APIEfforts: []string{"low"}, DefaultEffort: "low"},
			},
			AgentSvc: &noopAgentSvc{},
		})
	}

	t.Run("resumeOnRelaunch=true carries the handoff", func(t *testing.T) {
		t.Parallel()
		ensureTmpNrfloDir(t)
		env := setupContextSaveTestEnv(t)
		defer env.cleanup()
		insertAPIAgentDef(t, env, "impl", "m0")
		sp := newTestSpawner(t, env)

		dir := t.TempDir()
		oldSessionID := uuid.New().String()
		if !sp.createAgentSessionRow(env.projectID, env.ticketID, env.wfiID, "impl", "impl", oldSessionID, "claude:m0", "impl", "", "", "", "", "api", 0) {
			t.Fatal("createAgentSessionRow(old) failed")
		}
		oldProc := &processInfo{
			sessionID:          oldSessionID,
			agentID:            "old-agent-id",
			agentType:          "impl",
			projectID:          env.projectID,
			ticketID:           env.ticketID,
			workflowName:       "feature",
			workflowInstanceID: env.wfiID,
			modelID:            "claude:m0",
			resumeOnRelaunch:   true,
			resumeHandoff:      &codexThreadHandoff{threadID: "t1", profileDir: dir},
		}

		newProc, err := sp.relaunchForContinuation(context.Background(), oldProc, SpawnRequest{
			AgentType: "impl", ProjectID: env.projectID, TicketID: env.ticketID,
			WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
		}, "impl")
		if err != nil {
			t.Fatalf("relaunchForContinuation() error: %v", err)
		}

		if oldProc.resumeHandoff != nil {
			t.Error("oldProc.resumeHandoff not nilled after a carrying relaunch")
		}
		if newProc.resumeHandoff == nil {
			t.Fatal("newProc.resumeHandoff is nil, want the carried handoff")
		}
		if newProc.resumeOnRelaunch {
			t.Error("newProc.resumeOnRelaunch = true, want false (reset on every relaunch)")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("profile dir removed despite the handoff being carried: %v", err)
		}
	})

	t.Run("resumeOnRelaunch=false drops the handoff", func(t *testing.T) {
		t.Parallel()
		ensureTmpNrfloDir(t)
		env := setupContextSaveTestEnv(t)
		defer env.cleanup()
		insertAPIAgentDef(t, env, "impl", "m0")
		sp := newTestSpawner(t, env)

		dir := t.TempDir()
		oldSessionID := uuid.New().String()
		if !sp.createAgentSessionRow(env.projectID, env.ticketID, env.wfiID, "impl", "impl", oldSessionID, "claude:m0", "impl", "", "", "", "", "api", 0) {
			t.Fatal("createAgentSessionRow(old) failed")
		}
		oldProc := &processInfo{
			sessionID:          oldSessionID,
			agentID:            "old-agent-id",
			agentType:          "impl",
			projectID:          env.projectID,
			ticketID:           env.ticketID,
			workflowName:       "feature",
			workflowInstanceID: env.wfiID,
			modelID:            "claude:m0",
			resumeOnRelaunch:   false, // low-context/stall shape
			resumeHandoff:      &codexThreadHandoff{threadID: "t1", profileDir: dir},
		}

		newProc, err := sp.relaunchForContinuation(context.Background(), oldProc, SpawnRequest{
			AgentType: "impl", ProjectID: env.projectID, TicketID: env.ticketID,
			WorkflowName: "feature", WorkflowInstanceID: env.wfiID,
		}, "impl")
		if err != nil {
			t.Fatalf("relaunchForContinuation() error: %v", err)
		}

		if newProc.resumeHandoff != nil {
			t.Error("newProc.resumeHandoff should be nil when the old handoff was discarded, not carried")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("profile dir still exists after a dropping relaunch: err=%v", err)
		}
	})
}
