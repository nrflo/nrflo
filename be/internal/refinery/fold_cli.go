package refinery

import (
	"context"
	"errors"
	"strings"
	"time"

	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
)

// cliFoldTimeout bounds a cli_interactive fold attempt's one-off headless
// child — well under the 30s sidecar debounce and the 10s console Stop
// budget, so a stuck child never blocks either.
const cliFoldTimeout = 90 * time.Second

// CLIFolder runs one cli_interactive chain-entry fold attempt as a one-off
// headless child. Declared here (not imported) so refinery never depends on
// spawner; satisfied structurally by *spawner.Spawner
// (spawner.RunRefineryFold) — the exact mirror of spawner.RefinerySidecar's
// structural satisfaction by *refinery.Manager.
type CLIFolder interface {
	RunRefineryFold(ctx context.Context, req types.RefineryFoldRequest) (types.RefineryFoldResult, error)
}

// attemptFoldCLI resolves the `_refinery-cli` system agent def (a missing row
// counts as an unavailable backend, same as a nil seam), builds the fold
// request, and delegates the spawn to the nil-safe cliFolder seam. A
// types.ErrRefineryFoldProviderBuild-wrapped error advances the chain
// (build-time/credential failure); any other error stops it (mirrors a
// structural fold failure).
func (m *Manager) attemptFoldCLI(ctx context.Context, target foldTarget, projectID, userText string, entry service.AgentChainEntry) foldAttemptResult {
	logKey := target.logKey()

	if m.cliFolder == nil {
		logger.Warn(ctx, "refinery: no CLIFolder configured, advancing chain", "key", logKey)
		return foldAttemptResult{provName: entry.Provider, modelID: entry.ModelID, advance: true, err: errors.New("refinery: no CLIFolder configured")}
	}

	if _, err := m.systemAgentSvc.GetForBackend("refinery", "cli_interactive"); err != nil {
		logger.Warn(ctx, "refinery: _refinery-cli def not found, advancing chain", "key", logKey, "error", err)
		return foldAttemptResult{provName: entry.Provider, modelID: entry.ModelID, advance: true, err: err}
	}

	foldCtx, cancel := context.WithTimeout(ctx, cliFoldTimeout)
	defer cancel()

	req := types.RefineryFoldRequest{
		ProjectID:          projectID,
		SessionID:          target.sessionID,
		WorkflowInstanceID: target.workflowInstanceID,
		NodeID:             target.nodeID,
		ModelID:            entry.ModelID,
		Provider:           entry.Provider,
		ReasoningEffort:    entry.ReasoningEffort,
		UserText:           userText,
		TimeoutSec:         int(cliFoldTimeout.Seconds()),
	}

	res, err := m.cliFolder.RunRefineryFold(foldCtx, req)
	if err != nil {
		advance := errors.Is(err, types.ErrRefineryFoldProviderBuild)
		logger.Warn(ctx, "refinery: cli fold attempt failed", "key", logKey, "error", err, "advance", advance)
		return foldAttemptResult{provName: entry.Provider, modelID: entry.ModelID, advance: advance, err: err}
	}

	if strings.TrimSpace(res.Content) == "" {
		logger.Warn(ctx, "refinery: rejecting degenerate cli fold output", "key", logKey)
		return foldAttemptResult{provName: entry.Provider, modelID: entry.ModelID, advance: false, err: errors.New("degenerate fold output")}
	}

	return foldAttemptResult{
		content:  foldfmt.CapBytes(res.Content, maxDigestBytes),
		usage:    provider.Usage{InputTokens: res.InputTokens, OutputTokens: res.OutputTokens},
		provName: entry.Provider,
		modelID:  entry.ModelID,
	}
}
