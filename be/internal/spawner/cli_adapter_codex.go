package spawner

import (
	"fmt"
	"os/exec"
)

// CodexAdapter implements CLIAdapter for OpenAI Codex CLI
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	model := opts.MappedModel
	if model == "" {
		model = a.MapModel(opts.Model)
	}
	reasoningEffort := opts.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = a.GetReasoningEffort(opts.Model)
	}

	args := []string{
		"exec",
		"--json",
		"--model", model,
		"-c", fmt.Sprintf("model_reasoning_effort=\"%s\"", reasoningEffort),
		"--dangerously-bypass-approvals-and-sandbox",
	}

	// Prompt is piped via stdin (UsesStdinPrompt=true), no positional arg

	cmd := exec.Command("codex", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *CodexAdapter) MapModel(model string) string {
	modelMap := map[string]string{
		"codex_gpt_normal":   "gpt-5.3-codex",
		"codex_gpt_high":     "gpt-5.3-codex",
		"codex_gpt54_normal": "gpt-5.4",
		"codex_gpt54_high":   "gpt-5.4",
	}
	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model // pass through custom model names
}

// GetReasoningEffort returns the reasoning effort level for a model alias
func (a *CodexAdapter) GetReasoningEffort(model string) string {
	switch model {
	case "codex_gpt_normal", "codex_gpt_high":
		return "high"
	case "codex_gpt54_normal":
		return "medium"
	case "codex_gpt54_high":
		return "high"
	default:
		return "high"
	}
}

func (a *CodexAdapter) SupportsSessionID() bool {
	return false // Codex generates its own session IDs
}

func (a *CodexAdapter) SupportsSystemPromptFile() bool {
	return false // Prompt piped via stdin
}

func (a *CodexAdapter) SupportsResume() bool {
	return false
}

func (a *CodexAdapter) UsesStdinPrompt() bool {
	return true
}

func (a *CodexAdapter) SupportsInteractive() bool { return true }

func (a *CodexAdapter) BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd {
	args := []string{
		"--model", opts.Model,
		"--dangerously-bypass-approvals-and-sandbox",
	}
	cmd := exec.Command("codex", args...)
	cmd.Dir = opts.WorkDir
	if opts.CodexHome != "" {
		cmd.Env = append(opts.Env, "CODEX_HOME="+opts.CodexHome)
	} else {
		cmd.Env = opts.Env
	}
	return cmd
}

func (a *CodexAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}
