-- Refinery sidecar: seed the `_refinery` api-mode system agent, create the
-- refinery_digests head table (single row per console-chat session), and
-- point the `working-set` injectable at the ${DIGEST} var. readonly=1 rows
-- require template == default_template (migration058 invariant) — both
-- columns below carry the identical literal.

INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools, api_max_tokens,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode,
    created_at, updated_at
) VALUES (
    '_refinery',
    'refinery',
    'haiku-4-5',
    3,
    '# Working-Set Refinery

You maintain a compact working-set digest for an ongoing console conversation between a user and an AI agent. Given the previous digest and a batch of new events (finding updates, completed workflow results, plan-state changes), produce an updated digest.

Cover, in order: goal, constraints, plan state, active findings, open questions. Drop stale or superseded detail rather than growing unbounded — the digest must never exceed 4000 bytes.

Output ONLY the updated digest text: no preamble, no code fences, no commentary.',
    '',
    1500,
    60,
    120,
    'api',
    datetime('now'),
    datetime('now')
);

CREATE TABLE IF NOT EXISTS refinery_digests (
    console_session_id TEXT    PRIMARY KEY,
    project_id          TEXT    NOT NULL,
    version              INTEGER NOT NULL DEFAULT 0,
    content              TEXT    NOT NULL DEFAULT '',
    fold_count           INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL,
    FOREIGN KEY (console_session_id) REFERENCES agent_sessions (id) ON DELETE CASCADE
);

UPDATE default_templates
SET template = '## Working Set

${DIGEST}
',
    default_template = '## Working Set

${DIGEST}
',
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'working-set';
