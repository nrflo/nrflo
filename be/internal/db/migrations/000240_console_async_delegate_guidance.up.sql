-- Console chats now launch EVERY delegation tier async by default (the
-- delegate builtin's defaultWaitSec: an inline extractor/verifier block kept
-- the interactive turn busy for up to 2 minutes, deafening the chat to its
-- human, while the ChatNotifier already delivers completions as a turn).
-- Rewrite the t0 templates' inline-return contract to the async one, and
-- scope the delegation-guidance injectable's extractor bullet per surface
-- (workflow runs keep the inline default).

UPDATE default_templates
SET template = REPLACE(template,
    '- Extractor and verifier delegations return their findings inline in the delegate call itself (default wait_sec 120) — no follow-up needed. After an async launch (executor delegation with wait_sec 0, dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.',
    '- Every delegation in this chat launches async (extractor/verifier included): the delegate call returns immediately. After any launch (delegate, dynamic_workflow, workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    default_template = REPLACE(default_template,
    '- Extractor and verifier delegations return their findings inline in the delegate call itself (default wait_sec 120) — no follow-up needed. After an async launch (executor delegation with wait_sec 0, dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.',
    '- Every delegation in this chat launches async (extractor/verifier included): the delegate call returns immediately. After any launch (delegate, dynamic_workflow, workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');

-- delegation-guidance is appended to every prompt whose tool set includes
-- `delegate` — workflow agents AND console chats — so its inline-contract
-- bullets must name both defaults. Readonly row: template == default_template.
UPDATE default_templates
SET template = REPLACE(REPLACE(template,
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.',
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — in workflow runs it blocks inline by default and returns the result in one call; in interactive console chats it launches async and the completion arrives as a chat notification — collect it then with ONE `get_delegation` call.'),
    '- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot inline contract as extractor, stronger model, refute-by-default.',
    '- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot contract and defaults as extractor, stronger model, refute-by-default.'),
    default_template = REPLACE(REPLACE(default_template,
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.',
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — in workflow runs it blocks inline by default and returns the result in one call; in interactive console chats it launches async and the completion arrives as a chat notification — collect it then with ONE `get_delegation` call.'),
    '- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot inline contract as extractor, stronger model, refute-by-default.',
    '- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot contract and defaults as extractor, stronger model, refute-by-default.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'delegation-guidance';
