-- The delegate builtin returns extractor findings inline in the tool call
-- (default wait_sec 120 — tools_builtin/delegate.go), so 000233's blanket
-- wait-for-notification guidance mis-instructed the common extractor case:
-- scope the end-your-turn contract to async launches and state the inline
-- extractor contract explicitly.

UPDATE default_templates
SET template = REPLACE(template,
	'- After launching a delegation or a run (delegate/dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.',
	'- Extractor delegations return their findings inline in the delegate call itself (default wait_sec 120) — no follow-up needed. After an async launch (executor delegation with wait_sec 0, dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    default_template = REPLACE(default_template,
	'- After launching a delegation or a run (delegate/dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.',
	'- Extractor delegations return their findings inline in the delegate call itself (default wait_sec 120) — no follow-up needed. After an async launch (executor delegation with wait_sec 0, dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
