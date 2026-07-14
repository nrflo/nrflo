package console

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowGet_CrossProjectInstanceID_Rejected(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testOtherProjectID, "wfi-get-other")
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_get", `{"instance_id":"wfi-get-other"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr = false, want true for cross-project instance_id; out=%s", out)
	}
}

func TestWorkflowGet_HappyPath_ReturnsV4State(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedWorkflowInstance(t, testProjectID, "wfi-get-own")
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_get", `{"instance_id":"wfi-get-own"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var state map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &state); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	if state["instance_id"] != "wfi-get-own" {
		t.Errorf("instance_id = %v, want wfi-get-own", state["instance_id"])
	}
	if state["status"] != "active" {
		t.Errorf("status = %v, want active", state["status"])
	}
}

func TestWorkflowGet_MissingInstanceID_Errors(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_get", `{}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr || !strings.Contains(out, "instance_id is required") {
		t.Errorf("out=%q isErr=%v, want instance_id is required", out, isErr)
	}
}

func TestWorkflowList_ReturnsProjectDefs(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID)

	out, isErr, err := invoke(t, reg, toolEnv, "workflow_list", `{}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var defs map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &defs); jerr != nil {
		t.Fatalf("output does not unmarshal as object: %v", jerr)
	}
}
