package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/spawner/apirun"
)

// stubSubworkflows adapts funcs to apirun.SubworkflowRunner.
type stubSubworkflows struct {
	start func(ctx context.Context, parentID, projectID, workflow, instructions string) (string, error)
	get   func(ctx context.Context, projectID, instanceID, resultKey string) (string, json.RawMessage, string, error)
}

func (s stubSubworkflows) StartSubworkflow(ctx context.Context, parentID, projectID, workflow, instructions string) (string, error) {
	return s.start(ctx, parentID, projectID, workflow, instructions)
}

func (s stubSubworkflows) GetSubworkflow(ctx context.Context, projectID, instanceID, resultKey string) (string, json.RawMessage, string, error) {
	return s.get(ctx, projectID, instanceID, resultKey)
}

func TestRunSubworkflow_AsyncStart(t *testing.T) {
	r := stubSubworkflows{
		start: func(_ context.Context, parentID, _, workflow, instructions string) (string, error) {
			if parentID != "wfi-parent" || workflow != "deep-research" || instructions != "q" {
				t.Errorf("unexpected start args: %s %s %s", parentID, workflow, instructions)
			}
			return "child-1", nil
		},
	}
	out, isErr, err := runSubworkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{WorkflowInstanceID: "wfi-parent", Subworkflows: r},
		json.RawMessage(`{"workflow":"deep-research","instructions":"q"}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"child-1"`) || !strings.Contains(out, `"running"`) {
		t.Errorf("out = %q, want instance_id child-1 with status running", out)
	}
}

func TestRunSubworkflow_BoundedWaitReturnsResult(t *testing.T) {
	old := subworkflowPollInterval
	subworkflowPollInterval = time.Millisecond
	defer func() { subworkflowPollInterval = old }()

	var polls int32
	r := stubSubworkflows{
		start: func(context.Context, string, string, string, string) (string, error) { return "child-2", nil },
		get: func(_ context.Context, _, instanceID, resultKey string) (string, json.RawMessage, string, error) {
			if instanceID != "child-2" || resultKey != "report" {
				t.Errorf("unexpected get args: %s %s", instanceID, resultKey)
			}
			if atomic.AddInt32(&polls, 1) < 3 {
				return "running", nil, "", nil
			}
			return "completed", json.RawMessage(`{"summary":"done"}`), "", nil
		},
	}
	out, isErr, err := runSubworkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{WorkflowInstanceID: "p", Subworkflows: r},
		json.RawMessage(`{"workflow":"deep-research","instructions":"q","result_key":"report","wait_sec":5}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"completed"`) || !strings.Contains(out, "done") {
		t.Errorf("out = %q, want completed with result", out)
	}
}

func TestRunSubworkflow_Validation(t *testing.T) {
	r := stubSubworkflows{start: func(context.Context, string, string, string, string) (string, error) { return "x", nil }}
	if _, isErr, _ := (runSubworkflowHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"workflow":"","instructions":"q"}`)); !isErr {
		t.Error("want isError for empty workflow")
	}
	if _, isErr, _ := (runSubworkflowHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{}, json.RawMessage(`{"workflow":"w","instructions":"q"}`)); !isErr {
		t.Error("want isError when Subworkflows is nil")
	}
}

func TestRunSubworkflow_StartErrorPropagates(t *testing.T) {
	r := stubSubworkflows{start: func(context.Context, string, string, string, string) (string, error) {
		return "", context.DeadlineExceeded
	}}
	out, isErr, _ := runSubworkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"workflow":"w","instructions":"q"}`))
	if !isErr || !strings.Contains(out, "deadline") {
		t.Errorf("want propagated start error, got isErr=%v out=%q", isErr, out)
	}
}

func TestGetSubworkflow_TerminalStatuses(t *testing.T) {
	r := stubSubworkflows{get: func(context.Context, string, string, string) (string, json.RawMessage, string, error) {
		return "failed", nil, "boom", nil
	}}
	out, isErr, _ := getSubworkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1"}`))
	if !isErr || !strings.Contains(out, "boom") {
		t.Errorf("failed child should surface isError with reason, got isErr=%v out=%q", isErr, out)
	}

	r.get = func(context.Context, string, string, string) (string, json.RawMessage, string, error) {
		return "completed", json.RawMessage(`{"ok":true}`), "", nil
	}
	out, isErr, _ = getSubworkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1"}`))
	if isErr || !strings.Contains(out, `"ok":true`) {
		t.Errorf("completed child should return result, got isErr=%v out=%q", isErr, out)
	}
}

func TestGetSubworkflow_RequiresInstanceID(t *testing.T) {
	r := stubSubworkflows{}
	if _, isErr, _ := (getSubworkflowHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{}`)); !isErr {
		t.Error("want isError for missing instance_id")
	}
}
