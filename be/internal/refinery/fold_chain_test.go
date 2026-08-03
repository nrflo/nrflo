package refinery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/types"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// makeSDKErr builds a minimal *sdk.Error carrying statusCode, sufficient for
// apirun.ClassifyProviderError's StatusCode-based branches (mirrors
// apirun/errors_test.go's helper of the same shape).
func makeSDKErr(statusCode int) *sdk.Error {
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	resp := &http.Response{StatusCode: statusCode}
	return &sdk.Error{StatusCode: statusCode, Request: req, Response: resp}
}

// fakeCLIFolder is an in-memory CLIFolder seam: it records every request and
// replays a fixed (result, err) pair, so tests can drive attemptFoldCLI/
// walkFoldChain's cli_interactive branch without a real one-off child spawn.
type fakeCLIFolder struct {
	calls  []types.RefineryFoldRequest
	result types.RefineryFoldResult
	err    error
}

func (f *fakeCLIFolder) RunRefineryFold(_ context.Context, req types.RefineryFoldRequest) (types.RefineryFoldResult, error) {
	f.calls = append(f.calls, req)
	return f.result, f.err
}

var _ CLIFolder = (*fakeCLIFolder)(nil)

// TestResolveRefineryChain_TierOneShape asserts the migrated template DB's
// resolved chain for `_refinery` before relying on its length/shape in the
// walk tests below (be/CLAUDE.md: assert first, never assume).
func TestResolveRefineryChain_TierOneShape(t *testing.T) {
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
	if len(chain) != 3 {
		t.Fatalf("len(chain) = %d, want 3 (000195 api+cli haiku, 000220 cli luna)", len(chain))
	}
	if chain[0].ExecutionMode != "api" || chain[0].ModelID != "haiku-4-5" {
		t.Errorf("chain[0] = %+v, want {ExecutionMode:api ModelID:haiku-4-5}", chain[0])
	}
	if chain[1].ExecutionMode != "cli_interactive" || chain[1].ModelID != "haiku-4-5" {
		t.Errorf("chain[1] = %+v, want {ExecutionMode:cli_interactive ModelID:haiku-4-5}", chain[1])
	}
	if chain[2].ExecutionMode != "cli_interactive" || chain[2].ModelID != "gpt-5.6-luna" {
		t.Errorf("chain[2] = %+v, want {ExecutionMode:cli_interactive ModelID:gpt-5.6-luna}", chain[2])
	}
}

// TestWalkFoldChain_APIBuildFailureAdvancesToCLILanding drives the full
// console fold through the real 3-entry chain: pos0 (api) fails at the
// buildProvider seam, pos1 (cli_interactive) lands via a fake CLIFolder.
// Expects the digest written from the CLI landing and exactly two
// refinery_runs rows (failed pos0, ok pos1 with a non-empty fallback_from).
func TestWalkFoldChain_APIBuildFailureAdvancesToCLILanding(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-chain-cli-land", "proj-chain-cli-land"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProviderErr(t, errors.New("no anthropic API key"))
	cli := &fakeCLIFolder{result: types.RefineryFoldResult{Content: "cli digest v1", InputTokens: 9, OutputTokens: 4}}
	mgr.SetCLIFolder(cli)

	foldConsoleOnce(context.Background(), mgr, sessionID, projectID, []string{"event"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d == nil || d.Content != "cli digest v1" {
		t.Fatalf("Get after fold = %+v, want content %q", d, "cli digest v1")
	}

	if len(cli.calls) != 1 {
		t.Fatalf("CLIFolder calls = %d, want 1", len(cli.calls))
	}

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (failed pos0, ok pos1)", len(rows))
	}
	if rows[0].status != "failed" {
		t.Errorf("rows[0].status = %q, want failed", rows[0].status)
	}
	if rows[1].status != "ok" {
		t.Errorf("rows[1].status = %q, want ok", rows[1].status)
	}

	var pos0, pos1 int
	var mode0, mode1, fb0, fb1 string
	if err := pool.QueryRow(`SELECT chain_position, execution_mode, fallback_from FROM refinery_runs WHERE status='failed'`).Scan(&pos0, &mode0, &fb0); err != nil {
		t.Fatalf("query failed row: %v", err)
	}
	if err := pool.QueryRow(`SELECT chain_position, execution_mode, fallback_from FROM refinery_runs WHERE status='ok'`).Scan(&pos1, &mode1, &fb1); err != nil {
		t.Fatalf("query ok row: %v", err)
	}
	if pos0 != 0 || mode0 != "api" || fb0 != "" {
		t.Errorf("failed row = pos:%d mode:%q fallback:%q, want pos:0 mode:api fallback:\"\"", pos0, mode0, fb0)
	}
	if pos1 != 1 || mode1 != "cli_interactive" || fb1 == "" {
		t.Errorf("ok row = pos:%d mode:%q fallback:%q, want pos:1 mode:cli_interactive non-empty fallback", pos1, mode1, fb1)
	}
}

// TestWalkFoldChain_ChainExhausted_AllRowsFailedDigestUntouched drives every
// chain entry to an advance-eligible build-time failure (api buildProvider
// errors, cli seam returns a types.ErrRefineryFoldProviderBuild-wrapped
// error), asserting one failed row per entry and no digest write.
func TestWalkFoldChain_ChainExhausted_AllRowsFailedDigestUntouched(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-chain-exhausted", "proj-chain-exhausted"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProviderErr(t, errors.New("no anthropic API key"))
	mgr.SetCLIFolder(&fakeCLIFolder{err: fmt.Errorf("%w: no cli credentials", types.ErrRefineryFoldProviderBuild)})

	foldConsoleOnce(context.Background(), mgr, sessionID, projectID, []string{"event"})

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d != nil {
		t.Errorf("Get after exhausted chain = %+v, want nil (no digest write)", d)
	}

	rows := queryRefineryRuns(t, pool)
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 (one per chain entry)", len(rows))
	}
	for i, r := range rows {
		if r.status != "failed" {
			t.Errorf("rows[%d].status = %q, want failed", i, r.status)
		}
	}
}

// TestWalkFoldChain_CancelledContextAbortsPromptly verifies a pre-cancelled
// ctx aborts the walk without hanging — attemptFoldAPI's provider.Run must
// see ctx.Err() rather than the walk looping the whole chain.
func TestWalkFoldChain_CancelledContextAbortsPromptly(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-chain-cancel", "proj-chain-cancel"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mock.Script{Err: context.Canceled}))
	mgr.SetCLIFolder(&fakeCLIFolder{err: errors.New("unused")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		foldConsoleOnce(ctx, mgr, sessionID, projectID, []string{"event"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("foldConsole did not return promptly on a cancelled context")
	}

	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d != nil {
		t.Errorf("Get after cancelled fold = %+v, want nil", d)
	}
}

// TestAttemptFoldAPI table-drives every classification branch directly
// against attemptFoldAPI, isolated from chain resolution (a hand-built
// service.AgentChainEntry lets model/api_model/provider-run scenarios be
// constructed that ResolveAgentChain's own validation would otherwise
// reject before the walk ever ran).
func TestAttemptFoldAPI(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mgr := NewManager(pool, clk)
	def, err := mgr.systemAgentSvc.GetForBackend("refinery", "api")
	if err != nil {
		t.Fatalf("GetForBackend: %v", err)
	}
	entry, err := mgr.systemAgentSvc.ResolveAgentChain(def)
	if err != nil {
		t.Fatalf("ResolveAgentChain: %v", err)
	}
	apiEntry := entry[0] // anthropic/api/haiku-4-5

	target := foldTarget{sessionID: "sess-attempt-api"}

	t.Run("model row missing entirely advances", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mockScript("unused")))
		bad := apiEntry
		bad.ModelID = "does-not-exist"
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, bad)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
	})

	t.Run("model row present but api_model empty advances", func(t *testing.T) {
		if _, err := pool.Exec(`INSERT INTO models (id, provider, display_name, cli_model, api_model, enabled, created_at, updated_at) VALUES ('no-api-model', 'anthropic', 'No API Model', 'claude-x', '', 1, datetime('now'), datetime('now'))`); err != nil {
			t.Fatalf("seed model row: %v", err)
		}
		stubBuildProvider(t, mock.New(mockScript("unused")))
		bad := apiEntry
		bad.ModelID = "no-api-model"
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, bad)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
		if res.provName != "anthropic" {
			t.Errorf("res.provName = %q, want anthropic (populated even on this failure)", res.provName)
		}
	})

	t.Run("buildProvider failure advances", func(t *testing.T) {
		stubBuildProviderErr(t, errors.New("no anthropic API key"))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true with a non-nil err", res)
		}
	})

	t.Run("provider.Run RetryClassError advances", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mock.Script{Err: makeSDKErr(401)}))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if !res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=true (auth error is RetryClassError)", res)
		}
	})

	t.Run("provider.Run RetryClassRateLimit stops", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mock.Script{Err: makeSDKErr(429)}))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false (rate-limit must not advance)", res)
		}
	})

	t.Run("provider.Run unclassified error stops", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mock.Script{Err: errors.New("weird transport error")}))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false (RetryClassNone must not advance)", res)
		}
	})

	t.Run("empty text output stops", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mockScript("   ")))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false (degenerate output)", res)
		}
	})

	t.Run("max_tokens stop reason stops", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mock.Script{Final: provider.FinalResponse{
			StopReason: "max_tokens",
			Content:    []provider.ContentBlock{{Type: "text", Text: "truncated..."}},
		}}))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if res.advance || res.err == nil {
			t.Errorf("res = %+v, want advance=false (max_tokens is degenerate)", res)
		}
	})

	t.Run("success lands with content and no error", func(t *testing.T) {
		stubBuildProvider(t, mock.New(mockScript("a good digest")))
		res := mgr.attemptFoldAPI(context.Background(), target, "proj", "text", def, apiEntry)
		if res.err != nil {
			t.Fatalf("res.err = %v, want nil", res.err)
		}
		if res.content != "a good digest" {
			t.Errorf("res.content = %q, want %q", res.content, "a good digest")
		}
	})
}
