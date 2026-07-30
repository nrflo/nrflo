package tools_builtin

import (
	"errors"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

// TestAgentFinished_DelegateGuard: a _delegate worker may not finish before
// recording its _delegate_findings — the finding IS the deliverable; once
// recorded, agent_finished passes normally.
func TestAgentFinished_DelegateGuard(t *testing.T) {
	env := newBuiltinTestEnv(t)
	denv := env.env
	denv.NodeID = "_delegate"

	out, isErr, err := invoke(t, denv, "agent_finished", `{}`)
	if err != nil {
		t.Fatalf("guard rejection must not be terminal, got err %v", err)
	}
	if !isErr || !strings.Contains(out, "_delegate_findings") {
		t.Fatalf("out=%q isErr=%v, want tool error naming _delegate_findings", out, isErr)
	}
	if got := env.readSessionResult(t); got != "" {
		t.Fatalf("session.result = %q, want unset after rejected finish", got)
	}

	if out, isErr, err := invoke(t, denv, "findings_add", `{"key":"_delegate_findings","value":"{\"answer\":\"x\"}"}`); isErr || err != nil {
		t.Fatalf("findings_add: out=%q isErr=%v err=%v", out, isErr, err)
	}
	_, _, err = invoke(t, denv, "agent_finished", `{}`)
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) || ts.Status != "PASS" {
		t.Fatalf("err = %v, want PASS TerminalSignal after findings recorded", err)
	}
}

// TestAgentFinished_NonDelegateUnaffected: ordinary nodes keep finishing
// without any findings requirement.
func TestAgentFinished_NonDelegateUnaffected(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "agent_finished", `{}`)
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) || ts.Status != "PASS" {
		t.Fatalf("err = %v, want PASS TerminalSignal", err)
	}
}
