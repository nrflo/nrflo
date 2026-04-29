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

	// SupportsInteractive returns true if the CLI supports PTY-based interactive execution
	// without batch flags (--print, --verbose, --output-format, etc.).
	SupportsInteractive() bool

	// BuildInteractiveCommand creates the exec.Cmd for interactive PTY execution.
	// Unlike BuildCommand, it omits all batch/output-format flags so the CLI
	// runs in its normal interactive terminal UI mode.
	BuildInteractiveCommand(opts InteractiveSpawnOptions) *exec.Cmd
}

// InteractiveSpawnOptions contains parameters for building an interactive PTY command.
// PromptFilePath is intentionally absent — the prompt is delivered via PTY stdin Write.
type InteractiveSpawnOptions struct {
	SessionID        string
	Model            string
	ReasoningEffort  string // passed as --effort (Claude) or --variant (Opencode)
	WorkDir          string
	Env              []string
	SystemPromptFile string // path to suffix file; Claude: --append-system-prompt-file; others: ignored
	SettingsJSON     string // Claude: --settings JSON; others: ignored
}

// ResumeOptions contains parameters for resuming a CLI session
type ResumeOptions struct {
	SessionID        string
	Prompt           string
	WorkDir          string
	Env              []string
	SettingsJSON     string // Claude --settings JSON (ignored by non-Claude adapters)
	ReasoningEffort  string // Claude --effort level (ignored by non-Claude adapters)
	SystemPromptFile string // Path to system prompt suffix file (--append-system-prompt-file)
}

// SpawnOptions contains parameters for building a spawn command
type SpawnOptions struct {
	Model            string
	SessionID        string
	PromptFile       string // Path to system prompt file
	Prompt           string // Full prompt content (for CLIs without file support)
	InitialPrompt    string
	WorkDir          string
	Env              []string
	MappedModel      string // DB-sourced mapped model name; if set, adapters skip their own MapModel()
	ReasoningEffort  string // DB-sourced reasoning effort; if set, adapters skip their own GetReasoningEffort()
	SettingsJSON     string // Claude --settings JSON (ignored by non-Claude adapters)
	SystemPromptFile string // Path to system prompt suffix file (--append-system-prompt-file; Claude only)
}

// DefaultCLIForModel returns the appropriate CLI name for a model.
// opencode_* → opencode, codex_gpt* → codex, everything else → claude.
func DefaultCLIForModel(model string) string {
	if strings.HasPrefix(model, "opencode_") {
		return "opencode"
	}
	if strings.HasPrefix(model, "codex_gpt") {
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
