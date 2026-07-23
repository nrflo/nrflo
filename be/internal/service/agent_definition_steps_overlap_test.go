package service

// Tests for validatePathOverlap (P1's cross-key file-ownership gate) — see
// agent_definition_steps.go.

import (
	"errors"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/types"
)

func stepWithOverlap(overlap *model.PathOverlap) model.StepDefinition {
	s := validStep("s1")
	s.PathOverlap = overlap
	return s
}

func TestCreateAgentDef_PathOverlap_NilIsOK(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	steps := []model.StepDefinition{stepWithOverlap(nil)}
	_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:         "overlap-nil",
		Prompt:     "do work",
		PromptMode: PromptModeStepwise,
		Steps:      &steps,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
}

func TestCreateAgentDef_PathOverlap_RejectionMatrix(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)

	tooManyKeys := make([]string, 11)
	for i := range tooManyKeys {
		tooManyKeys[i] = "k" + string(rune('a'+i))
	}
	longKey := strings.Repeat("k", 129)

	cases := []struct {
		name    string
		overlap *model.PathOverlap
	}{
		{"empty left", &model.PathOverlap{Left: nil, Right: []string{"fe_a"}}},
		{"empty right", &model.PathOverlap{Left: []string{"be_a"}, Right: nil}},
		{"both empty", &model.PathOverlap{Left: []string{}, Right: []string{}}},
		{"too many keys on left", &model.PathOverlap{Left: tooManyKeys, Right: []string{"fe_a"}}},
		{"too many keys on right", &model.PathOverlap{Left: []string{"be_a"}, Right: tooManyKeys}},
		{"whitespace key on left", &model.PathOverlap{Left: []string{"has space"}, Right: []string{"fe_a"}}},
		{"empty-string key on right", &model.PathOverlap{Left: []string{"be_a"}, Right: []string{""}}},
		{"oversized key", &model.PathOverlap{Left: []string{longKey}, Right: []string{"fe_a"}}},
		{"key present in both groups", &model.PathOverlap{Left: []string{"shared_key"}, Right: []string{"shared_key"}}},
		{"key present in both groups among others", &model.PathOverlap{Left: []string{"be_a", "dup"}, Right: []string{"fe_a", "dup"}}},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps := []model.StepDefinition{stepWithOverlap(tc.overlap)}
			_, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
				ID:         "overlap-reject-" + string(rune('a'+i)),
				Prompt:     "do work",
				PromptMode: PromptModeStepwise,
				Steps:      &steps,
			})
			if err == nil {
				t.Fatalf("CreateAgentDef(%s): expected error, got nil", tc.name)
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("CreateAgentDef(%s): error = %v, want errors.Is(err, ErrValidation)", tc.name, err)
			}
		})
	}
}

// TestCreateAgentDef_PathOverlap_HappyPathRoundTrips verifies a well-formed
// path_overlap gate round-trips through canonical JSON.
func TestCreateAgentDef_PathOverlap_HappyPathRoundTrips(t *testing.T) {
	t.Parallel()
	_, svc, wfID := setupAgentDefTestEnv(t, nil)
	overlap := &model.PathOverlap{
		Left:  []string{"be_files_to_modify", "be_files_to_create"},
		Right: []string{"fe_files_to_modify", "fe_files_to_create"},
	}
	steps := []model.StepDefinition{stepWithOverlap(overlap)}
	def, err := svc.CreateAgentDef("proj1", wfID, &types.AgentDefCreateRequest{
		ID:         "overlap-ok",
		Prompt:     "do work",
		PromptMode: PromptModeStepwise,
		Steps:      &steps,
	})
	if err != nil {
		t.Fatalf("CreateAgentDef: %v", err)
	}
	if def.Steps == nil {
		t.Fatal("Steps = nil, want canonical JSON")
	}
	if !strings.Contains(*def.Steps, "path_overlap") || !strings.Contains(*def.Steps, "be_files_to_modify") {
		t.Errorf("Steps = %q, want it to contain the path_overlap gate", *def.Steps)
	}
}
