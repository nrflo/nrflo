package spawner

import (
	"context"
	"os"

	"be/internal/logger"
	"be/internal/model"
)

// agentDefSystemTemplateID returns def.SystemTemplateID, or "" for a nil def
// (global workflows / defs not yet loaded).
func agentDefSystemTemplateID(def *model.AgentDefinition) string {
	if def == nil {
		return ""
	}
	return def.SystemTemplateID
}

// noSystemPromptFilePrefix builds the prompt-body prefix for adapters without
// --system-prompt-file support (Codex): override first, then suffix, so both
// are delivered via the prompt file instead of a dedicated flag. Returns ""
// (no-op) for adapters that do support the flag, or when both are empty.
func noSystemPromptFilePrefix(suffix, systemPromptOverride string, adapter CLIAdapter) string {
	if adapter.SupportsSystemPromptFile() {
		return ""
	}
	if systemPromptOverride == "" {
		return suffix
	}
	if suffix == "" {
		return systemPromptOverride
	}
	return systemPromptOverride + "\n\n" + suffix
}

// writeSuffixAndOverrideFiles writes the system-prompt-suffix and
// system-prompt-override injectables to temp files for adapters that support
// --append-system-prompt-file / --system-prompt-file (Claude). A write
// failure is logged and leaves the corresponding path empty rather than
// failing the spawn — these are prompt-shaping niceties, not required inputs.
func writeSuffixAndOverrideFiles(suffix, systemPromptOverride string, adapter CLIAdapter) (suffixFilePath, systemPromptOverrideFilePath string) {
	if suffix != "" && adapter.SupportsSystemPromptFile() {
		sf, sfErr := createScratchTemp("system-suffix-*.md")
		if sfErr != nil {
			logger.Warn(context.Background(), "failed to create suffix temp file", "error", sfErr)
		} else {
			if _, sfErr = sf.WriteString(suffix); sfErr != nil {
				sf.Close()
				os.Remove(sf.Name())
				logger.Warn(context.Background(), "failed to write suffix temp file", "error", sfErr)
			} else {
				sf.Close()
				suffixFilePath = sf.Name()
			}
		}
	}

	if systemPromptOverride != "" && adapter.SupportsSystemPromptFile() {
		of, ofErr := createScratchTemp("system-prompt-*.md")
		if ofErr != nil {
			logger.Warn(context.Background(), "failed to create system-prompt override temp file", "error", ofErr)
		} else {
			if _, ofErr = of.WriteString(systemPromptOverride); ofErr != nil {
				of.Close()
				os.Remove(of.Name())
				logger.Warn(context.Background(), "failed to write system-prompt override temp file", "error", ofErr)
			} else {
				of.Close()
				systemPromptOverrideFilePath = of.Name()
			}
		}
	}
	return suffixFilePath, systemPromptOverrideFilePath
}
