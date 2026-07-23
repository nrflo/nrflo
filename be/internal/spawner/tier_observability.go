package spawner

import (
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
)

// recordResolvedSpawn best-effort persists what actually resolved for proc's
// spawn: provider/execution_mode/effort/tier (from chain[pos] when a chain
// was resolved, else derived from the plain model config) and
// fallback_from — the chain entries attempted and failed before pos, empty
// when pos==0 so the column stays NULL. Never fails the spawn: nil pool is a
// no-op, errors are logged only (mirrors the spawn hot path in database.go).
func (s *Spawner) recordResolvedSpawn(proc *processInfo, chain []service.AgentChainEntry, pos int) {
	pool := s.pool()
	if pool == nil {
		return
	}

	var provider, execMode, effort string
	var tier *int
	if pos >= 0 && pos < len(chain) {
		entry := chain[pos]
		provider, execMode, effort = entry.Provider, entry.ExecutionMode, entry.ReasoningEffort
		t := entry.Tier
		tier = &t
	} else {
		cli, modelSlug := parseModelID(proc.modelID)
		if mc, ok := s.config.ModelConfigs[modelSlug]; ok {
			provider = mc.Provider
		} else {
			provider = cliForProvider(cli)
		}
		execMode = proc.effectiveMode
		effort = proc.resolvedEffort
	}

	fallbackFrom := ""
	if pos > 0 && pos <= len(chain) {
		if b, err := json.Marshal(chain[:pos]); err == nil {
			fallbackFrom = string(b)
		}
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	if err := sessionRepo.UpdateTierResolution(proc.sessionID, tier, provider, execMode, effort, pos, fallbackFrom); err != nil {
		logger.Warn(context.Background(), "record resolved spawn: update tier resolution failed",
			"session_id", proc.sessionID, "err", err)
	}
}
