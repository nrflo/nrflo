package spawner

import (
	"fmt"
	"os/exec"
	"strings"
)

// CLIAdapter defines the interface for different CLI backends
type CLIAdapter interface {
	// Name returns the CLI identifier (e.g., "claude", "opencode")
	Name() string

	// BuildCommand creates the exec.Cmd for spawning an agent
	BuildCommand(opts SpawnOptions) *exec.Cmd

	// MapModel converts a short model name to the CLI's expected format
	MapModel(model string) string

	// SupportsSessionID returns true if the CLI supports custom session IDs
	SupportsSessionID() bool

	// SupportsMaxTurns returns true if the CLI supports max turns limit
	SupportsMaxTurns() bool

	// SupportsSystemPromptFile returns true if the CLI supports --append-system-prompt-file
	SupportsSystemPromptFile() bool
}

// SpawnOptions contains parameters for building a spawn command
type SpawnOptions struct {
	Model         string
	MaxTurns      int
	SessionID     string
	PromptFile    string // Path to system prompt file
	Prompt        string // Full prompt content (for CLIs without file support)
	InitialPrompt string
	WorkDir       string
	Env           []string
}

// GetCLIAdapter returns the appropriate adapter for a CLI name
func GetCLIAdapter(name string) (CLIAdapter, error) {
	switch name {
	case "claude":
		return &ClaudeAdapter{}, nil
	case "opencode":
		return &OpencodeAdapter{}, nil
	case "codex":
		return &CodexAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown CLI: %s", name)
	}
}

// =============================================================================
// Claude CLI Adapter
// =============================================================================

// ClaudeAdapter implements CLIAdapter for Claude Code CLI
type ClaudeAdapter struct{}

func (a *ClaudeAdapter) Name() string {
	return "claude"
}

func (a *ClaudeAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	args := []string{
		"--print",
		"--verbose",
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--model", opts.Model,
		"--max-turns", fmt.Sprintf("%d", opts.MaxTurns),
		"--session-id", opts.SessionID,
		"--append-system-prompt-file", opts.PromptFile,
		opts.InitialPrompt,
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *ClaudeAdapter) MapModel(model string) string {
	// Claude CLI uses short names directly: opus, sonnet, haiku
	return model
}

func (a *ClaudeAdapter) SupportsSessionID() bool {
	return true
}

func (a *ClaudeAdapter) SupportsMaxTurns() bool {
	return true
}

func (a *ClaudeAdapter) SupportsSystemPromptFile() bool {
	return true
}

// =============================================================================
// Opencode CLI Adapter
// =============================================================================

// OpencodeAdapter implements CLIAdapter for Opencode CLI
type OpencodeAdapter struct{}

func (a *OpencodeAdapter) Name() string {
	return "opencode"
}

func (a *OpencodeAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	// Opencode uses provider/model format
	model := a.MapModel(opts.Model)
	reasoningEffort := a.GetReasoningEffort(opts.Model)

	// Opencode doesn't support --append-system-prompt-file, so we pass full prompt as message
	fullPrompt := opts.Prompt + "\n\n" + opts.InitialPrompt

	args := []string{
		"run",
		"--format", "json",
		"--model", model,
	}

	// Add reasoning effort variant if specified
	if reasoningEffort != "" {
		args = append(args, "--variant", reasoningEffort)
	}

	args = append(args, fullPrompt)

	cmd := exec.Command("opencode", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *OpencodeAdapter) MapModel(model string) string {
	// If already in provider/model format, return as-is
	if strings.Contains(model, "/") {
		return model
	}

	// Map short names to full opencode model names
	// For GPT models with reasoning suffixes, map to openai/o3
	modelMap := map[string]string{
		"opus":       "anthropic/claude-opus-4-5",
		"sonnet":     "anthropic/claude-sonnet-4-5",
		"haiku":      "anthropic/claude-haiku-4-5",
		"gpt_max":    "openai/gpt-5.2-codex",
		"gpt_high":   "openai/gpt-5.2-codex",
		"gpt_medium": "openai/gpt-5.2-codex",
		"gpt_low":    "openai/gpt-5.2-codex",
	}

	if mapped, ok := modelMap[model]; ok {
		return mapped
	}

	// Default: assume anthropic provider
	return "anthropic/" + model
}

// GetReasoningEffort returns the reasoning effort variant for a model alias
// Opencode uses --variant flag with values: max, high, medium, low, minimal
func (a *OpencodeAdapter) GetReasoningEffort(model string) string {
	switch model {
	case "gpt_max":
		return "max"
	case "gpt_high":
		return "high"
	case "gpt_medium":
		return "medium"
	case "gpt_low":
		return "low"
	default:
		// No variant for Anthropic models or unknown models
		return ""
	}
}

func (a *OpencodeAdapter) SupportsSessionID() bool {
	return false // Opencode generates its own session IDs
}

func (a *OpencodeAdapter) SupportsMaxTurns() bool {
	return false // Opencode runs until completion
}

func (a *OpencodeAdapter) SupportsSystemPromptFile() bool {
	return false // Must pass prompt inline
}

// =============================================================================
// Codex CLI Adapter
// =============================================================================

// CodexAdapter implements CLIAdapter for OpenAI Codex CLI
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	model := a.MapModel(opts.Model)
	reasoningEffort := a.GetReasoningEffort(opts.Model)

	// Codex doesn't support --append-system-prompt-file, so we pass full prompt as message
	fullPrompt := opts.Prompt + "\n\n" + opts.InitialPrompt

	args := []string{
		"exec",
		"--json",
		"--full-auto",
		"--sandbox", "danger-full-access",
		"--skip-git-repo-check",
		"--model", model,
		"-c", fmt.Sprintf("model_reasoning_effort=%s", reasoningEffort),
		fullPrompt,
	}

	cmd := exec.Command("codex", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
}

func (a *CodexAdapter) MapModel(model string) string {
	// Map short names to gpt-5.2-codex with reasoning levels
	// Reasoning effort is handled separately in GetReasoningEffort
	modelMap := map[string]string{
		"opus":       "gpt-5.2-codex",
		"sonnet":     "gpt-5.2-codex",
		"haiku":      "gpt-5.2-codex",
		"gpt_xhigh":  "gpt-5.2-codex",
		"gpt_high":   "gpt-5.2-codex",
		"gpt_medium": "gpt-5.2-codex",
	}
	if mapped, ok := modelMap[model]; ok {
		return mapped
	}
	return model // pass through custom model names
}

// GetReasoningEffort returns the reasoning effort level for a model alias
func (a *CodexAdapter) GetReasoningEffort(model string) string {
	switch model {
	case "gpt_xhigh":
		return "xhigh"
	case "gpt_high", "opus":
		return "high"
	case "gpt_medium", "sonnet", "haiku":
		return "medium"
	default:
		return "medium" // default for custom models
	}
}

func (a *CodexAdapter) SupportsSessionID() bool {
	return false // Codex generates its own session IDs
}

func (a *CodexAdapter) SupportsMaxTurns() bool {
	return false // Codex runs until completion
}

func (a *CodexAdapter) SupportsSystemPromptFile() bool {
	return false // Must pass prompt inline
}
