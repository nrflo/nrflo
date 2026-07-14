package spawner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCodexAdapter_PrepareUserSession_Interactive verifies argv/env for a
// non-plan-mode human session: --dangerously-bypass-approvals-and-sandbox is
// present, a CODEX_HOME profile dir is created, and cleanup removes it.
func TestCodexAdapter_PrepareUserSession_Interactive(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-codex-int",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
		Prompt:    "do the thing",
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("PrepareUserSession() returned nil cleanup")
	}

	if launch.Command != "codex" {
		t.Errorf("launch.Command = %q, want codex", launch.Command)
	}
	if launch.Dir != workDir {
		t.Errorf("launch.Dir = %q, want %q", launch.Dir, workDir)
	}
	if findArgElement(launch.Args, "--dangerously-bypass-approvals-and-sandbox") == -1 {
		t.Errorf("interactive mode should emit --dangerously-bypass-approvals-and-sandbox: %v", launch.Args)
	}
	if findArgElement(launch.Args, "--sandbox") != -1 {
		t.Errorf("interactive mode should not emit --sandbox: %v", launch.Args)
	}
	if launch.Args[len(launch.Args)-1] != "do the thing" {
		t.Errorf("prompt should be the trailing positional argv, got: %v", launch.Args)
	}

	var codexHome string
	for _, e := range launch.Env {
		if strings.HasPrefix(e, "CODEX_HOME=") {
			codexHome = strings.TrimPrefix(e, "CODEX_HOME=")
		}
	}
	if codexHome == "" {
		t.Fatalf("launch.Env missing CODEX_HOME: %v", launch.Env)
	}
	if _, err := os.Stat(codexHome); err != nil {
		t.Errorf("CODEX_HOME dir %q should exist before cleanup: %v", codexHome, err)
	}

	cleanup()
	if _, err := os.Stat(codexHome); !os.IsNotExist(err) {
		t.Errorf("cleanup() should remove CODEX_HOME dir %q, stat err = %v", codexHome, err)
	}
}

// TestCodexAdapter_PrepareUserSession_PlanMode verifies plan mode uses
// --sandbox read-only --ask-for-approval on-request instead of the bypass flag.
func TestCodexAdapter_PrepareUserSession_PlanMode(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-codex-plan",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
		PlanMode:  true,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	t.Cleanup(cleanup)

	argsStr := strings.Join(launch.Args, " ")
	if !strings.Contains(argsStr, "--sandbox read-only") {
		t.Errorf("plan mode missing --sandbox read-only: %v", launch.Args)
	}
	if !strings.Contains(argsStr, "--ask-for-approval on-request") {
		t.Errorf("plan mode missing --ask-for-approval on-request: %v", launch.Args)
	}
	if findArgElement(launch.Args, "--dangerously-bypass-approvals-and-sandbox") != -1 {
		t.Errorf("plan mode must not emit --dangerously-bypass-approvals-and-sandbox: %v", launch.Args)
	}
}

// TestCodexAdapter_PrepareUserSession_ReasoningEffort verifies effort rides as
// a -c model_reasoning_effort override, and is absent when unset.
func TestCodexAdapter_PrepareUserSession_ReasoningEffort(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	t.Run("set", func(t *testing.T) {
		workDir := t.TempDir()
		launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
			SessionID:       "sess-effort",
			Model:           "gpt-5-codex",
			WorkDir:         workDir,
			ReasoningEffort: "high",
		})
		if err != nil {
			t.Fatalf("PrepareUserSession() error: %v", err)
		}
		t.Cleanup(cleanup)
		if !strings.Contains(strings.Join(launch.Args, " "), `model_reasoning_effort="high"`) {
			t.Errorf("args missing model_reasoning_effort override: %v", launch.Args)
		}
	})

	t.Run("unset", func(t *testing.T) {
		workDir := t.TempDir()
		launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
			SessionID: "sess-noeffort",
			Model:     "gpt-5-codex",
			WorkDir:   workDir,
		})
		if err != nil {
			t.Fatalf("PrepareUserSession() error: %v", err)
		}
		t.Cleanup(cleanup)
		if strings.Contains(strings.Join(launch.Args, " "), "model_reasoning_effort") {
			t.Errorf("args should not contain model_reasoning_effort: %v", launch.Args)
		}
	})
}

// TestCodexAdapter_PrepareUserSession_EmptyPromptOmitsPositional verifies no
// trailing positional argv is appended when Prompt is empty.
func TestCodexAdapter_PrepareUserSession_EmptyPromptOmitsPositional(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	workDir := t.TempDir()

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-noprompt",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	t.Cleanup(cleanup)

	// check_for_update_on_startup=false is always present as -c value; the last
	// arg must not be a stray empty positional.
	if launch.Args[len(launch.Args)-1] == "" {
		t.Errorf("empty prompt should not append an empty positional: %v", launch.Args)
	}
}

// TestCodexAdapter_PrepareUserSession_MkdirTempFailure verifies an error is
// returned (not a launch with a broken profile) when the profile dir cannot
// be created.
func TestCodexAdapter_PrepareUserSession_MkdirTempFailure(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("TMPDIR", "/nonexistent-nrflo-user-session-test-xyz")
	adapter := &CodexAdapter{}

	_, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID: "sess-fail",
		Model:     "gpt-5-codex",
		WorkDir:   workDir,
	})
	if err == nil {
		t.Fatal("PrepareUserSession() should error when profile dir creation fails")
	}
	if cleanup == nil {
		t.Fatal("PrepareUserSession() should still return a non-nil (noop) cleanup on error")
	}
	cleanup() // must not panic
}

// TestCodexAdapter_PlanPromptSuffix_NamesPlanFile verifies the suffix tells
// the agent where to write its plan.
func TestCodexAdapter_PlanPromptSuffix_NamesPlanFile(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	suffix := adapter.PlanPromptSuffix(PlanCaptureOptions{SessionID: "s", PlanFile: "/tmp/my-plan.md"})
	if !strings.Contains(suffix, "/tmp/my-plan.md") {
		t.Errorf("PlanPromptSuffix() = %q, want to contain the plan file path", suffix)
	}
	if !strings.Contains(suffix, "Do not implement the plan") {
		t.Errorf("PlanPromptSuffix() = %q, want to instruct the agent not to implement", suffix)
	}
}

// ── ReadPlan ─────────────────────────────────────────────────────────────────

func TestCodexAdapter_ReadPlan_ReadsPlanFile(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	planFile := filepath.Join(t.TempDir(), "plan.md")
	content := "# Codex Plan\n\nStep 1"
	if err := os.WriteFile(planFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result := adapter.ReadPlan(PlanCaptureOptions{SessionID: "s", PlanFile: planFile})
	if result != content {
		t.Errorf("ReadPlan() = %q, want %q", result, content)
	}
}

func TestCodexAdapter_ReadPlan_MissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}
	result := adapter.ReadPlan(PlanCaptureOptions{SessionID: "s", PlanFile: filepath.Join(t.TempDir(), "missing.md")})
	if result != "" {
		t.Errorf("ReadPlan() = %q, want empty string for missing plan file", result)
	}
}
