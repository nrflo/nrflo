-- Add observer_workflow_id to agent_sessions so workflow-scope observer sessions
-- can persist their bound workflow definition id. Required for socket-side scope
-- authorization on observer.workflow.* calls; observer sessions are not tied to a
-- specific workflow_instance, so we cannot derive the workflow id by joining.
ALTER TABLE agent_sessions ADD COLUMN observer_workflow_id TEXT DEFAULT NULL;
