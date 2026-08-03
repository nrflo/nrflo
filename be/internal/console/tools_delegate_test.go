package console

import (
	"context"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/spawner/apirun"
)

// fakeDelegator adapts func fields to apirun.Delegator for console-side
// delegate/get_delegation dispatch tests.
type fakeDelegator struct {
	delegateFn        func(ctx context.Context, callerSessionID string, req apirun.DelegateRequest) (string, error)
	getDelegationFn   func(ctx context.Context, callerSessionID, delegationID string) (string, error)
	mergeDelegationFn func(ctx context.Context, callerSessionID, delegationID string) (string, error)
	lastCaller        string
	lastReq           apirun.DelegateRequest
}

var _ apirun.Delegator = (*fakeDelegator)(nil)

func (f *fakeDelegator) Delegate(ctx context.Context, callerSessionID string, req apirun.DelegateRequest) (string, error) {
	f.lastCaller = callerSessionID
	f.lastReq = req
	return f.delegateFn(ctx, callerSessionID, req)
}

func (f *fakeDelegator) GetDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	return f.getDelegationFn(ctx, callerSessionID, delegationID)
}

func (f *fakeDelegator) MergeDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	f.lastCaller = callerSessionID
	return f.mergeDelegationFn(ctx, callerSessionID, delegationID)
}

func TestConsoleDelegate_HappyPath_RoutesThroughDepsDelegator(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
		// An extractor call with no wait_sec blocks inline (builtin default),
		// so the handler polls GetDelegation through the same Deps.Delegator.
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}
	env.deps.Delegator = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "delegate", `{"tier":"extractor","brief":"summarize the ticket"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if !strings.Contains(out, "wfi.abc") || !strings.Contains(out, "completed") {
		t.Errorf("out=%q, want the inline-waited terminal result", out)
	}
	if fake.lastCaller != "sess-1" {
		t.Errorf("lastCaller=%q, want sess-1", fake.lastCaller)
	}
	if fake.lastReq.Tier != "extractor" || fake.lastReq.Brief != "summarize the ticket" {
		t.Errorf("lastReq = %+v, unexpected", fake.lastReq)
	}
}

func TestConsoleDelegate_NilDelegator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "delegate", `{"tier":"extractor","brief":"do it"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "delegator") {
		t.Errorf("out=%q isErr=%v, want missing delegator error", out, isErr)
	}
}

func TestConsoleDelegate_InvalidTier_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Delegator = &fakeDelegator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "delegate", `{"tier":"manager","brief":"do it"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "tier must be") {
		t.Errorf("out=%q isErr=%v, want tier validation error", out, isErr)
	}
}

func TestConsoleDelegate_MissingBrief_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Delegator = &fakeDelegator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "delegate", `{"tier":"extractor","brief":""}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "brief is required") {
		t.Errorf("out=%q isErr=%v, want brief is required", out, isErr)
	}
}

func TestConsoleDelegate_ContextTooLarge_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Delegator = &fakeDelegator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	bigContext := strings.Repeat("x", 4097)
	out, isErr, err := invoke(t, reg, toolEnv, "delegate", `{"tier":"extractor","brief":"do it","context":"`+bigContext+`"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "artifact") {
		t.Errorf("out=%q isErr=%v, want error steering to an artifact", out, isErr)
	}
}

func TestConsoleDelegate_FanoutExceedsCap_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Delegator = &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			t.Fatal("Delegate must not be called once the fanout cap check fails")
			return "", nil
		},
	}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	fanout := make([]string, 21) // default delegate_max_fanout is 20
	for i := range fanout {
		fanout[i] = `"item"`
	}
	args := `{"tier":"extractor","brief":"do it","fanout":[` + strings.Join(fanout, ",") + `]}`

	out, isErr, err := invoke(t, reg, toolEnv, "delegate", args)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "delegate_max_fanout") {
		t.Errorf("out=%q isErr=%v, want fanout cap error", out, isErr)
	}
}

func TestConsoleGetDelegation_HappyPath_RoutesThroughDepsDelegator(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeDelegator{
		getDelegationFn: func(_ context.Context, callerSessionID, delegationID string) (string, error) {
			if callerSessionID != "sess-1" || delegationID != "wfi.abc" {
				t.Errorf("unexpected args: caller=%q delegation=%q", callerSessionID, delegationID)
			}
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}
	env.deps.Delegator = fake
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("out=%q, want completed", out)
	}
}

func TestConsoleGetDelegation_NilDelegator_MissingService(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "delegator") {
		t.Errorf("out=%q isErr=%v, want missing delegator error", out, isErr)
	}
}

func TestConsoleGetDelegation_MissingID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.deps.Delegator = &fakeDelegator{}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "get_delegation", `{}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "delegation_id is required") {
		t.Errorf("out=%q isErr=%v, want delegation_id is required", out, isErr)
	}
}
