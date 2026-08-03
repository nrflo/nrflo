package refinery

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
)

// TestAttemptFoldCLI drives attemptFoldCLI directly against a real
// cli_interactive chain entry (chain[1] of the resolved `_refinery` chain),
// isolating the CLIFolder seam's classification branches.
func TestAttemptFoldCLI(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager(pool, clk)

	def, err := mgr.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		t.Fatalf("GetForBackend: %v", err)
	}
	chain, err := mgr.systemAgentSvc.ResolveAgentChain(def)
	if err != nil {
		t.Fatalf("ResolveAgentChain: %v", err)
	}
	cliEntry := chain[1] // anthropic/cli_interactive/haiku-4-5

	target := foldTarget{sessionID: "sess-attempt-cli"}

	t.Run("nil CLIFolder advances", func(t *testing.T) {
		// mgr.cliFolder is nil by construction (never set in this subtest).
		fresh := NewManager(pool, clk)
		res := fresh.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
	})

	t.Run("missing _refinery-cli def advances", func(t *testing.T) {
		// Independent pool copy so deleting the def never affects the other
		// subtests sharing mgr/pool.
		soloPool := newTestPool(t)
		if _, err := soloPool.Exec(`DELETE FROM system_agent_definitions WHERE id = '_refinery-cli'`); err != nil {
			t.Fatalf("delete _refinery-cli def: %v", err)
		}
		soloMgr := NewManager(soloPool, clk)
		soloMgr.SetCLIFolder(&fakeCLIFolder{result: types.RefineryFoldResult{Content: "unused"}})
		res := soloMgr.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
	})

	t.Run("ErrRefineryFoldProviderBuild wrapped error advances", func(t *testing.T) {
		mgr.SetCLIFolder(&fakeCLIFolder{err: fmt.Errorf("%w: no credentials", types.ErrRefineryFoldProviderBuild)})
		res := mgr.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
	})

	t.Run("other seam error stops", func(t *testing.T) {
		mgr.SetCLIFolder(&fakeCLIFolder{err: errors.New("no session registered for _refinery-cli")})
		res := mgr.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false with a non-nil err", res)
		}
	})

	t.Run("empty content from successful call is degenerate, stops", func(t *testing.T) {
		mgr.SetCLIFolder(&fakeCLIFolder{result: types.RefineryFoldResult{Content: "   "}})
		res := mgr.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false with a non-nil err (degenerate/whitespace-only content)", res)
		}
	})

	t.Run("non-empty content lands", func(t *testing.T) {
		mgr.SetCLIFolder(&fakeCLIFolder{result: types.RefineryFoldResult{Content: "a folded digest", InputTokens: 3, OutputTokens: 5}})
		res := mgr.attemptFoldCLI(context.Background(), target, "proj", "text", cliEntry)
		if res.err != nil {
			t.Fatalf("res.err = %v, want nil", res.err)
		}
		if res.content != "a folded digest" {
			t.Errorf("res.content = %q, want %q", res.content, "a folded digest")
		}
		if res.usage.InputTokens != 3 || res.usage.OutputTokens != 5 {
			t.Errorf("res.usage = %+v, want {InputTokens:3 OutputTokens:5}", res.usage)
		}
	})
}
