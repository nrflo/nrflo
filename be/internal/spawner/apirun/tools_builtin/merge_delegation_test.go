package tools_builtin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMergeDelegation_HappyPath_ForwardsCallerAndID(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeDelegator{
		mergeDelegationFn: func(_ context.Context, _, delegationID string) (string, error) {
			if delegationID != "wfi.abc" {
				t.Errorf("delegationID = %q, want wfi.abc", delegationID)
			}
			return `{"delegation_id":"wfi.abc","status":"merged","merge_commit":"sha1"}`, nil
		},
	}
	env.env.Delegator = fake

	out, isErr, err := invoke(t, env.env, "merge_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false; out=%q", out)
	}
	if !strings.Contains(out, "merged") || !strings.Contains(out, "sha1") {
		t.Errorf("out=%q, want merged status with commit", out)
	}
	if fake.lastCaller != testSessionID {
		t.Errorf("lastCaller=%q, want %q", fake.lastCaller, testSessionID)
	}
}

func TestMergeDelegation_NilDelegator_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)

	out, isErr, err := invoke(t, env.env, "merge_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true; out=%q", out)
	}
}

func TestMergeDelegation_EmptyID_Rejected(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{}

	out, isErr, err := invoke(t, env.env, "merge_delegation", `{"delegation_id":"  "}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "required") {
		t.Errorf("out=%q isErr=%v, want required-field rejection", out, isErr)
	}
}

func TestMergeDelegation_DelegatorError_SurfacesAsToolError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.Delegator = &fakeDelegator{
		mergeDelegationFn: func(context.Context, string, string) (string, error) {
			return "", errors.New("delegate merge: live tree has uncommitted changes")
		},
	}

	out, isErr, err := invoke(t, env.env, "merge_delegation", `{"delegation_id":"wfi.abc"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr || !strings.Contains(out, "uncommitted") {
		t.Errorf("out=%q isErr=%v, want the delegator error surfaced", out, isErr)
	}
}
