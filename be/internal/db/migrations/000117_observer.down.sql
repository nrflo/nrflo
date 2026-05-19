ALTER TABLE agent_sessions DROP COLUMN observer_scope;
ALTER TABLE agent_sessions DROP COLUMN kind;
ALTER TABLE workflows DROP COLUMN observer_model;
ALTER TABLE workflows DROP COLUMN observer_provider;
ALTER TABLE workflows DROP COLUMN observer_context;
