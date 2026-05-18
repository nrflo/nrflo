ALTER TABLE python_scripts ADD COLUMN kind TEXT NOT NULL DEFAULT 'agent'
    CHECK (kind IN ('agent', 'tool'));
ALTER TABLE python_scripts ADD COLUMN tool_description TEXT NOT NULL DEFAULT '';
ALTER TABLE python_scripts ADD COLUMN input_schema TEXT NOT NULL DEFAULT '{}';
ALTER TABLE python_scripts ADD COLUMN timeout_sec INTEGER NOT NULL DEFAULT 30;

CREATE UNIQUE INDEX python_scripts_tool_name ON python_scripts(project_id, name)
    WHERE kind = 'tool';

DROP INDEX IF EXISTS idx_review_items_lookup;
DROP TABLE IF EXISTS review_items;

DROP INDEX IF EXISTS idx_customer_config_versions_unique;
DROP TABLE IF EXISTS customer_config_versions;

DELETE FROM config WHERE key = 'customer_config_dir';

DELETE FROM tool_dispatches
WHERE tool_name NOT IN (SELECT name FROM tool_definitions)
  AND tool_name NOT IN (
      'findings_add', 'findings_add_bulk', 'findings_append', 'findings_append_bulk',
      'findings_get', 'findings_delete',
      'project_findings_add', 'project_findings_add_bulk', 'project_findings_append',
      'project_findings_append_bulk', 'project_findings_get', 'project_findings_delete',
      'agent_fail', 'agent_finished', 'agent_continue', 'agent_callback', 'agent_context_update',
      'workflow_skip',
      'artifact_add', 'artifact_list', 'artifact_get'
  );
