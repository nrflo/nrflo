package tools_builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun"
)

// The verifier tier shares the extractor's inline-by-default contract: an
// omitted wait_sec blocks for the result instead of returning async.
func TestDelegate_VerifierOmittedWaitSec_BlocksInline(t *testing.T) {
	old := delegatePollInterval
	delegatePollInterval = time.Millisecond
	defer func() { delegatePollInterval = old }()

	env := newBuiltinTestEnv(t)
	polls := 0
	var gotTier string
	env.env.Delegator = &fakeDelegator{
		delegateFn: func(_ context.Context, _ string, req apirun.DelegateRequest) (string, error) {
			gotTier = req.Tier
			return `{"delegation_id":"wfi.ver","status":"running"}`, nil
		},
		getDelegationFn: func(context.Context, string, string) (string, error) {
			polls++
			if polls < 2 {
				return `{"delegation_id":"wfi.ver","status":"running"}`, nil
			}
			return `{"delegation_id":"wfi.ver","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"verifier","brief":"refute this claim"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if gotTier != "verifier" {
		t.Errorf("Delegator saw tier=%q, want verifier", gotTier)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("out=%q, want completed via the default inline wait", out)
	}
	if polls < 2 {
		t.Errorf("polls=%d, want >=2 (default verifier wait must poll)", polls)
	}
}
