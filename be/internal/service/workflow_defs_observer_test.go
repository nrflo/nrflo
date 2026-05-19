package service

import (
	"testing"

	"be/internal/types"
)

// TestCreateWorkflowDef_ObserverContext verifies observer_context is persisted.
func TestCreateWorkflowDef_ObserverContext(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:              "wf-obs-ctx",
		ObserverContext: "watch for regressions",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-obs-ctx")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverContext != "watch for regressions" {
		t.Errorf("ObserverContext = %q, want %q", wf.ObserverContext, "watch for regressions")
	}
}

// TestCreateWorkflowDef_ObserverProviderAndModel verifies observer_provider and observer_model are persisted.
func TestCreateWorkflowDef_ObserverProviderAndModel(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	provider := "openai"
	model := "gpt-5"
	_, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:               "wf-obs-pm",
		ObserverProvider: &provider,
		ObserverModel:    &model,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-obs-pm")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverProvider == nil || *wf.ObserverProvider != provider {
		t.Errorf("ObserverProvider = %v, want %q", wf.ObserverProvider, provider)
	}
	if wf.ObserverModel == nil || *wf.ObserverModel != model {
		t.Errorf("ObserverModel = %v, want %q", wf.ObserverModel, model)
	}
}

// TestGetWorkflowDef_ObserverFieldsDefaultEmpty verifies nil/empty defaults on fresh workflow.
func TestGetWorkflowDef_ObserverFieldsDefaultEmpty(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{ID: "wf-no-obs"}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-no-obs")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverContext != "" {
		t.Errorf("ObserverContext = %q, want empty", wf.ObserverContext)
	}
	if wf.ObserverProvider != nil {
		t.Errorf("ObserverProvider = %v, want nil", wf.ObserverProvider)
	}
	if wf.ObserverModel != nil {
		t.Errorf("ObserverModel = %v, want nil", wf.ObserverModel)
	}
}

// TestUpdateWorkflowDef_ObserverContext verifies observer_context can be updated.
func TestUpdateWorkflowDef_ObserverContext(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID: "wf-upd-obs",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	ctx := "updated context"
	if err := svc.UpdateWorkflowDef("proj1", "wf-upd-obs", &types.WorkflowDefUpdateRequest{
		ObserverContext: &ctx,
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-upd-obs")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverContext != "updated context" {
		t.Errorf("ObserverContext = %q, want %q", wf.ObserverContext, "updated context")
	}
}

// TestUpdateWorkflowDef_ObserverProviderModel verifies provider and model can be set via update.
func TestUpdateWorkflowDef_ObserverProviderModel(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID: "wf-upd-pm",
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	if err := svc.UpdateWorkflowDef("proj1", "wf-upd-pm", &types.WorkflowDefUpdateRequest{
		ObserverProvider: strPtr("gemini"),
		ObserverModel:    strPtr("flash"),
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-upd-pm")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverProvider == nil || *wf.ObserverProvider != "gemini" {
		t.Errorf("ObserverProvider = %v, want gemini", wf.ObserverProvider)
	}
	if wf.ObserverModel == nil || *wf.ObserverModel != "flash" {
		t.Errorf("ObserverModel = %v, want flash", wf.ObserverModel)
	}
}

// TestUpdateWorkflowDef_ObserverNilFieldNoChange verifies nil update fields do not overwrite existing values.
func TestUpdateWorkflowDef_ObserverNilFieldNoChange(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:              "wf-nil-obs",
		ObserverContext: "original-ctx",
		ObserverProvider: strPtr("claude"),
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	// Update description only — observer fields must be unchanged.
	desc := "new description"
	if err := svc.UpdateWorkflowDef("proj1", "wf-nil-obs", &types.WorkflowDefUpdateRequest{
		Description: &desc,
	}); err != nil {
		t.Fatalf("UpdateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-nil-obs")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}
	if wf.ObserverContext != "original-ctx" {
		t.Errorf("ObserverContext = %q, want original-ctx (should be unchanged)", wf.ObserverContext)
	}
	if wf.ObserverProvider == nil || *wf.ObserverProvider != "claude" {
		t.Errorf("ObserverProvider = %v, want claude (should be unchanged)", wf.ObserverProvider)
	}
}

// TestListWorkflowDefs_ObserverFields verifies ListWorkflowDefs returns observer fields.
func TestListWorkflowDefs_ObserverFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:              "wf-list-obs",
		ObserverContext: "list-ctx",
		ObserverProvider: strPtr("claude"),
		ObserverModel:    strPtr("opus"),
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	defs, err := svc.ListWorkflowDefs("proj1")
	if err != nil {
		t.Fatalf("ListWorkflowDefs: %v", err)
	}
	wf, ok := defs["wf-list-obs"]
	if !ok {
		t.Fatal("wf-list-obs not in ListWorkflowDefs result")
	}
	if wf.ObserverContext != "list-ctx" {
		t.Errorf("ObserverContext = %q, want list-ctx", wf.ObserverContext)
	}
	if wf.ObserverProvider == nil || *wf.ObserverProvider != "claude" {
		t.Errorf("ObserverProvider = %v, want claude", wf.ObserverProvider)
	}
	if wf.ObserverModel == nil || *wf.ObserverModel != "opus" {
		t.Errorf("ObserverModel = %v, want opus", wf.ObserverModel)
	}
}

// TestWorkflowDefMarshalJSON_ObserverFields verifies JSON serialization includes observer fields.
func TestWorkflowDefMarshalJSON_ObserverFields(t *testing.T) {
	t.Parallel()
	_, svc := setupWorkflowDefsTestEnv(t)

	if _, err := svc.CreateWorkflowDef("proj1", &types.WorkflowDefCreateRequest{
		ID:               "wf-json-obs",
		ObserverContext:  "json-ctx",
		ObserverProvider: strPtr("claude"),
		ObserverModel:    strPtr("sonnet"),
	}); err != nil {
		t.Fatalf("CreateWorkflowDef: %v", err)
	}

	wf, err := svc.GetWorkflowDef("proj1", "wf-json-obs")
	if err != nil {
		t.Fatalf("GetWorkflowDef: %v", err)
	}

	// MarshalJSON should include observer fields.
	data, err := wf.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"observer_context":"json-ctx"`, `"observer_provider":"claude"`, `"observer_model":"sonnet"`} {
		found := false
		for i := 0; i <= len(s)-len(want); i++ {
			if s[i:i+len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("JSON missing %q; got %s", want, s)
		}
	}
}
