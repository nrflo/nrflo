-- The bundled deep-research workflow was removed from the product; wipe its
-- seeded definition and any historical runs. FKs (foreign_keys=1) cascade
-- workflows -> agent_definitions + workflow_layer_policies (def rows) and
-- workflows -> workflow_instances (def_project_id FK) -> agent_sessions ->
-- agent_messages / artifacts / plan_revisions / workflow_instance_nodes.
-- findings / findings_history carry no FK, so delete them explicitly first
-- (findings_history.finding_id is ON DELETE SET NULL — history rows survive
-- the findings delete). Each history DELETE pins the scope column so
-- idx_findings_history_scope(scope, scope_id, ...) can seek instead of
-- scanning the whole table.
DELETE FROM findings_history WHERE scope = 'session' AND scope_id IN (
    SELECT id FROM agent_sessions WHERE workflow_instance_id IN (
        SELECT id FROM workflow_instances WHERE def_project_id = '__global__' AND workflow_id = 'deep-research'));
DELETE FROM findings_history WHERE scope = 'workflow_instance' AND scope_id IN (
    SELECT id FROM workflow_instances WHERE def_project_id = '__global__' AND workflow_id = 'deep-research');
DELETE FROM findings WHERE workflow_instance_id IN (
    SELECT id FROM workflow_instances WHERE def_project_id = '__global__' AND workflow_id = 'deep-research');
DELETE FROM workflows WHERE project_id = '__global__' AND id = 'deep-research';
