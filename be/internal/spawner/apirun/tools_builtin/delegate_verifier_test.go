package tools_builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/model"
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

// In a console chat (SessionKind console_chat) an omitted wait_sec launches
// async for every tier — the interactive turn must not block; completion
// arrives via the ChatNotifier. The hint points at the notification contract.
func TestDelegate_ConsoleChat_OmittedWaitSec_StaysAsync(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.SessionKind = model.AgentSessionKindConsoleChat
	env.env.Delegator = &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return `{"delegation_id":"wfi.cc","status":"running"}`, nil
		},
		getDelegationFn: func(context.Context, string, string) (string, error) {
			t.Fatal("GetDelegation must not be called — console chats default async")
			return "", nil
		},
	}

	for _, tier := range []string{"extractor", "verifier"} {
		out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"`+tier+`","brief":"do it"}`)
		if err != nil {
			t.Fatalf("Invoke err: %v", err)
		}
		if isErr {
			t.Errorf("tier %s: isErr=true, want false; out=%q", tier, out)
		}
		if !strings.Contains(out, "running") || !strings.Contains(out, "notified when the delegation completes") {
			t.Errorf("tier %s: out=%q, want async running result with the notification hint", tier, out)
		}
	}
}
