package tools_builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun"
)

// fakeDelegator adapts func fields to apirun.Delegator, mirroring
// stubSubworkflows (run_subworkflow_test.go) — tests set only the methods
// they exercise.
type fakeDelegator struct {
	delegateFn      func(ctx context.Context, callerSessionID string, req apirun.DelegateRequest) (string, error)
	getDelegationFn func(ctx context.Context, callerSessionID, delegationID string) (string, error)
	lastCaller      string
	lastReq         apirun.DelegateRequest
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

func TestDelegate_HappyPath_ForwardsFieldsAndCallerSessionID(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
	}
	env.env.Delegator = fake

	out, isErr, err := invoke(t, env.env, "delegate",
		`{"tier":"executor","brief":"do the thing","context":"some context","artifacts":["a.txt","b.txt"],"fanout":["x","y"]}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "wfi.abc") || !strings.Contains(out, "running") {
		t.Errorf("out=%q, want delegation_id wfi.abc status running", out)
	}
	if fake.lastCaller != testSessionID {
		t.Errorf("lastCaller=%q, want %q", fake.lastCaller, testSessionID)
	}
	if fake.lastReq.Tier != "executor" || fake.lastReq.Brief != "do the thing" || fake.lastReq.Context != "some context" {
		t.Errorf("lastReq = %+v, unexpected", fake.lastReq)
	}
	if len(fake.lastReq.Artifacts) != 2 || len(fake.lastReq.Fanout) != 2 {
		t.Errorf("lastReq = %+v, want artifacts/fanout of len 2", fake.lastReq)
	}
}

func TestDelegate_NilDelegator_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	// Delegator is nil by default.

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"extractor","brief":"do it"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "delegator") {
		t.Errorf("out=%q, want contains 'delegator'", out)
	}
}

func TestDelegate_InvalidJSON_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "delegate", `not-json`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "invalid arguments") {
		t.Errorf("out=%q isErr=%v, want invalid arguments", out, isErr)
	}
}

func TestDelegate_MissingBrief_Errors(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"extractor","brief":"  "}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "brief is required") {
		t.Errorf("out=%q isErr=%v, want brief is required", out, isErr)
	}
}

func TestDelegate_InvalidTier_Errors(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"manager","brief":"do it"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, `tier must be`) {
		t.Errorf("out=%q isErr=%v, want tier validation error", out, isErr)
	}
}

func TestDelegate_ContextTooLarge_StearsToArtifact(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	bigContext := strings.Repeat("x", delegateMaxContextBytes+1)
	out, isErr, err := invoke(t, env.env, "delegate",
		`{"tier":"extractor","brief":"do it","context":"`+bigContext+`"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "artifact") {
		t.Errorf("out=%q isErr=%v, want error steering to an artifact", out, isErr)
	}
}

func TestDelegate_FanoutExceedsCap_Errors(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			t.Fatal("Delegate must not be called once the fanout cap check fails")
			return "", nil
		},
	}

	fanout := make([]string, 21) // default cap is 20
	for i := range fanout {
		fanout[i] = "item"
	}
	raw, _ := json.Marshal(map[string]interface{}{"tier": "extractor", "brief": "do it", "fanout": fanout})

	out, isErr, err := invoke(t, env.env, "delegate", string(raw))
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "delegate_max_fanout") {
		t.Errorf("out=%q isErr=%v, want fanout cap error", out, isErr)
	}
}

func TestDelegate_DelegateErrorPropagates(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return "", errors.New("no such tier definition")
		},
	}

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"executor","brief":"do it"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || out != "no such tier definition" {
		t.Errorf("out=%q isErr=%v, want propagated Delegate error", out, isErr)
	}
}

func TestDelegate_WaitSecPositive_PollsUntilDone(t *testing.T) {
	old := delegatePollInterval
	delegatePollInterval = time.Millisecond
	defer func() { delegatePollInterval = old }()

	env := newBuiltinTestEnv(t)
	polls := 0
	env.env.Delegator = &fakeDelegator{
		delegateFn: func(context.Context, string, apirun.DelegateRequest) (string, error) {
			return `{"delegation_id":"wfi.abc","status":"running"}`, nil
		},
		getDelegationFn: func(context.Context, string, string) (string, error) {
			polls++
			if polls < 3 {
				return `{"delegation_id":"wfi.abc","status":"running"}`, nil
			}
			return `{"delegation_id":"wfi.abc","status":"completed","results":[{"status":"completed"}]}`, nil
		},
	}

	out, isErr, err := invoke(t, env.env, "delegate", `{"tier":"extractor","brief":"do it","wait_sec":5}`)
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
