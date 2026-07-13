package spawner

import (
	"context"
	"os"

	"be/internal/logger"
)

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
