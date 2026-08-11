package refinery

import (
	"context"
	"errors"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
)

// TestWalkFoldChain_NoCredentialsSkipsAPIEntry verifies that when the static
// credential pre-check reports the api provider unavailable, the walk skips
// pos0 without an attempt (no refinery_runs row) and lands the cli entry.
func TestWalkFoldChain_NoCredentialsSkipsAPIEntry(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-chain-skip", "proj-chain-skip"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProviderErr(t, errors.New("must not be called: entry skipped before build"))
	stubHasAPICreds(t, false)
	cli := &fakeCLIFolder{result: types.RefineryFoldResult{Content: "cli digest skip", InputTokens: 3, OutputTokens: 2}}
	mgr.SetCLIFolder(cli)

	foldConsoleOnce(context.Background(), mgr, sessionID, projectID, []string{"event"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d == nil || d.Content != "cli digest skip" {
		t.Fatalf("Get after fold = %+v, want content %q", d, "cli digest skip")
	}
	if len(cli.calls) != 1 {
		t.Fatalf("CLIFolder calls = %d, want 1", len(cli.calls))
	}

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (skipped pos0 records no row)", len(rows))
	}
	if rows[0].status != "ok" {
		t.Errorf("rows[0].status = %q, want ok", rows[0].status)
	}
	var pos int
	var mode string
	if err := pool.QueryRow(`SELECT chain_position, execution_mode FROM refinery_runs WHERE status='ok'`).Scan(&pos, &mode); err != nil {
		t.Fatalf("query ok row: %v", err)
	}
	if pos != 1 || mode != "cli_interactive" {
		t.Errorf("ok row = pos:%d mode:%q, want pos:1 mode:cli_interactive", pos, mode)
	}
}
