package service

import (
	"fmt"
	"strings"
)

var validReasoningEfforts = map[string]bool{
	"":       true,
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
	"ultra":  true,
}

// ValidateReasoningEffort checks that effort is one of the allowed levels and
// enforces the per-model restrictions: "xhigh" only with Claude Opus 4.7/4.8
// or Sonnet 5, "ultra" only with codex GPT-5.6 Sol/Terra. Exported so the
// spawner can re-validate a def-level override against the model row at
// spawn time, reusing the same gating rules instead of duplicating them.
func ValidateReasoningEffort(cliType, mappedModel, effort string) error {
	if !validReasoningEfforts[effort] {
		return fmt.Errorf("invalid reasoning_effort %q: must be one of low, medium, high, xhigh, max, ultra", effort)
	}
	if effort == "xhigh" && cliType == "claude" && !supportsXHighEffort(mappedModel) {
		return fmt.Errorf("reasoning_effort 'xhigh' is only supported on Opus 4.7/4.8 or Sonnet 5 Claude models")
	}
	if effort == "ultra" && (cliType != "codex" || !supportsUltraEffort(mappedModel)) {
		return fmt.Errorf("reasoning_effort 'ultra' is only supported on Codex GPT-5.6 Sol/Terra models")
	}
	return nil
}

// ValidateAPIReasoningEffort checks that effort is one of the allowed levels and
// enforces that "xhigh" is only used with Anthropic Opus 4.7/4.8 or Sonnet 5
// models. "ultra" is a codex-CLI-only effort and is rejected for API models.
// Exported for the same spawn-time re-validation reason as ValidateReasoningEffort.
func ValidateAPIReasoningEffort(provider, mappedModel, effort string) error {
	if !validReasoningEfforts[effort] {
		return fmt.Errorf("invalid reasoning_effort %q: must be one of low, medium, high, xhigh, max", effort)
	}
	if effort == "xhigh" && (provider != "anthropic" || !supportsXHighEffort(mappedModel)) {
		return fmt.Errorf("reasoning_effort 'xhigh' is only supported on Anthropic Opus 4.7/4.8 or Sonnet 5 models")
	}
	if effort == "ultra" {
		return fmt.Errorf("reasoning_effort 'ultra' is not supported for API models")
	}
	return nil
}

// supportsXHighEffort reports whether mappedModel supports the "xhigh"
// reasoning effort (Opus 4.7, Opus 4.8, and Sonnet 5). Shared by the CLI and
// API model reasoning-effort validators.
func supportsXHighEffort(mappedModel string) bool {
	return strings.HasPrefix(mappedModel, "claude-opus-4-7") ||
		strings.HasPrefix(mappedModel, "claude-opus-4-8") ||
		strings.HasPrefix(mappedModel, "claude-sonnet-5")
}

// supportsUltraEffort reports whether mappedModel supports the "ultra"
// reasoning effort (codex GPT-5.6 Sol/Terra only; Luna's catalog tops out at
// "max", and pre-5.6 models 400 on it at the provider).
func supportsUltraEffort(mappedModel string) bool {
	return strings.HasPrefix(mappedModel, "gpt-5.6-sol") ||
		strings.HasPrefix(mappedModel, "gpt-5.6-terra")
}
