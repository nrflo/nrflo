package spawner

import (
	"strings"
	"testing"
)

// ── ClaudeAdapter.PrepareUserSession ────────────────────────────────────────

// TestClaudeAdapter_PrepareUserSession_Interactive verifies the argv built for
// a non-plan-mode human session: --dangerously-skip-permissions is present and
// --permission-mode is absent.
func TestClaudeAdapter_PrepareUserSession_Interactive(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	launch, cleanup, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID:  "sess-int",
		Model:      "claude-sonnet",
		WorkDir:    "/tmp/proj",
		PromptFile: "/tmp/prompt.md",
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("PrepareUserSession() returned nil cleanup")
	}
	cleanup() // must not panic

	if launch.Command != "claude" {
		t.Errorf("launch.Command = %q, want claude", launch.Command)
	}
	if launch.Dir != "/tmp/proj" {
		t.Errorf("launch.Dir = %q, want /tmp/proj", launch.Dir)
	}
	if findArgElement(launch.Args, "--dangerously-skip-permissions") == -1 {
		t.Errorf("interactive mode should emit --dangerously-skip-permissions: %v", launch.Args)
	}
	if findArgElement(launch.Args, "--permission-mode") != -1 {
		t.Errorf("interactive mode should not emit --permission-mode: %v", launch.Args)
	}
	sidPos := findArgElement(launch.Args, "--session-id")
	if sidPos == -1 || launch.Args[sidPos+1] != "sess-int" {
		t.Errorf("--session-id sess-int missing: %v", launch.Args)
	}
	modelPos := findArgElement(launch.Args, "--model")
	if modelPos == -1 || launch.Args[modelPos+1] != "claude-sonnet" {
		t.Errorf("--model claude-sonnet missing: %v", launch.Args)
	}
	appendPos := findArgElement(launch.Args, "--append-system-prompt-file")
	if appendPos == -1 || launch.Args[appendPos+1] != "/tmp/prompt.md" {
		t.Errorf("--append-system-prompt-file /tmp/prompt.md missing: %v", launch.Args)
	}
}

// TestClaudeAdapter_PrepareUserSession_PlanMode verifies plan mode emits
// --permission-mode plan and --disallowed-tools ExitPlanMode, and never
// --dangerously-skip-permissions (which would override plan mode).
func TestClaudeAdapter_PrepareUserSession_PlanMode(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	launch, _, err := adapter.PrepareUserSession(UserSessionOptions{
		SessionID:  "sess-plan",
		Model:      "claude-opus",
		WorkDir:    "/tmp/proj",
		PromptFile: "/tmp/prompt.md",
		PlanMode:   true,
	})
	if err != nil {
		t.Fatalf("PrepareUserSession() error: %v", err)
	}

	argsStr := strings.Join(launch.Args, " ")
	if !strings.Contains(argsStr, "--permission-mode plan") {
		t.Errorf("plan mode missing --permission-mode plan: %v", launch.Args)
	}
	if !strings.Contains(argsStr, "--disallowed-tools ExitPlanMode") {
		t.Errorf("plan mode missing --disallowed-tools ExitPlanMode: %v", launch.Args)
	}
	if findArgElement(launch.Args, "--dangerously-skip-permissions") != -1 {
		t.Errorf("plan mode must not emit --dangerously-skip-permissions: %v", launch.Args)
	}
}

// TestClaudeAdapter_PrepareUserSession_OptionalFlags verifies effort, fallback
// model, system-prompt override, and settings JSON are emitted only when set,
// and that the override flag precedes the append flag.
func TestClaudeAdapter_PrepareUserSession_OptionalFlags(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	t.Run("all set", func(t *testing.T) {
		launch, _, err := adapter.PrepareUserSession(UserSessionOptions{
			SessionID:                "sess-opt",
			Model:                    "claude-sonnet",
			ReasoningEffort:          "high",
			FallbackModels:           "claude-a,claude-b",
			WorkDir:                  "/tmp/proj",
			PromptFile:               "/tmp/prompt.md",
			SystemPromptOverrideFile: "/tmp/override.md",
			SettingsJSON:             `{"hooks":{}}`,
		})
		if err != nil {
			t.Fatalf("PrepareUserSession() error: %v", err)
		}
		argsStr := strings.Join(launch.Args, " ")
		for _, want := range []string{"--effort high", "--fallback-model claude-a,claude-b", "--settings"} {
			if !strings.Contains(argsStr, want) {
				t.Errorf("args missing %q: %v", want, launch.Args)
			}
		}
		overrideIdx := findArgElement(launch.Args, "--system-prompt-file")
		appendIdx := findArgElement(launch.Args, "--append-system-prompt-file")
		if overrideIdx == -1 || appendIdx == -1 || overrideIdx >= appendIdx {
			t.Errorf("--system-prompt-file must precede --append-system-prompt-file: %v", launch.Args)
		}
	})

	t.Run("none set", func(t *testing.T) {
		launch, _, err := adapter.PrepareUserSession(UserSessionOptions{
			SessionID:  "sess-noopt",
			Model:      "claude-sonnet",
			WorkDir:    "/tmp/proj",
			PromptFile: "/tmp/prompt.md",
		})
		if err != nil {
			t.Fatalf("PrepareUserSession() error: %v", err)
		}
		for _, absent := range []string{"--effort", "--fallback-model", "--system-prompt-file", "--settings"} {
			if findArgElement(launch.Args, absent) != -1 {
				t.Errorf("args should not contain %q: %v", absent, launch.Args)
			}
		}
	})
}

// TestClaudeAdapter_PlanPromptSuffix_IsEmpty verifies Claude relies on its
// native plan store and never appends prompt text.
func TestClaudeAdapter_PlanPromptSuffix_IsEmpty(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}
	if got := adapter.PlanPromptSuffix(PlanCaptureOptions{SessionID: "s", PlanFile: "/tmp/plan.md"}); got != "" {
		t.Errorf("PlanPromptSuffix() = %q, want empty string", got)
	}
}
