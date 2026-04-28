-- Add role, execution_mode, tools, api_max_iterations to system_agent_definitions.
-- Mirrors the pattern from 000062_api_mode.up.sql for agent_definitions.

ALTER TABLE system_agent_definitions ADD COLUMN role TEXT NOT NULL DEFAULT '';
ALTER TABLE system_agent_definitions ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'cli'
    CHECK (execution_mode IN ('cli', 'api'));
ALTER TABLE system_agent_definitions ADD COLUMN tools TEXT NOT NULL DEFAULT '';
ALTER TABLE system_agent_definitions ADD COLUMN api_max_iterations INTEGER;

-- Backfill role = id for all existing rows (legacy rows get role matching their id).
UPDATE system_agent_definitions SET role = id WHERE role = '';

-- Unique index ensures at most one row per (role, execution_mode) pair.
CREATE UNIQUE INDEX idx_system_agent_role_mode ON system_agent_definitions (role, execution_mode);

-- Seed the API-mode context-saver sibling.
-- Session-targeting via ${TARGET_SESSION_ID} is preserved as a template variable
-- for future spawner integration; findings_add writes to the current session only
-- (cross-session write is a follow-up concern).
INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools, api_max_iterations,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode,
    created_at, updated_at
) VALUES (
    'context-saver-api',
    'context-saver',
    'haiku',
    3,
    '# Context Saver

You are a context-saving agent. Your job is to analyze an agent''s message history and produce a concise progress summary so a fresh agent can continue the work.

## Agent Info

- **Agent type**: ${AGENT_TYPE}
- **Workflow**: ${WORKFLOW}
- **Ticket**: ${TICKET_ID}
- **Target session**: ${TARGET_SESSION_ID}

## Message History

The following is the message history from the agent whose context ran low:

<messages>
${AGENT_MESSAGES}
</messages>

## Task

Analyze the message history above and produce a concise summary covering:
1. **What was accomplished** — files created/modified, features implemented, bugs fixed
2. **Current state** — what is working, what was last being worked on
3. **What remains** — tasks not yet started or partially completed
4. **Key decisions** — any important design choices or constraints discovered

Call the `findings_add` tool with:
- key: `to_resume`
- value: your concise summary (keep it under 2000 characters)

## Rules

- Keep the summary under 2000 characters — a fresh agent needs a quick briefing, not a novel
- Focus on actionable information: file paths, function names, error messages, remaining TODOs
- Do NOT re-read files or run any code — work only from the message history provided
- Call `findings_add` exactly once with key=to_resume and your summary as the value',
    'findings_add',
    8,
    60,
    120,
    'api',
    datetime('now'),
    datetime('now')
);
