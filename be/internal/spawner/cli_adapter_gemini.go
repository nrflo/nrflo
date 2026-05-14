package spawner

import (
	"context"
	"os/exec"
	"time"
)

// GeminiAdapter implements CLIAdapter for Google Gemini CLI.
type GeminiAdapter struct{}

func (a *GeminiAdapter) Name() string { return "gemini" }

func (a *GeminiAdapter) MapModel(model string) string {
	modelMap := map[string]string{
		"gemini_pro":        "gemini-2.5-pro",
		"gemini_flash":      "gemini-2.5-flash",
		"gemini_flash_lite": "gemini-2.5-flash-lite",
	}
	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model // pass through custom model names
}

func (a *GeminiAdapter) SupportsSessionID() bool       { return true }
func (a *GeminiAdapter) SupportsSystemPromptFile() bool { return false }
func (a *GeminiAdapter) SupportsResume() bool           { return true }

func (a *GeminiAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--skip-trust",
		"-y",
		"-m", opts.Model,
		"--session-id", opts.SessionID,
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	cmd := exec.Command("gemini", args...)
	cmd.Dir = opts.WorkDir
	env := opts.Env
	env = removeEnvKey(env, "HOME=")
	env = removeEnvKey(env, "GEMINI_HOME=")
	env = removeEnvKey(env, "XDG_CONFIG_HOME=")
	if opts.GeminiHome != "" {
		env = append(env, "HOME="+opts.GeminiHome)
	}
	if !envHasKey(env, "TERM=") {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env
	return cmd
}

// PrepareInteractive creates a per-session HOME dir with .gemini/ auth symlinks
// and a settings.json with hooks for all Gemini hook events.
func (a *GeminiAdapter) PrepareInteractive(opts InteractivePrepOptions) (InteractiveExtras, func(), error) {
	dir, cleanup, err := prepareGeminiHome(opts)
	if err != nil {
		return InteractiveExtras{}, func() {}, err
	}
	return InteractiveExtras{GeminiHome: dir}, cleanup, nil
}

func (a *GeminiAdapter) DeliversPromptInline() bool      { return true }
func (a *GeminiAdapter) NeedsTerminalQueryReplies() bool { return false }
func (a *GeminiAdapter) BumpsOnPTYBytes() bool           { return false }
func (a *GeminiAdapter) NaturalExitGrace() time.Duration { return 2 * time.Second }

// PostStart is a noop stub. JSONL tailer implementation is tracked separately.
func (a *GeminiAdapter) PostStart(_ context.Context, _ PostStartOptions) (func(), error) {
	return func() {}, nil
}
