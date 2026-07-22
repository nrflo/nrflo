-- tier_models is an ordered per-tier fallback chain: for a given tier, the
-- row at position 0 is the primary entry, higher positions are the
-- advance-on-failure fallbacks (advancement itself is a separate ticket).
-- system_agent_definitions.tier is nullable (NULL = untiered, no chain) and
-- selects a chain; ResolveAgentChain (service layer) prefers a per-agent
-- model override over the tier chain, then walks down to the nearest lower
-- populated tier when the assigned tier has no rows (inheritance).
CREATE TABLE tier_models (
    tier INTEGER NOT NULL,
    position INTEGER NOT NULL,
    provider TEXT NOT NULL,
    execution_mode TEXT NOT NULL,
    model_id TEXT NOT NULL,
    reasoning_effort TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (tier, position)
);

ALTER TABLE system_agent_definitions ADD COLUMN tier INTEGER
    CHECK (tier IS NULL OR tier BETWEEN 1 AND 5);

-- Backfill tier for the 9 system agents: tier1 = haiku-class, tier4 = sonnet-class.
UPDATE system_agent_definitions SET tier = 1
    WHERE id IN ('_refinery', '_t2_extractor', 'context-saver', 'context-saver-api', 'spec-normalizer');
UPDATE system_agent_definitions SET tier = 4
    WHERE id IN ('_t1_executor', 'conflict-resolver', 'planner-system', 'planner-system-api');

-- Seed the anthropic api -> cli_interactive fallback chains for the tiers
-- actually used. Per-agent model overrides on system_agent_definitions are
-- left untouched, so the resolved primary entry still matches each agent's
-- current model (behavior-preserving; the chain is inert until overrides
-- are cleared by the advance-on-failure follow-up).
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort) VALUES
    (1, 0, 'anthropic', 'api', 'haiku-4-5', 'low'),
    (1, 1, 'anthropic', 'cli_interactive', 'haiku-4-5', 'low'),
    (4, 0, 'anthropic', 'api', 'sonnet-5', 'medium'),
    (4, 1, 'anthropic', 'cli_interactive', 'sonnet-5', 'medium');
