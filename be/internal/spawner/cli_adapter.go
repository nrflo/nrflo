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

	// SupportsSystemPromptFile returns true if the CLI supports --append-system-prompt-file
	SupportsSystemPromptFile() bool

	// SupportsResume returns true if the CLI supports resuming a session
	SupportsResume() bool

	// UsesStdinPrompt returns true if the CLI reads the prompt from stdin
	// instead of a positional argument (e.g., opencode run < prompt.txt)
	UsesStdinPrompt() bool

	// BuildResumeCommand creates the exec.Cmd for resuming a session with a prompt
	BuildResumeCommand(opts ResumeOptions) *exec.Cmd
}

// ResumeOptions contains parameters for resuming a CLI session
type ResumeOptions struct {
	SessionID string
	Prompt    string
	WorkDir   string
	Env       []string
}

// SpawnOptions contains parameters for building a spawn command
type SpawnOptions struct {
	Model         string
	SessionID     string
	PromptFile    string // Path to system prompt file
	Prompt        string // Full prompt content (for CLIs without file support)
	InitialPrompt string
	WorkDir       string
	Env           []string
}

// DefaultCLIForModel returns the appropriate CLI name for a model.
// opencode_gpt_* → opencode, codex_gpt_* → codex, everything else → claude.
func DefaultCLIForModel(model string) string {
	if strings.HasPrefix(model, "opencode_gpt_") {
		return "opencode"
	}
	if strings.HasPrefix(model, "codex_gpt_") {
		return "codex"
	}
	return "claude"
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

func (a *ClaudeAdapter) SupportsSystemPromptFile() bool {
	return true
}

func (a *ClaudeAdapter) SupportsResume() bool {
	return true
}

func (a *ClaudeAdapter) UsesStdinPrompt() bool {
	return false
}

func (a *ClaudeAdapter) BuildResumeCommand(opts ResumeOptions) *exec.Cmd {
	args := []string{
		"--resume", opts.SessionID,
		"--print",
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format", "stream-json",
		opts.Prompt,
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	return cmd
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

	args := []string{
		"run",
		"--format", "json",
		"--model", model,
	}

	// Add reasoning effort variant if specified
	if reasoningEffort != "" {
		args = append(args, "--variant", reasoningEffort)
	}

	// Prompt is piped via stdin (UsesStdinPrompt=true), no positional arg

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

	modelMap := map[string]string{
		"opencode_gpt_normal": "openai/gpt-5.3-codex",
		"opencode_gpt_high":   "openai/gpt-5.3-codex",
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
	case "opencode_gpt_normal", "opencode_gpt_high":
		return "high"
	default:
		return ""
	}
}

func (a *OpencodeAdapter) SupportsSessionID() bool {
	return false // Opencode generates its own session IDs
}

func (a *OpencodeAdapter) SupportsSystemPromptFile() bool {
	return false // Prompt piped via stdin
}

func (a *OpencodeAdapter) SupportsResume() bool {
	return false
}

func (a *OpencodeAdapter) UsesStdinPrompt() bool {
	return true
}

func (a *OpencodeAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
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
	modelMap := map[string]string{
		"codex_gpt_normal": "gpt-5.3-codex",
		"codex_gpt_high":   "gpt-5.3-codex",
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
	default:
		return "high"
	}
}

func (a *CodexAdapter) SupportsSessionID() bool {
	return false // Codex generates its own session IDs
}

func (a *CodexAdapter) SupportsSystemPromptFile() bool {
	return false // Must pass prompt inline
}

func (a *CodexAdapter) SupportsResume() bool {
	return false
}

func (a *CodexAdapter) UsesStdinPrompt() bool {
	return false
}

func (a *CodexAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd {
	return nil
}
