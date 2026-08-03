DELETE FROM system_agent_definitions WHERE id = '_refinery-cli';

-- Rebuild refinery_runs without chain_position/fallback_from/execution_mode
-- (SQLite table rebuild, same shape as 000105).
CREATE TABLE refinery_runs_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL DEFAULT '',
	workflow_instance_id TEXT NOT NULL DEFAULT '',
	node_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL CHECK (status IN ('ok', 'failed')),
	error TEXT NOT NULL DEFAULT '',
	fold_count INTEGER NOT NULL DEFAULT 0,
	folded_at TEXT NOT NULL
);

INSERT INTO refinery_runs_new
    SELECT id, session_id, workflow_instance_id, node_id, project_id, provider, model,
           prompt_tokens, output_tokens, status, error, fold_count, folded_at
    FROM refinery_runs;

DROP TABLE refinery_runs;
ALTER TABLE refinery_runs_new RENAME TO refinery_runs;

CREATE INDEX idx_refinery_runs_folded_at ON refinery_runs (folded_at DESC);
