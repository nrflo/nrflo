package refinery

// TestAttemptFoldAPI lives in its own file (split from fold_chain_test.go)
// to stay under the 300-line source file cap; see fold_chain_test.go for the
// makeSDKErr/fakeCLIFolder helpers and the walkFoldChain-level tests.

import (
	"context"
	"errors"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

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
