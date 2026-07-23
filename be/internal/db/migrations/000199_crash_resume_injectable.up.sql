-- Seed a readonly `crash-resume` injectable: rendered as the first turn's
-- input when a crash/fail-restart relaunch resumes the previous codex
-- app-server thread instead of a fresh spawn (spawner/codex_appserver_resume.go
-- startOrResumeThread). The conversation above is already inside the resumed
-- thread — this text stands in for the full re-rendered prompt.

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('crash-resume', 'Crash Resume',
     'Your previous run was interrupted (${RESTART_REASON}). The conversation above is intact — do not redo work already completed in it.

Your nrflo session id has been replaced: drive nrflo only through your MCP tools, never through a session id quoted earlier in this conversation. The tools take your identity from the current session automatically.

## Completion Contract

You MUST call exactly one of the completion tools to finish this run — do not exit silently.

- **Success**: call `agent_finished` — the orchestrator advances to the next phase.
- **Failure**: call `agent_fail` with a reason — the orchestrator stops at this layer.

Continue from where you left off.',
     'Your previous run was interrupted (${RESTART_REASON}). The conversation above is intact — do not redo work already completed in it.

Your nrflo session id has been replaced: drive nrflo only through your MCP tools, never through a session id quoted earlier in this conversation. The tools take your identity from the current session automatically.

## Completion Contract

You MUST call exactly one of the completion tools to finish this run — do not exit silently.

- **Success**: call `agent_finished` — the orchestrator advances to the next phase.
- **Failure**: call `agent_fail` with a reason — the orchestrator stops at this layer.

Continue from where you left off.',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
