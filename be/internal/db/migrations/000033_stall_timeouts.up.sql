ALTER TABLE agent_definitions ADD COLUMN stall_start_timeout_sec INTEGER DEFAULT NULL;
ALTER TABLE agent_definitions ADD COLUMN stall_running_timeout_sec INTEGER DEFAULT NULL;
