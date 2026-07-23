package spawner

import "fmt"

// codexAutoCompactTokenLimit pins codex app-server's own auto-compaction
// threshold far above any real session so nrflo's context_left kill→save→
// relaunch stays the single authority over context shedding.
//
// Evidence (probed live against codex-cli 0.145.0, 2026-07-23): every
// built-in 0.145 model preset ships `auto_compact_token_limit: null`, so the
// pin is normally a no-op override of "no limit" — but writeCodexProfileForSession
// copies the developer's ~/.codex/config.toml verbatim into the per-session
// profile, and a user with `model_auto_compact_token_limit` already set there
// (or a future remote model-catalog change) would silently flip managed-agent
// behaviour. A `-c` override wins over both the preset AND the user's
// config.toml (validated: `-c model_auto_compact_token_limit=1` forced
// compaction even against a config.toml with no such key), so it is delivered
// as an argv `-c` pair, the same mechanism as codexProjectDocArgs.
//
// codex's per-thread token/rollout budgets (TokenBudgetConfigToml,
// RolloutBudgetConfigToml) are server-pushed and NOT client-settable — `-c
// token_budget=...`/`-c rollout_budget=...` fail with "unknown configuration
// field" under --strict-config — so remote compaction stays possible and is
// detected via dispatchContextCompaction below, not disabled.
const codexAutoCompactTokenLimit = 1_000_000_000

// codexAutoCompactArgs returns the `-c model_auto_compact_token_limit=<N>`
// pair for appServerArgs().
func codexAutoCompactArgs() []string {
	return []string{"-c", fmt.Sprintf("model_auto_compact_token_limit=%d", codexAutoCompactTokenLimit)}
}

// dispatchContextCompaction handles a completed `contextCompaction` item: it
// resets context_left to 100 (the DB half of the reset; codexAppServerBackend's
// run loop resets the in-memory proc.contextLeft watermark from the returned
// appServerSignal), records a system message noting the compaction, and emits
// EventContextCompacted so the codex ledger emitter can reset its own
// composition. Kept pure (no proc) so it stays unit-testable like the rest of
// the events mapper.
func dispatchContextCompaction(sessionID string, sink Sink, emit EventEmitter) {
	emitMessage(sessionID, "[context] codex compacted the thread context; nrflo context_left reset pending next usage report", "system", sink)
	sink.UpdateContextLeft(sessionID, 100)
	emitContextCompactedEvent(sessionID, emit)
}
