package tools_builtin

import (
	"errors"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
	"be/internal/ws"
)

func TestAgentFail_TerminalSignalAndDB(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "agent_fail", `{"reason":"boom"}`)
	if isErr {
		t.Errorf("isErr = true, want false (terminal signals are not isError)")
	}
	if out != "" {
		t.Errorf("output = %q, want empty", out)
	}
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) {
		t.Fatalf("err = %v, want TerminalSignal", err)
	}
	if ts.Status != "FAIL" {
		t.Errorf("Status = %q, want FAIL", ts.Status)
	}
	if ts.Reason != "boom" {
		t.Errorf("Reason = %q, want boom", ts.Reason)
	}
	if got := env.readSessionResult(t); got != "fail" {
		t.Errorf("session.result = %q, want fail", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventAgentCompleted {
		t.Errorf("expected agent.completed broadcast, got %+v", env.hub.events)
	}
	if env.hub.events[0].Data["action"] != "fail" {
		t.Errorf("action = %v, want fail", env.hub.events[0].Data["action"])
	}
}

func TestAgentFail_NoReason(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "agent_fail", ``)
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) {
		t.Fatalf("err = %v, want TerminalSignal", err)
	}
	if ts.Status != "FAIL" || ts.Reason != "" {
		t.Errorf("ts = %+v, want FAIL with empty Reason", ts)
	}
	if got := env.readSessionResult(t); got != "fail" {
		t.Errorf("session.result = %q, want fail", got)
	}
}

func TestAgentContinue_TerminalSignalAndDB(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "agent_continue", `{}`)
	if isErr || out != "" {
		t.Errorf("output=%q isErr=%v want empty/false", out, isErr)
	}
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) {
		t.Fatalf("err = %v, want TerminalSignal", err)
	}
	if ts.Status != "CONTINUE" {
		t.Errorf("Status = %q, want CONTINUE", ts.Status)
	}
	if got := env.readSessionResult(t); got != "continue" {
		t.Errorf("session.result = %q, want continue", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventAgentContinued {
		t.Errorf("expected agent.continued event, got %+v", env.hub.events)
	}
}

func TestAgentCallback_TerminalSignalAndLevelFinding(t *testing.T) {
	env := newBuiltinTestEnv(t)
	_, _, err := invoke(t, env.env, "agent_callback", `{"level":2}`)
	var ts apirun.TerminalSignal
	if !errors.As(err, &ts) {
		t.Fatalf("err = %v, want TerminalSignal", err)
	}
	if ts.Status != "CALLBACK" {
		t.Errorf("Status = %q, want CALLBACK", ts.Status)
	}
	if ts.Level != 2 {
		t.Errorf("Level = %d, want 2", ts.Level)
	}
	if got := env.readSessionResult(t); got != "callback" {
		t.Errorf("session.result = %q, want callback", got)
	}
	got := env.readSessionFindings(t)
	if !strings.Contains(got, `"callback_level":2`) {
		t.Errorf("findings = %s, want callback_level=2", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventAgentCompleted {
		t.Errorf("expected agent.completed event, got %+v", env.hub.events)
	}
	if env.hub.events[0].Data["action"] != "callback" {
		t.Errorf("action = %v, want callback", env.hub.events[0].Data["action"])
	}
}

func TestAgentContextUpdate_NonTerminal(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "agent_context_update", `{"context_left":42}`)
	if err != nil {
		t.Fatalf("err = %v, want nil (non-terminal)", err)
	}
	if isErr {
		t.Fatalf("isErr=true output=%q", out)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	if got := env.readSessionContextLeft(t); got != 42 {
		t.Errorf("context_left = %d, want 42", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventAgentContextUpdated {
		t.Errorf("expected agent.context_updated event, got %+v", env.hub.events)
	}
}

func TestAgentCallback_InvalidLevel(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "agent_callback", `not-json`)
	if err != nil {
		t.Fatalf("err = %v, want nil for validation failure", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for invalid input")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output = %q, want invalid arguments", out)
	}
}
