-- native_tools: CSV allowlist of the CLI's built-in tools for anthropic
-- cli_interactive agents, emitted as claude --tools; '' = unrestricted,
-- sentinel 'none' = disable all native tools (MCP-only agent).
-- sandbox: codex app-server thread/start sandbox for openai cli_interactive
-- agents (read-only | workspace-write | danger-full-access);
-- '' = danger-full-access. Both backfill-safe: existing rows default to ''.
ALTER TABLE agent_definitions ADD COLUMN native_tools TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_definitions ADD COLUMN sandbox TEXT NOT NULL DEFAULT '';
