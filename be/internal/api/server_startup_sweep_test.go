package api

import (
	"context"
	"testing"
)

// TestStartupOrphanSweep_FailsCrashLeftovers verifies the startup sweep fails
// rows a crashed (non-graceful) previous process left in-flight: a running
// agent session flips to failed with reason=startup_orphan_sweep and an
// active workflow instance is failed, exactly like the graceful shutdown
// sweep but stamped with the startup reason.
func TestStartupOrphanSweep_FailsCrashLeftovers(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)
	sdActiveWFI(t, srv, pid, "", "project", "wfi-startup-orphan")
	sdRunningSession(t, srv, pid, "", "wfi-startup-orphan", "sess-startup-orphan")

	srv.startupOrphanSweep(context.Background())

	var status, reason string
	if err := srv.pool.QueryRow(
		`SELECT status, result_reason FROM agent_sessions WHERE id = 'sess-startup-orphan'`,
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if status != "failed" || reason != "startup_orphan_sweep" {
		t.Errorf("session = %s/%s, want failed/startup_orphan_sweep", status, reason)
	}

	var wfiStatus string
	if err := srv.pool.QueryRow(
		`SELECT status FROM workflow_instances WHERE id = 'wfi-startup-orphan'`,
	).Scan(&wfiStatus); err != nil {
		t.Fatalf("read wfi: %v", err)
	}
	if wfiStatus != "failed" {
		t.Errorf("workflow instance status = %s, want failed", wfiStatus)
	}
}
