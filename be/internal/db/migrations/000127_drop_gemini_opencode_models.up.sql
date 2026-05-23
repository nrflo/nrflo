-- Reassign agent definitions off gemini/opencode models, delete orphaned
-- cli_models rows, and rebuild cli_models with CHECK(claude,codex) only.

PRAGMA foreign_keys = OFF;

BEGIN;

-- Reassign agent_definitions that reference removed models to sonnet.
UPDATE agent_definitions
    SET model = 'sonnet'
    WHERE model LIKE 'gemini_%' OR model LIKE 'opencode_%';

UPDATE agent_definitions
    SET low_consumption_model = ''
    WHERE low_consumption_model LIKE 'gemini_%'
       OR low_consumption_model LIKE 'opencode_%';

UPDATE system_agent_definitions
    SET model = 'sonnet'
    WHERE model LIKE 'gemini_%' OR model LIKE 'opencode_%';

-- Delete the removed cli_models rows.
DELETE FROM cli_models WHERE cli_type IN ('gemini', 'opencode');

-- Rebuild cli_models with tightened CHECK constraint.
-- All current columns must be listed: id, cli_type, display_name, mapped_model,
-- reasoning_effort, context_length, read_only, created_at, updated_at, enabled,
-- override_system_prompt (added by 000126).
CREATE TABLE cli_models_new (
    id                    TEXT    PRIMARY KEY,
    cli_type              TEXT    NOT NULL,
    display_name          TEXT    NOT NULL,
    mapped_model          TEXT    NOT NULL,
    reasoning_effort      TEXT    NOT NULL DEFAULT '',
    context_length        INTEGER NOT NULL DEFAULT 200000,
    read_only             INTEGER NOT NULL DEFAULT 0,
    created_at            TEXT    NOT NULL,
    updated_at            TEXT    NOT NULL,
    enabled               INTEGER NOT NULL DEFAULT 1,
    override_system_prompt INTEGER NOT NULL DEFAULT 0,
    CHECK (cli_type IN ('claude', 'codex'))
);

INSERT INTO cli_models_new
    SELECT id, cli_type, display_name, mapped_model, reasoning_effort,
           context_length, read_only, created_at, updated_at, enabled,
           override_system_prompt
    FROM cli_models;

DROP TABLE cli_models;
ALTER TABLE cli_models_new RENAME TO cli_models;

COMMIT;

PRAGMA foreign_keys = ON;
