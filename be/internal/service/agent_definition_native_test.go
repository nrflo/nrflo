package service

import (
	"errors"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

// TestCreateAgentDef_NativeFields_ValidationMatrix exercises
// validateNativeFields through CreateAgentDef: native_tools is claude-only
// (cli_interactive + anthropic), sandbox is codex-only (cli_interactive +
// openai), bad sandbox enums and non-sole 'none' are rejected.
func TestCreateAgentDef_NativeFields_ValidationMatrix(t *testing.T) {
	t.Parallel()
	svc, settingsSvc, wfID := setupAgentDefAPIModeEnv(t)
	if err := settingsSvc.Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("Set api_mode_enabled: %v", err)
	}

	cases := []struct {
		name          string
		executionMode string
		model         string
		nativeTools   string
		sandbox       string
		wantErr       bool
	}{
		{"native_tools on anthropic cli accepted", "cli_interactive", "sonnet-5", "Read,Grep", "", false},
		{"native_tools none sentinel accepted", "cli_interactive", "sonnet-5", "none", "", false},
		{"native_tools none plus others rejected", "cli_interactive", "sonnet-5", "none,Read", "", true},
		{"native_tools empty-entries-only rejected", "cli_interactive", "sonnet-5", ",,", "", true},
		{"native_tools on openai cli rejected", "cli_interactive", "gpt-5.6-sol", "Read", "", true},
		{"native_tools on api mode rejected", "api", "sonnet-5", "Read", "", true},
		{"sandbox read-only on openai cli accepted", "cli_interactive", "gpt-5.6-sol", "", model.SandboxReadOnly, false},
		{"sandbox workspace-write on openai cli accepted", "cli_interactive", "gpt-5.6-sol", "", model.SandboxWorkspaceWrite, false},
		{"sandbox on anthropic cli rejected", "cli_interactive", "sonnet-5", "", model.SandboxReadOnly, true},
		{"sandbox on api mode rejected", "api", "gpt-5.6-sol", "", model.SandboxReadOnly, true},
		{"sandbox bad enum rejected", "cli_interactive", "gpt-5.6-sol", "", "readonly", true},
		{"both empty accepted anywhere", "api", "gpt-5.6-sol", "", "", false},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "native-agent-" + string(rune('a'+i))
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:            id,
				Prompt:        "do work",
				ExecutionMode: tc.executionMode,
				Model:         tc.model,
				NativeTools:   tc.nativeTools,
				Sandbox:       tc.sandbox,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("error not tagged ErrValidation: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateAgentDef: %v", err)
			}
		})
	}
}

// TestCreateAgentDef_NativeTools_Normalized verifies CSV normalization
// (trimmed entries, dropped empties) on create.
func TestCreateAgentDef_NativeTools_Normalized(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:          "native-normalize",
		Prompt:      "do work",
		Model:       "sonnet-5",
		NativeTools: " Read , Grep ,",
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.NativeTools != "Read,Grep" {
		t.Errorf("NativeTools = %q, want normalized Read,Grep", def.NativeTools)
	}

	got, err := svc.GetAgentDef("proj1", wfID, "native-normalize")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if got.NativeTools != "Read,Grep" {
		t.Errorf("Get NativeTools = %q, want Read,Grep", got.NativeTools)
	}
}

// TestUpdateAgentDef_NativeFields_PatchSafetyNet verifies the merged-values
// revalidation: swapping only the model to another provider while a stale
// restriction stays on the row must 400, and clearing the field in the same
// PATCH must succeed.
func TestUpdateAgentDef_NativeFields_PatchSafetyNet(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:          "native-patch",
		Prompt:      "do work",
		Model:       "sonnet-5",
		NativeTools: "Read",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Model swap to openai with stale native_tools → rejected, row untouched.
	newModel := "gpt-5.6-sol"
	err := svc.UpdateAgentDef("proj1", wfID, "native-patch", &types.AgentDefUpdateRequest{Model: &newModel})
	if err == nil {
		t.Fatal("UpdateAgentDef(model=gpt-5.6-sol) with stale native_tools: expected error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error not tagged ErrValidation: %v", err)
	}
	def, getErr := svc.GetAgentDef("proj1", wfID, "native-patch")
	if getErr != nil {
		t.Fatalf("GetAgentDef: %v", getErr)
	}
	if def.Model != "sonnet-5" || def.NativeTools != "Read" {
		t.Errorf("row after rejected PATCH = (%q, %q), want unchanged (sonnet-5, Read)", def.Model, def.NativeTools)
	}

	// Same swap with native_tools cleared in the same PATCH → accepted.
	empty := ""
	if err := svc.UpdateAgentDef("proj1", wfID, "native-patch", &types.AgentDefUpdateRequest{
		Model: &newModel, NativeTools: &empty,
	}); err != nil {
		t.Fatalf("UpdateAgentDef(model swap + clear native_tools): %v", err)
	}
	def, getErr = svc.GetAgentDef("proj1", wfID, "native-patch")
	if getErr != nil {
		t.Fatalf("GetAgentDef after clear: %v", getErr)
	}
	if def.Model != "gpt-5.6-sol" || def.NativeTools != "" {
		t.Errorf("row after accepted PATCH = (%q, %q), want (gpt-5.6-sol, \"\")", def.Model, def.NativeTools)
	}

	// Sandbox now settable on the openai def; sets and round-trips.
	sandbox := model.SandboxReadOnly
	if err := svc.UpdateAgentDef("proj1", wfID, "native-patch", &types.AgentDefUpdateRequest{Sandbox: &sandbox}); err != nil {
		t.Fatalf("UpdateAgentDef(sandbox=read-only): %v", err)
	}
	def, getErr = svc.GetAgentDef("proj1", wfID, "native-patch")
	if getErr != nil {
		t.Fatalf("GetAgentDef after sandbox: %v", getErr)
	}
	if def.Sandbox != model.SandboxReadOnly {
		t.Errorf("Sandbox = %q, want read-only", def.Sandbox)
	}
}

// TestUpdateAgentDef_NativeFields_DirectPatchValidated verifies direct PATCHes
// of the new fields are validated against the current row's mode/provider.
func TestUpdateAgentDef_NativeFields_DirectPatchValidated(t *testing.T) {
	t.Parallel()
	svc, _, wfID := setupAgentDefAPIModeEnv(t)

	if _, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:     "native-direct",
		Prompt: "do work",
		Model:  "sonnet-5",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	sandbox := model.SandboxReadOnly
	if err := svc.UpdateAgentDef("proj1", wfID, "native-direct", &types.AgentDefUpdateRequest{Sandbox: &sandbox}); err == nil {
		t.Fatal("UpdateAgentDef(sandbox) on anthropic def: expected error, got nil")
	}

	native := "Edit,Write"
	if err := svc.UpdateAgentDef("proj1", wfID, "native-direct", &types.AgentDefUpdateRequest{NativeTools: &native}); err != nil {
		t.Fatalf("UpdateAgentDef(native_tools) on anthropic def: %v", err)
	}
	def, err := svc.GetAgentDef("proj1", wfID, "native-direct")
	if err != nil {
		t.Fatalf("GetAgentDef: %v", err)
	}
	if def.NativeTools != "Edit,Write" {
		t.Errorf("NativeTools = %q, want Edit,Write", def.NativeTools)
	}
}
