package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/ws"
)

func TestWorkflowSkip_PersistsTagAndBroadcasts(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_skip", `{"tag":"frontend"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if isErr || out != "ok" {
		t.Errorf("output=%q isErr=%v want ok", out, isErr)
	}
	got := env.readSkipTags(t)
	if !strings.Contains(got, `"frontend"`) {
		t.Errorf("skip_tags = %q, want contains frontend", got)
	}
	if len(env.hub.events) != 1 || env.hub.events[0].Type != ws.EventSkipTagAdded {
		t.Errorf("expected skip_tag.added event, got %+v", env.hub.events)
	}
	if env.hub.events[0].Data["tag"] != "frontend" {
		t.Errorf("tag = %v, want frontend", env.hub.events[0].Data["tag"])
	}
}

func TestWorkflowSkip_TagNotInGroups(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_skip", `{"tag":"backend"}`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true for tag not in groups")
	}
	if !strings.Contains(out, "not in workflow groups") {
		t.Errorf("output = %q, want contains 'not in workflow groups'", out)
	}
	if len(env.hub.events) != 0 {
		t.Errorf("expected no broadcast on validation failure, got %+v", env.hub.events)
	}
}

func TestWorkflowSkip_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, env.env, "workflow_skip", `not-json`)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output = %q, want invalid arguments", out)
	}
}
