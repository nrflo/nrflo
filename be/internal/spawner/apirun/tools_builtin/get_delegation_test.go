package tools_builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
)

func TestGetDelegation_HappyPath_ForwardsIDAndCallerSessionID(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeDelegator{
		getDelegationFn: func(_ context.Context, callerSessionID, delegationID string) (string, error) {
			if delegationID != "wfi.abc" {
				t.Errorf("delegationID=%q, want wfi.abc", delegationID)
			}
			if callerSessionID != testSessionID {
				t.Errorf("callerSessionID=%q, want %q", callerSessionID, testSessionID)
			}
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}
	env.env.Delegator = fake

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("out=%q, want completed", out)
	}
}

func TestGetDelegation_NilDelegator_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "delegator") {
		t.Errorf("out=%q isErr=%v, want missing delegator error", out, isErr)
	}
}

func TestGetDelegation_InvalidJSON_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "get_delegation", `not-json`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "invalid arguments") {
		t.Errorf("out=%q isErr=%v, want invalid arguments", out, isErr)
	}
}

func TestGetDelegation_MissingID_Errors(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "get_delegation", `{}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "delegation_id is required") {
		t.Errorf("out=%q isErr=%v, want delegation_id is required", out, isErr)
	}
}

func TestGetDelegation_FailedStatus_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"failed"}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for a failed delegation; out=%q", out)
	}
}

func TestGetDelegation_BareRunning_CarriesWaitSecHint(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "wait_sec") {
		t.Errorf("out=%q, want running result carrying a wait_sec hint", out)
	}
}

func TestGetDelegation_TerminalResult_NoHint(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}

	out, _, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if strings.Contains(out, "hint") {
		t.Errorf("out=%q, want terminal result without a poll hint", out)
	}
}

func TestGetDelegation_WaitExpires_StillRunningCarriesHint(t *testing.T) {
	old := delegatePollInterval
	delegatePollInterval = time.Millisecond
	defer func() { delegatePollInterval = old }()

	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // expire immediately: first poll runs, then ctx.Done wins
	out, isErr, err := getDelegationHandler{}.Invoke(ctx, env.env, []byte(`{"delegation_id":"wfi.abc","wait_sec":5}`))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "wait_sec") {
		t.Errorf("out=%q, want expired-wait running result carrying a wait_sec hint", out)
	}
}

func TestGetDelegation_BareRunning_HintCarriesConsoleBackgroundCaveat(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	for _, anchor := range []string{"background-task", "does not consume", "get_delegation once more"} {
		if !strings.Contains(out, anchor) {
			t.Errorf("out=%q, want hint to contain %q", out, anchor)
		}
	}
}

func TestToolSpecs_DescriptionsCarryConsoleBackgroundCaveat(t *testing.T) {
	for name, spec := range map[string]interface {
		Spec() provider.ToolSpec
	}{
		"get_delegation": getDelegationHandler{},
		"delegate":       delegateHandler{},
	} {
		desc := spec.Spec().Description
		for _, anchor := range []string{"background-task", "does not consume", "get_delegation once more"} {
			if !strings.Contains(desc, anchor) {
				t.Errorf("%s Description missing anchor %q; got %q", name, anchor, desc)
			}
		}
	}
}

func TestGetDelegation_WaitSecPositive_PollsUntilDone(t *testing.T) {
	old := delegatePollInterval
	delegatePollInterval = time.Millisecond
	defer func() { delegatePollInterval = old }()

	env := newBuiltinTestEnv(t)
	polls := 0
	env.env.Delegator = &fakeDelegator{
		getDelegationFn: func(context.Context, string, string) (string, error) {
			polls++
			if polls < 3 {
				return `{"delegation_id":"wfi.abc","status":"running"}`, nil
			}
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "get_delegation", `{"delegation_id":"wfi.abc","wait_sec":5}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("out=%q, want completed after polling", out)
	}
	if polls < 3 {
		t.Errorf("polls=%d, want at least 3", polls)
	}
}
