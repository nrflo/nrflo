-- Extends the shipped system-agent tier matrix (000195) to regular workflow
-- agent_definitions: same nullable tier column, same CHECK shape.
ALTER TABLE agent_definitions ADD COLUMN tier INTEGER
    CHECK (tier IS NULL OR tier BETWEEN 1 AND 5);

-- tier_models.execution_mode gains '' = "inherit the agent's own mode",
-- letting one global tier serve both api-mode system agents and cli-mode
-- workflow agents. Rewrite the existing tier1/tier4 position-0 rows to ''
-- — today those chains are inert (all 9 system agents still carry model
-- overrides per 000195), and tier 4 spans both planner-system
-- (cli_interactive) and planner-system-api (api), so a pinned api primary
-- would flip modes.
UPDATE tier_models SET execution_mode = '' WHERE tier IN (1, 4) AND position = 0;

-- Seed tier 2 (setup-analyzer/qa-verifier class) and tier 3
-- (test-writer/implementor class) chains: inherit-mode primary, explicit
-- cli_interactive fallback.
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort) VALUES
    (2, 0, 'anthropic', '', 'sonnet-5', 'low'),
    (2, 1, 'anthropic', 'cli_interactive', 'sonnet-5', 'low'),
    (3, 0, 'anthropic', '', 'sonnet-5', 'medium'),
    (3, 1, 'anthropic', 'cli_interactive', 'sonnet-5', 'medium');

-- Behavior-preserving backfill: only agent_definitions rows already fully
-- re-tiered onto TierMap's recommended model/effort/template are switched
-- to tier-driven (tier set, model/effort cleared). Rows still on their
-- original seed model are left untouched and simply surface in the Tiering
-- report as applicable.
UPDATE agent_definitions SET tier = 2, model = '', reasoning_effort = NULL
    WHERE node_role = 'static' AND consultant = 0 AND id = 'setup-analyzer'
      AND model = 'sonnet-5' AND COALESCE(reasoning_effort, '') = 'low'
      AND system_template_id = 'tier-t2-extractor';

UPDATE agent_definitions SET tier = 2, model = '', reasoning_effort = NULL
    WHERE node_role = 'static' AND consultant = 0 AND id = 'qa-verifier'
      AND model = 'sonnet-5' AND COALESCE(reasoning_effort, '') = 'low'
      AND system_template_id = 'tier-t2-extractor';

UPDATE agent_definitions SET tier = 3, model = '', reasoning_effort = NULL
    WHERE node_role = 'static' AND consultant = 0 AND id = 'test-writer'
      AND model = 'sonnet-5' AND COALESCE(reasoning_effort, '') = 'medium'
      AND system_template_id = 'tier-t1-executor';

UPDATE agent_definitions SET tier = 3, model = '', reasoning_effort = NULL
    WHERE node_role = 'static' AND consultant = 0 AND id = 'implementor'
      AND NOT (workflow_id = 'hotfix')
      AND model = 'sonnet-5' AND COALESCE(reasoning_effort, '') = 'medium'
      AND system_template_id = 'tier-t1-executor';

UPDATE agent_definitions SET tier = 1, model = '', reasoning_effort = NULL
    WHERE node_role = 'static' AND consultant = 0 AND id = 'doc-updater'
      AND model = 'haiku-4-5' AND COALESCE(reasoning_effort, '') = 'low'
      AND system_template_id = 'tier-t1-executor';
