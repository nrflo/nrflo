-- Per-agent API-mode output token cap (max_tokens). NULL -> spawner default.
-- Mirrors api_max_iterations (000062/000063); only meaningful for execution_mode='api'.
ALTER TABLE agent_definitions ADD COLUMN api_max_tokens INTEGER;
ALTER TABLE system_agent_definitions ADD COLUMN api_max_tokens INTEGER;
