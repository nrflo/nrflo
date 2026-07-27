package tools_builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/types"
)

// --- revise_plan ---

// TestRevisePlanHandler_SpecMentionsPremiumCap guards the tier-policy/
// premium-cap prompt text against drift, mirroring
// TestReadDocumentPathHandler_SpecDescriptionMentionsPath's convention. The cap
// documents the `plan` argument, so the whole spec (description + schema) is
// the surface that must carry it — both reach the model.
func TestRevisePlanHandler_SpecMentionsPremiumCap(t *testing.T) {
	spec := (revisePlanHandler{}).Spec()
	text := spec.Description + string(spec.InputSchema)
	if !strings.Contains(text, service.PremiumWorkerCapKey) {
		t.Errorf("Spec() = %q; want to mention %q", text, service.PremiumWorkerCapKey)
	}
}

func TestRevisePlan_HappyPath(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return &model.PlanRevision{InstanceID: "c1", Revision: 3, Author: "caller"}, nil
		},
	}
	out, isErr, err := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r},
		json.RawMessage(`{"instance_id":"c1","revision":2,"feedback":"tweak it"}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"revision":3`) {
		t.Errorf("out = %q, want revision 3", out)
	}
}

func TestRevisePlan_Validation(t *testing.T) {
	r := stubSubworkflows{revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
		return &model.PlanRevision{}, nil
	}}
	if _, isErr, _ := (revisePlanHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"","revision":0}`)); !isErr {
		t.Error("want isError for empty instance_id")
	}
}

func TestRevisePlan_MissingService(t *testing.T) {
	if _, isErr, _ := (revisePlanHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{}, json.RawMessage(`{"instance_id":"c1","revision":0}`)); !isErr {
		t.Error("want isError when Subworkflows is nil")
	}
}

func TestRevisePlan_ArgsPassthrough(t *testing.T) {
	var got types.PlanReviseRequest
	r := stubSubworkflows{
		revisePlan: func(_ context.Context, _, _, instanceID string, req types.PlanReviseRequest) (*model.PlanRevision, error) {
			if instanceID != "c1" {
				t.Errorf("instanceID = %q, want c1", instanceID)
			}
			got = req
			return &model.PlanRevision{Revision: 1}, nil
		},
	}
	input := json.RawMessage(`{
		"instance_id":"c1",
		"revision":2,
		"plan":{"version":1,"goal":"g"},
		"feedback":"do better",
		"answers":[{"question_id":"q1","answer":"a1"}]
	}`)
	if _, isErr, err := (revisePlanHandler{}).Invoke(context.Background(), apirun.ToolEnv{Subworkflows: r}, input); err != nil || isErr {
		t.Fatalf("Invoke failed: isErr=%v err=%v", isErr, err)
	}
	if got.Revision != 2 {
		t.Errorf("Revision = %d, want 2", got.Revision)
	}
	if got.Feedback != "do better" {
		t.Errorf("Feedback = %q, want %q", got.Feedback, "do better")
	}
	if len(got.Answers) != 1 || got.Answers[0].QuestionID != "q1" || got.Answers[0].Answer != "a1" {
		t.Errorf("Answers = %+v, want one {q1 a1}", got.Answers)
	}
	if !strings.Contains(string(got.Manifest), `"goal":"g"`) {
		t.Errorf("Manifest = %s, want it to carry the plan body", got.Manifest)
	}
}

func TestRevisePlan_StaleRevisionMapping(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return nil, service.ErrStalePlanRevision
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{Revision: 5}, nil
		},
	}
	out, isErr, err := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke returned err: %v", err)
	}
	if !isErr || !strings.Contains(out, "current revision: 5") {
		t.Errorf("out = %q isErr=%v, want isError mentioning current revision: 5", out, isErr)
	}
}

func TestRevisePlan_StaleRevision_GetSubworkflowErrors(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return nil, service.ErrStalePlanRevision
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{}, errors.New("lookup failed")
		},
	}
	out, isErr, err := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke returned err: %v", err)
	}
	if !isErr {
		t.Fatal("want isError")
	}
	if out != service.ErrStalePlanRevision.Error() {
		t.Errorf("out = %q, want bare error text %q (no current-revision suffix)", out, service.ErrStalePlanRevision.Error())
	}
}

func TestRevisePlan_StaleRevision_GetSubworkflowZeroRevision(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return nil, service.ErrStalePlanRevision
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{Revision: 0}, nil
		},
	}
	out, isErr, _ := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if !isErr {
		t.Fatal("want isError")
	}
	if out != service.ErrStalePlanRevision.Error() {
		t.Errorf("out = %q, want bare error text (revision:0 should not add suffix)", out)
	}
}

func TestRevisePlan_NotDraftMapping(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return nil, service.ErrPlanNotDraft
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{Revision: 7}, nil
		},
	}
	out, isErr, _ := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if !isErr || !strings.Contains(out, "current revision: 7") {
		t.Errorf("out = %q isErr=%v, want isError mentioning current revision: 7", out, isErr)
	}
}

func TestRevisePlan_PlainErrorNoGetCall(t *testing.T) {
	r := stubSubworkflows{
		revisePlan: func(context.Context, string, string, string, types.PlanReviseRequest) (*model.PlanRevision, error) {
			return nil, errors.New("boom: some other failure")
		},
		// get intentionally left nil: if planServiceErrorText called it for a
		// plain error, this would panic (nil func call) and fail the test.
	}
	out, isErr, err := revisePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke returned err: %v", err)
	}
	if !isErr || out != "boom: some other failure" {
		t.Errorf("out = %q isErr=%v, want bare plain error text with no suffix", out, isErr)
	}
}

// --- approve_plan ---

func TestApprovePlan_HappyPath(t *testing.T) {
	r := stubSubworkflows{
		approvePlan: func(_ context.Context, _, _, instanceID string, revision int) (*model.PlanRevision, error) {
			if instanceID != "c1" || revision != 2 {
				t.Errorf("unexpected approvePlan args: instanceID=%s revision=%d", instanceID, revision)
			}
			return &model.PlanRevision{InstanceID: "c1", Revision: 2}, nil
		},
	}
	out, isErr, err := approvePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":2}`))
	if err != nil || isErr {
		t.Fatalf("Invoke: %q isErr=%v err=%v", out, isErr, err)
	}
	if !strings.Contains(out, `"revision":2`) {
		t.Errorf("out = %q, want revision 2", out)
	}
}

func TestApprovePlan_Validation(t *testing.T) {
	r := stubSubworkflows{approvePlan: func(context.Context, string, string, string, int) (*model.PlanRevision, error) {
		return &model.PlanRevision{}, nil
	}}
	if _, isErr, _ := (approvePlanHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"","revision":0}`)); !isErr {
		t.Error("want isError for empty instance_id")
	}
}

func TestApprovePlan_MissingService(t *testing.T) {
	if _, isErr, _ := (approvePlanHandler{}).Invoke(context.Background(),
		apirun.ToolEnv{}, json.RawMessage(`{"instance_id":"c1","revision":0}`)); !isErr {
		t.Error("want isError when Subworkflows is nil")
	}
}

func TestApprovePlan_StaleRevisionMapping(t *testing.T) {
	r := stubSubworkflows{
		approvePlan: func(context.Context, string, string, string, int) (*model.PlanRevision, error) {
			return nil, service.ErrStalePlanRevision
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{Revision: 9}, nil
		},
	}
	out, isErr, _ := approvePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if !isErr || !strings.Contains(out, "current revision: 9") {
		t.Errorf("out = %q isErr=%v, want isError mentioning current revision: 9", out, isErr)
	}
}

func TestApprovePlan_NotDraftMapping(t *testing.T) {
	r := stubSubworkflows{
		approvePlan: func(context.Context, string, string, string, int) (*model.PlanRevision, error) {
			return nil, service.ErrPlanNotDraft
		},
		get: func(context.Context, string, string, string, string) (apirun.SubworkflowState, error) {
			return apirun.SubworkflowState{Revision: 4}, nil
		},
	}
	out, isErr, _ := approvePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if !isErr || !strings.Contains(out, "current revision: 4") {
		t.Errorf("out = %q isErr=%v, want isError mentioning current revision: 4", out, isErr)
	}
}

func TestApprovePlan_PlainErrorNoGetCall(t *testing.T) {
	r := stubSubworkflows{
		approvePlan: func(context.Context, string, string, string, int) (*model.PlanRevision, error) {
			return nil, errors.New("nope")
		},
		// get intentionally left nil — a call would panic.
	}
	out, isErr, err := approvePlanHandler{}.Invoke(context.Background(),
		apirun.ToolEnv{Subworkflows: r}, json.RawMessage(`{"instance_id":"c1","revision":1}`))
	if err != nil {
		t.Fatalf("Invoke returned err: %v", err)
	}
	if !isErr || out != "nope" {
		t.Errorf("out = %q isErr=%v, want bare plain error text", out, isErr)
	}
}
