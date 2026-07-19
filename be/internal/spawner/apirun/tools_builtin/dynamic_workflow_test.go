package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun"
)

// TestDynamicWorkflowHandler_SpecDescriptionMentionsPremiumCap guards the
// tier-policy/premium-cap prompt text against drift, mirroring
// TestReadDocumentPathHandler_SpecDescriptionMentionsPath's convention.
func TestDynamicWorkflowHandler_SpecDescriptionMentionsPremiumCap(t *testing.T) {
	spec := (dynamicWorkflowHandler{}).Spec()
	if !strings.Contains(spec.Description, service.PremiumWorkerCapKey) {
		t.Errorf("Spec().Description = %q; want to mention %q", spec.Description, service.PremiumWorkerCapKey)
	}
}

func TestDynamicWorkflow_AsyncStart(t *testing.T) {
	r := stubSubworkflows{
		startDynamic: func(_ context.Context, parentID, _, instructions, mode string) (string, error) {
			if parentID != "wfi-parent" || instructions != "build a widget" || mode != "" {
				t.Errorf("unexpected startDynamic args: parent=%s instructions=%s mode=%q", parentID, instructions, mode)
			}
			return "child-1", nil
		},
	}
	out, isErr, err := dynamicWorkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{WorkflowInstanceID: "wfi-parent", Subworkflows: r},
		json.RawMessage(`{"instructions":"build a widget"}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"child-1"`) || !strings.Contains(out, `"planning"`) {
		t.Errorf("out = %q, want instance_id child-1 with status planning", out)
	}
}

func TestDynamicWorkflow_ModePassthrough(t *testing.T) {
	var gotMode string
	r := stubSubworkflows{
		startDynamic: func(_ context.Context, _, _, _, mode string) (string, error) {
			gotMode = mode
			return "child-2", nil
		},
	}
	out, isErr, err := dynamicWorkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r},
		json.RawMessage(`{"instructions":"go","mode":"auto"}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if gotMode != "auto" {
		t.Errorf("mode passed through = %q, want auto", gotMode)
	}
}

func TestDynamicWorkflow_Validation(t *testing.T) {
	r := stubSubworkflows{startDynamic: func(context.Context, string, string, string, string) (string, error) { return "x", nil }}
	if _, isErr, _ := (dynamicWorkflowHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instructions":""}`)); !isErr {
		t.Error("want isError for empty instructions")
	}
}

func TestDynamicWorkflow_MissingService(t *testing.T) {
	if _, isErr, _ := (dynamicWorkflowHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{}, json.RawMessage(`{"instructions":"go"}`)); !isErr {
		t.Error("want isError when Subworkflows is nil")
	}
}

func TestDynamicWorkflow_DisabledByConfig(t *testing.T) {
	env := newBuiltinTestEnv(t)
	if err := env.pool.SetProjectConfig(testProjectID, service.SubworkflowToolsEnabledKey, "false"); err != nil {
		t.Fatalf("SetProjectConfig: %v", err)
	}
	toolEnv := env.env
	toolEnv.Subworkflows = stubSubworkflows{
		startDynamic: func(context.Context, string, string, string, string) (string, error) {
			t.Fatal("StartDynamicWorkflow should not be called when tools are disabled")
			return "", nil
		},
	}
	out, isErr, err := dynamicWorkflowHandler{}.Invoke(context.Background(), toolEnv, json.RawMessage(`{"instructions":"go"}`))
	if err != nil {
		t.Fatalf("Invoke returned err: %v", err)
	}
	if !isErr || !strings.Contains(out, "disabled") {
		t.Errorf("want isError mentioning disabled, got isErr=%v out=%q", isErr, out)
	}
}

func TestDynamicWorkflow_StartErrorPropagates(t *testing.T) {
	r := stubSubworkflows{startDynamic: func(context.Context, string, string, string, string) (string, error) {
		return "", context.DeadlineExceeded
	}}
	out, isErr, _ := dynamicWorkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instructions":"go"}`))
	if !isErr || !strings.Contains(out, "deadline") {
		t.Errorf("want propagated start error, got isErr=%v out=%q", isErr, out)
	}
}

func TestDynamicWorkflow_BoundedWaitReturnsWaitingApproval(t *testing.T) {
	old := subworkflowPollInterval
	subworkflowPollInterval = time.Millisecond
	defer func() { subworkflowPollInterval = old }()

	var polls int32
	r := stubSubworkflows{
		startDynamic: func(context.Context, string, string, string, string) (string, error) { return "child-3", nil },
		get: func(_ context.Context, _, _, instanceID, _ string) (apirun.SubworkflowState, error) {
			if instanceID != "child-3" {
				t.Errorf("unexpected get instanceID: %s", instanceID)
			}
			switch atomic.AddInt32(&polls, 1) {
			case 1, 2:
				return apirun.SubworkflowState{Status: "planning"}, nil
			default:
				return apirun.SubworkflowState{
					Status:   "waiting_approval",
					Plan:     json.RawMessage(`{"goal":"widget"}`),
					Revision: 1,
				}, nil
			}
		},
	}
	out, isErr, err := dynamicWorkflowHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r},
		json.RawMessage(`{"instructions":"go","wait_sec":5}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"waiting_approval"`) {
		t.Errorf("out = %q, want status waiting_approval", out)
	}
	if !strings.Contains(out, `"goal":"widget"`) {
		t.Errorf("out = %q, want plan payload included", out)
	}
}
