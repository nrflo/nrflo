package spawner

// UserSessionOptions carries the parameters for a human-driven PTY session the
// orchestrator owns (agent_sessions.status = user_interactive) — the
// orchestrator's interactive/plan pre-step and the spawner's take-control
// resume. Distinct from a managed cli_interactive spawn (InteractiveSpawnOptions):
// Model is already mapped (DB row or adapter.MapModel by the caller).
type UserSessionOptions struct {
	SessionID                string
	Model                    string // already mapped (DB row or adapter.MapModel), ready for the CLI's --model flag
	ReasoningEffort          string
	FallbackModels           string // Claude only; ignored by non-Claude adapters
	WorkDir                  string
	Prompt                   string // rendered prompt body; delivered per-adapter (PTY stdin write vs argv positional)
	PromptFile               string // path to the temp file holding Prompt, for adapters that read prompts from disk
	SystemPromptOverrideFile string // Claude: --system-prompt-file; others ignored
	SettingsJSON             string // Claude: --settings JSON; others ignored
	PlanMode                 bool
	PlanFile                 string // adapter-agnostic path a plan-mode session should write its final plan to
}

// PlanCaptureOptions carries the parameters ReadPlan/PlanPromptSuffix need to
// locate a plan-mode session's output.
type PlanCaptureOptions struct {
	SessionID string
	WorkDir   string // the directory the PTY actually ran in (worktree, not project root)
	PlanFile  string
}
