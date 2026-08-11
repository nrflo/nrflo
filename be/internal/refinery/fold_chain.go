package refinery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// foldAttemptResult is one chain-entry attempt's outcome: either a landed
// fold (err == nil) or a failure classified as advance-eligible or not.
type foldAttemptResult struct {
	content  string
	usage    provider.Usage
	provName string
	modelID  string
	advance  bool // true => try the next chain entry; false => stop the walk
	err      error
}

// walkFoldChain runs def's resolved chain in order starting at position 0,
// advancing over a provider-build/credential/hard-transport failure
// (mirrors spawnEntryWithBuildFallback, be/internal/spawner/tier_fallback.go)
// and stopping (ok=false, no advance) on a structural error, rate-limit, or
// degenerate output. A weighted chain is reordered first
// (applyWeightedRotation) so a non-primary entry can lead. Every attempt
// writes its own refinery_runs row via recordFoldRun: chain_position=the
// entry's canonical tier_models position, execution_mode=the
// landing/attempted entry's mode, fallback_from=json(chain[:pos]) for pos>0.
func (m *Manager) walkFoldChain(ctx context.Context, target foldTarget, projectID, userText string, def *model.SystemAgentDefinition, chain []service.AgentChainEntry) (content string, usage provider.Usage, execMode string, ok bool) {
	logKey := target.logKey()
	chain = m.applyWeightedRotation(ctx, chain, projectID)

	for pos := 0; pos < len(chain); pos++ {
		entry := chain[pos]

		// A statically doomed api entry (credentials unresolvable) is skipped
		// when a later entry exists — no attempt, no refinery_runs row —
		// instead of failing the same way on every fold.
		if entry.ExecutionMode != "cli_interactive" && pos < len(chain)-1 &&
			!hasAPICreds(ctx, m.pool, m.clock, entry.Provider, projectID) {
			logger.Debug(ctx, "refinery: skipping api entry, credentials unavailable", "key", logKey, "chain_pos", pos, "provider", entry.Provider)
			continue
		}

		var res foldAttemptResult
		if entry.ExecutionMode == "cli_interactive" {
			res = m.attemptFoldCLI(ctx, target, projectID, userText, entry)
		} else {
			res = m.attemptFoldAPI(ctx, target, projectID, userText, def, entry)
		}

		fallbackFrom := ""
		if pos > 0 {
			if b, err := json.Marshal(chain[:pos]); err == nil {
				fallbackFrom = string(b)
			}
		}

		if res.err == nil {
			m.recordFoldRun(ctx, target, projectID, res.provName, res.modelID, res.usage, "ok", "", entry.Position, entry.ExecutionMode, fallbackFrom)
			return res.content, res.usage, entry.ExecutionMode, true
		}

		m.recordFoldRun(ctx, target, projectID, res.provName, res.modelID, res.usage, "failed", res.err.Error(), entry.Position, entry.ExecutionMode, fallbackFrom)

		if !res.advance {
			logger.Warn(ctx, "refinery: fold attempt stopped, not advancing chain", "key", logKey, "chain_pos", pos, "execution_mode", entry.ExecutionMode, "error", res.err)
			return "", provider.Usage{}, "", false
		}
		logger.Warn(ctx, "refinery: fold attempt failed, advancing chain", "key", logKey, "chain_pos", pos, "execution_mode", entry.ExecutionMode, "error", res.err)
	}

	logger.Warn(ctx, "refinery: fold chain exhausted", "key", logKey, "chain_len", len(chain))
	return "", provider.Usage{}, "", false
}

// attemptFoldAPI runs one api-mode chain entry: resolve the model row, build
// the provider, run a single direct provider.Run (no tools) over userText,
// and cap the result to maxDigestBytes. A model-row lookup failure, an empty
// api_model, or a buildProvider failure is advance-eligible (build-time,
// mirrors isProviderBuildError). A provider.Run error is classified via the
// shared apirun.ClassifyProviderError: RetryClassError advances,
// RetryClassRateLimit/RetryClassNone stop. Degenerate output always stops.
func (m *Manager) attemptFoldAPI(ctx context.Context, target foldTarget, projectID, userText string, def *model.SystemAgentDefinition, entry service.AgentChainEntry) foldAttemptResult {
	logKey := target.logKey()

	modelRow, err := m.modelSvc.Get(entry.ModelID)
	if err != nil {
		logger.Error(ctx, "refinery: resolve model row failed", "key", logKey, "model", entry.ModelID, "error", err)
		return foldAttemptResult{modelID: entry.ModelID, advance: true, err: err}
	}
	if modelRow.APIModel == "" {
		logger.Error(ctx, "refinery: model row has no api_model", "key", logKey, "model", entry.ModelID)
		return foldAttemptResult{provName: modelRow.Provider, modelID: entry.ModelID, advance: true, err: errors.New("model row has no api_model")}
	}

	prov, err := buildProvider(ctx, m.pool, m.clock, modelRow.Provider, projectID)
	if err != nil {
		logger.Error(ctx, "refinery: build provider failed", "key", logKey, "provider", modelRow.Provider, "error", err)
		return foldAttemptResult{provName: modelRow.Provider, modelID: entry.ModelID, advance: true, err: err}
	}

	maxTokens := defaultFoldMaxTokens
	if def.APIMaxTokens != nil && *def.APIMaxTokens > 0 {
		maxTokens = *def.APIMaxTokens
	}

	req := provider.Request{
		System: def.Prompt,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type: "text",
				Text: userText,
			}},
		}},
		MaxTokens:       maxTokens,
		Model:           modelRow.APIModel,
		ReasoningEffort: entry.ReasoningEffort,
	}

	resp, err := prov.Run(ctx, req, noopSink{})
	if err != nil {
		logger.Error(ctx, "refinery: provider run failed", "key", logKey, "error", err)
		_, _, class := apirun.ClassifyProviderError(ctx, err)
		return foldAttemptResult{provName: modelRow.Provider, modelID: entry.ModelID, advance: class == apirun.RetryClassError, err: err}
	}

	text := extractText(resp.Content)
	if strings.TrimSpace(text) == "" || isDegenerateStopReason(resp.StopReason) {
		logger.Warn(ctx, "refinery: rejecting degenerate fold output", "key", logKey, "stop_reason", resp.StopReason, "text_bytes", len(text))
		return foldAttemptResult{provName: modelRow.Provider, modelID: entry.ModelID, usage: resp.Usage, advance: false, err: errors.New("degenerate fold output")}
	}

	return foldAttemptResult{
		content:  foldfmt.CapBytes(text, maxDigestBytes),
		usage:    resp.Usage,
		provName: modelRow.Provider,
		modelID:  entry.ModelID,
	}
}
