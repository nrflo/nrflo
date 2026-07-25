package anthropic

import (
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

// contextSuffix1M is the Claude Code "[1m]" marker appended to a model id to
// select the 1M-token context window. It is a CLI convention, not an API model
// id: the bracket form 404s on /v1/messages, so it must be stripped before a
// request and is used only to pick the context window.
const contextSuffix1M = "[1m]"

// stripContextSuffix removes a trailing "[1m]" marker, returning the bare model
// id the API accepts. No-op for ids that don't carry the suffix.
func stripContextSuffix(model string) string {
	return strings.TrimSuffix(model, contextSuffix1M)
}

// isAdaptiveMillionModel reports whether the bare model id is both
// 1M-context-native and adaptive-thinking capable. The long-term fix is to
// query the Models API for capabilities instead of hardcoding ids here.
func isAdaptiveMillionModel(base string) bool {
	switch base {
	case "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8",
		"claude-opus-5", "claude-sonnet-5", "claude-fable-5", "claude-mythos-5":
		return true
	}
	return false
}

// contextWindow returns the input context window in tokens for a model id. A
// "[1m]" suffix or an adaptive 1M family yields 1M; everything else defaults
// to 200k.
func contextWindow(model string) int {
	if strings.HasSuffix(model, contextSuffix1M) {
		return 1_000_000
	}
	if isAdaptiveMillionModel(model) {
		return 1_000_000
	}
	return 200_000
}

// effortParam maps a nrflo reasoning-effort string onto the SDK output-config
// effort enum. Unknown non-empty values fall back to medium, matching the
// thinkingBudget default for the budget path.
func effortParam(effort string) sdk.OutputConfigEffort {
	switch effort {
	case "low":
		return sdk.OutputConfigEffortLow
	case "medium":
		return sdk.OutputConfigEffortMedium
	case "high":
		return sdk.OutputConfigEffortHigh
	case "xhigh":
		return sdk.OutputConfigEffortXhigh
	case "max":
		return sdk.OutputConfigEffortMax
	default:
		return sdk.OutputConfigEffortMedium
	}
}
