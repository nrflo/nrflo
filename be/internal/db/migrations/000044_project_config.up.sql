-- Recreate config table with composite PK (project_id, key)
CREATE TABLE config_new (
    project_id TEXT NOT NULL DEFAULT '',
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (project_id, key)
);
INSERT INTO config_new (project_id, key, value) SELECT '', key, value FROM config;
DROP TABLE config;
ALTER TABLE config_new RENAME TO config;

-- Add config column to agent_sessions for audit/replay of safety settings
ALTER TABLE agent_sessions ADD COLUMN config TEXT NOT NULL DEFAULT '';
