package console

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDispatch_UnknownTool_ReturnsErrToolNotFound(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	_, _, err = Dispatch(context.Background(), reg, toolEnv, "nope", json.RawMessage(`{}`))
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
}

func TestDispatch_KnownTool_EmptyArgsDefaultToEmptyObject(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := Dispatch(context.Background(), reg, toolEnv, "project_list", nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr {
		t.Errorf("isErr = true, want false; out=%s", out)
	}
	var projects []map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &projects); jerr != nil {
		t.Errorf("output does not unmarshal as array: %v", jerr)
	}
}

func TestDispatch_KnownTool_ArgsForwarded(t *testing.T) {
	env := newConsoleTestEnv(t)
	fake := &fakeOrchestrator{}
	env.deps.Orch = fake
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	_, isErr, err := Dispatch(context.Background(), reg, toolEnv, "workflow_run", json.RawMessage(`{"workflow":"w1"}`))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr {
		t.Errorf("isErr = true, want false")
	}
	if fake.startWorkflow != "w1" {
		t.Errorf("startWorkflow = %q, want w1", fake.startWorkflow)
	}
	if fake.startProjectID != testProjectID {
		t.Errorf("startProjectID = %q, want %q", fake.startProjectID, testProjectID)
	}
}
