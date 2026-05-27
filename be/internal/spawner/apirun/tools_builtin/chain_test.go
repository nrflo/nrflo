package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

type fakeChainRun struct {
	instrInstanceID string
	instrValue      string
	ticketInstance  string
	ticketValue     string
	instrErr        error
	ticketErr       error
}

func (f *fakeChainRun) SetNextStepInstructions(instanceID, instructions string) error {
	f.instrInstanceID = instanceID
	f.instrValue = instructions
	return f.instrErr
}

func (f *fakeChainRun) SetNextStepTicket(instanceID, ticketID string) error {
	f.ticketInstance = instanceID
	f.ticketValue = ticketID
	return f.ticketErr
}

func TestChainNextInstructions_NilControl_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "chain_next_instructions", `{"instructions":"next"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "chain_run") {
		t.Errorf("output=%q isErr=%v, want isErr + contains 'chain_run'", out, isErr)
	}
}

func TestChainNextInstructions_HappyPath(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeChainRun{}
	env.env.ChainRun = fake

	out, isErr, err := invoke(t, env.env, "chain_next_instructions", `{"instructions":"do the next thing"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr || out != "ok" {
		t.Errorf("output=%q isErr=%v, want ok/false", out, isErr)
	}
	if fake.instrInstanceID != testInstanceID {
		t.Errorf("instanceID = %q, want %q (from env.WorkflowInstanceID)", fake.instrInstanceID, testInstanceID)
	}
	if fake.instrValue != "do the next thing" {
		t.Errorf("instructions = %q, want 'do the next thing'", fake.instrValue)
	}
}

func TestChainNextTicket_HappyPath(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeChainRun{}
	env.env.ChainRun = fake

	out, isErr, err := invoke(t, env.env, "chain_next_ticket", `{"ticket_id":"T-77"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr || out != "ok" {
		t.Errorf("output=%q isErr=%v, want ok/false", out, isErr)
	}
	if fake.ticketInstance != testInstanceID {
		t.Errorf("instanceID = %q, want %q", fake.ticketInstance, testInstanceID)
	}
	if fake.ticketValue != "T-77" {
		t.Errorf("ticket_id = %q, want T-77", fake.ticketValue)
	}
}

func TestChainNext_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	env.env.ChainRun = &fakeChainRun{}
	out, isErr, err := invoke(t, env.env, "chain_next_ticket", `not-json`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "invalid arguments") {
		t.Errorf("output=%q isErr=%v, want isErr + 'invalid arguments'", out, isErr)
	}
}

// Compile-time check: fakeChainRun satisfies apirun.ChainRunController.
var _ apirun.ChainRunController = (*fakeChainRun)(nil)
