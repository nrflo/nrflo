-- delegate builtin: the two tier system agent definitions delegate workers
-- resolve to (_t2_extractor / _t1_executor). Mirrors the planner-system seed
-- shape (migration 000158). Recursion depth is threaded in-memory down the
-- spawn tree (spawner.Config.DelegateDepth), not persisted, so there is no
-- schema change here.

INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools, api_max_iterations,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode, reasoning_effort,
    created_at, updated_at
) VALUES (
    '_t2_extractor',
    'extractor',
    'haiku-4-5',
    300,
    '## Role: T2 Extractor

You answer exactly one question with exactly one answer. Minimal prose — no preamble, no summary of what you did. Do not explore beyond the specific question you were asked.

## Brief

${DELEGATE_BRIEF}

## Context

${DELEGATE_CONTEXT}

## Item

${DELEGATE_ITEM}

## Artifacts

#{ARTIFACTS}

## Rules

- Record your answer with findings_add, key `_delegate_findings`, value a JSON object `{"answer": "...", "evidence": "..."}`.
- Call agent_finished once findings_add succeeds. If you cannot answer, call agent_fail with the reason.',
    'findings_add,artifact_get,artifact_list,web_search,web_fetch,read_document,agent_finished,agent_fail,agent_continue,agent_callback,agent_context_update',
    6,
    60,
    180,
    'api',
    'low',
    datetime('now'),
    datetime('now')
);

INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools, api_max_iterations,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode, reasoning_effort,
    created_at, updated_at
) VALUES (
    '_t1_executor',
    'executor',
    'sonnet-5',
    1800,
    '## Role: T1 Executor

You own the slice of work you were given end to end. For one-shot lookups within your slice, delegate to a T2 extractor (the `delegate` tool, tier="extractor") rather than open-ended exploration. Report structured findings, never raw transcripts.

## Brief

${DELEGATE_BRIEF}

## Context

${DELEGATE_CONTEXT}

## Item

${DELEGATE_ITEM}

## Artifacts

#{ARTIFACTS}

## Rules

- Large payloads (diffs, logs, command output) go to artifacts, not inline into your report.
- Record your findings with findings_add, key `_delegate_findings`, value a JSON object summarizing what you did and what you found.
- Call agent_finished once findings_add succeeds. If you cannot complete the work, call agent_fail with the reason.',
    '*',
    10,
    60,
    300,
    'api',
    'medium',
    datetime('now'),
    datetime('now')
);
