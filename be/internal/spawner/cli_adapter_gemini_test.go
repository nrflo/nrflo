package spawner

import (
	"testing"
	"time"
)

func TestGeminiAdapterMapModel(t *testing.T) {
	t.Parallel()
	a := &GeminiAdapter{}
	tests := []struct {
		input string
		want  string
	}{
		{"gemini_pro", "gemini-2.5-pro"},
		{"gemini_flash", "gemini-2.5-flash"},
		{"gemini_flash_lite", "gemini-2.5-flash-lite"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
		{"custom-model", "custom-model"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := a.MapModel(tt.input)
			if got != tt.want {
				t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeminiAdapterMethodValues(t *testing.T) {
	t.Parallel()
	a := &GeminiAdapter{}
	if !a.SupportsSessionID() {
		t.Error("SupportsSessionID() = false, want true")
	}
	if a.SupportsSystemPromptFile() {
		t.Error("SupportsSystemPromptFile() = true, want false")
	}
	if !a.SupportsResume() {
		t.Error("SupportsResume() = false, want true")
	}
	if !a.DeliversPromptInline() {
		t.Error("DeliversPromptInline() = false, want true")
	}
	if a.NeedsTerminalQueryReplies() {
		t.Error("NeedsTerminalQueryReplies() = true, want false")
	}
	if a.BumpsOnPTYBytes() {
		t.Error("BumpsOnPTYBytes() = true, want false")
	}
	if got, want := a.NaturalExitGrace(), 2*time.Second; got != want {
		t.Errorf("NaturalExitGrace() = %v, want %v", got, want)
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_NoResumeNoPrompt verifies baseline argv order.
func TestGeminiAdapter_BuildInteractiveCommand_NoResumeNoPrompt(t *testing.T) {
	t.Parallel()
	cmd := (&GeminiAdapter{}).BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID: "sess-1",
		Model:     "gemini-2.5-pro",
		WorkDir:   "/tmp/wd",
	})
	if cmd == nil {
		t.Fatal("BuildInteractiveCommand returned nil")
	}
	want := []string{"gemini", "--skip-trust", "-y", "-m", "gemini-2.5-pro", "--session-id", "sess-1"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v (len=%d), want %v (len=%d)", cmd.Args, len(cmd.Args), want, len(want))
	}
	for i, w := range want {
		if cmd.Args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], w)
		}
	}
	if cmd.Dir != "/tmp/wd" {
		t.Errorf("cmd.Dir = %q, want /tmp/wd", cmd.Dir)
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_ResumeOnly verifies --resume is appended after session-id.
func TestGeminiAdapter_BuildInteractiveCommand_ResumeOnly(t *testing.T) {
	t.Parallel()
	cmd := (&GeminiAdapter{}).BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID:       "sess-2",
		Model:           "gemini-2.5-flash",
		WorkDir:         "/tmp/wd",
		ResumeSessionID: "prev-sess",
	})
	want := []string{"gemini", "--skip-trust", "-y", "-m", "gemini-2.5-flash", "--session-id", "sess-2", "--resume", "prev-sess"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v (len=%d), want %v (len=%d)", cmd.Args, len(cmd.Args), want, len(want))
	}
	for i, w := range want {
		if cmd.Args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], w)
		}
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_PromptOnly verifies prompt is the last positional.
func TestGeminiAdapter_BuildInteractiveCommand_PromptOnly(t *testing.T) {
	t.Parallel()
	cmd := (&GeminiAdapter{}).BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID: "sess-3",
		Model:     "gemini-2.5-flash-lite",
		WorkDir:   "/tmp/wd",
		Prompt:    "do the thing",
	})
	want := []string{"gemini", "--skip-trust", "-y", "-m", "gemini-2.5-flash-lite", "--session-id", "sess-3", "do the thing"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v (len=%d), want %v (len=%d)", cmd.Args, len(cmd.Args), want, len(want))
	}
	for i, w := range want {
		if cmd.Args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], w)
		}
	}
	if got := cmd.Args[len(cmd.Args)-1]; got != "do the thing" {
		t.Errorf("last arg = %q, want prompt", got)
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_ResumeAndPrompt verifies --resume precedes prompt.
func TestGeminiAdapter_BuildInteractiveCommand_ResumeAndPrompt(t *testing.T) {
	t.Parallel()
	cmd := (&GeminiAdapter{}).BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID:       "sess-4",
		Model:           "gemini-2.5-pro",
		WorkDir:         "/tmp/wd",
		ResumeSessionID: "old-sess",
		Prompt:          "continue this",
	})
	want := []string{"gemini", "--skip-trust", "-y", "-m", "gemini-2.5-pro", "--session-id", "sess-4", "--resume", "old-sess", "continue this"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v (len=%d), want %v (len=%d)", cmd.Args, len(cmd.Args), want, len(want))
	}
	for i, w := range want {
		if cmd.Args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], w)
		}
	}
	if got := cmd.Args[len(cmd.Args)-1]; got != "continue this" {
		t.Errorf("last arg = %q, want prompt", got)
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_EnvStrips verifies that HOME/GEMINI_HOME/XDG_CONFIG_HOME
// are removed, HOME=GeminiHome is injected, and TERM + PATH are preserved.
func TestGeminiAdapter_BuildInteractiveCommand_EnvStrips(t *testing.T) {
	t.Parallel()
	opts := InteractiveSpawnOptions{
		SessionID:  "sess-e",
		Model:      "gemini-2.5-pro",
		WorkDir:    "/tmp/wd",
		GeminiHome: "/tmp/ghome",
		Env:        []string{"HOME=/old", "GEMINI_HOME=/foo", "XDG_CONFIG_HOME=/bar", "TERM=screen", "PATH=/usr/bin"},
	}
	env := (&GeminiAdapter{}).BuildInteractiveCommand(opts).Env

	for _, bad := range []string{"HOME=/old", "GEMINI_HOME=/foo", "XDG_CONFIG_HOME=/bar"} {
		for _, e := range env {
			if e == bad {
				t.Errorf("env contains unwanted %q: %v", bad, env)
			}
		}
	}
	if !geminiTestEnvHas(env, "HOME=/tmp/ghome") {
		t.Errorf("env missing HOME=/tmp/ghome: %v", env)
	}
	if !geminiTestEnvHas(env, "TERM=screen") {
		t.Errorf("env missing TERM=screen (should be preserved when already set): %v", env)
	}
	if !geminiTestEnvHas(env, "PATH=/usr/bin") {
		t.Errorf("env missing PATH=/usr/bin: %v", env)
	}
}

// TestGeminiAdapter_BuildInteractiveCommand_EnvAddsTERM verifies TERM=xterm-256color
// is injected when absent from the inherited env.
func TestGeminiAdapter_BuildInteractiveCommand_EnvAddsTERM(t *testing.T) {
	t.Parallel()
	opts := InteractiveSpawnOptions{
		SessionID: "sess-term",
		Model:     "gemini-2.5-pro",
		WorkDir:   "/tmp/wd",
		Env:       []string{"PATH=/usr/bin"},
	}
	env := (&GeminiAdapter{}).BuildInteractiveCommand(opts).Env
	if !geminiTestEnvHas(env, "TERM=xterm-256color") {
		t.Errorf("env missing TERM=xterm-256color when TERM absent: %v", env)
	}
}

func TestGetCLIAdapter_GeminiReturnsGeminiAdapter(t *testing.T) {
	t.Parallel()
	adapter, err := GetCLIAdapter("gemini")
	if err != nil {
		t.Fatalf("GetCLIAdapter(\"gemini\") error: %v", err)
	}
	if _, ok := adapter.(*GeminiAdapter); !ok {
		t.Errorf("GetCLIAdapter(\"gemini\") returned %T, want *GeminiAdapter", adapter)
	}
}

func TestDefaultCLIForModel_GeminiModels(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gemini_pro", "gemini_flash", "gemini_flash_lite"} {
		model := model
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			got := DefaultCLIForModel(model)
			if got != "gemini" {
				t.Errorf("DefaultCLIForModel(%q) = %q, want %q", model, got, "gemini")
			}
		})
	}
}

func geminiTestEnvHas(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}
