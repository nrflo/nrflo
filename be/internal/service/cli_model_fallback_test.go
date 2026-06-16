package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

func TestNormalizeFallbackModels(t *testing.T) {
	cases := []struct {
		name    string
		cliType string
		raw     string
		want    string
		wantErr string
	}{
		{"empty", "claude", "", "", ""},
		{"single", "claude", "claude-opus-4-7", "claude-opus-4-7", ""},
		{"trims and drops empties", "claude", " a , , b ", "a,b", ""},
		{"three ok", "claude", "a,b,c", "a,b,c", ""},
		{"four rejected", "claude", "a,b,c,d", "", "at most 3"},
		{"non-claude rejected", "codex", "a", "", "only supported for claude"},
		{"non-claude empty ok", "codex", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeFallbackModels(tc.cliType, tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCLIModel_CreateWithFallback(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	m, err := svc.Create(types.CLIModelCreateRequest{
		ID:             "fb-model",
		CLIType:        "claude",
		DisplayName:    "FB",
		MappedModel:    "claude-opus-4-8",
		FallbackModels: " claude-opus-4-7 , claude-sonnet-4-6 ",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.FallbackModels != "claude-opus-4-7,claude-sonnet-4-6" {
		t.Errorf("FallbackModels = %q, want normalized chain", m.FallbackModels)
	}
	got, err := svc.Get("fb-model")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FallbackModels != "claude-opus-4-7,claude-sonnet-4-6" {
		t.Errorf("persisted FallbackModels = %q", got.FallbackModels)
	}
}

func TestCLIModel_CreateFallbackOnCodex_Rejected(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	_, err := svc.Create(types.CLIModelCreateRequest{
		ID:             "codex-fb",
		CLIType:        "codex",
		DisplayName:    "Codex FB",
		MappedModel:    "gpt-5.3-codex",
		FallbackModels: "gpt-4",
	})
	if err == nil || !strings.Contains(err.Error(), "only supported for claude") {
		t.Fatalf("want claude-only error, got %v", err)
	}
}

// Fallback IS editable on built-in (read_only) rows, like reasoning_effort.
func TestCLIModel_UpdateReadonly_FallbackModels_Allowed(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	fb := "claude-opus-4-7,claude-sonnet-4-6"
	updated, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		FallbackModels: &fb,
	})
	if err != nil {
		t.Fatalf("Update fallback on read_only row: %v", err)
	}
	if updated.FallbackModels != fb {
		t.Errorf("FallbackModels = %q, want %q", updated.FallbackModels, fb)
	}
	if !updated.ReadOnly {
		t.Error("ReadOnly flag cleared by fallback update")
	}
}

func TestCLIModel_UpdateFallback_TooMany_Rejected(t *testing.T) {
	t.Parallel()
	svc, cleanup := setupCLIModelTestEnv(t)
	defer cleanup()

	fb := "a,b,c,d"
	_, err := svc.Update("opus_4_7", types.CLIModelUpdateRequest{
		FallbackModels: &fb,
	})
	if err == nil || !strings.Contains(err.Error(), "at most 3") {
		t.Fatalf("want at-most-3 error, got %v", err)
	}
}
