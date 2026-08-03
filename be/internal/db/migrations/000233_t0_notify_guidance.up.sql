-- The server now pushes delegation/sub-workflow completions into the
-- launching console chat as a turn (console.ChatNotifier), and tier-t0-*
-- templates are used only by console-chat profiles — so replace 000232's
-- poll-with-workflow_wait guidance with wait-for-notification guidance.

UPDATE default_templates
SET template = REPLACE(template,
	'- After launching a run (dynamic_workflow/workflow_run), block on `workflow_wait` until it changes state — never spawn sleep/timer delegations to pace polling.',
	'- After launching a delegation or a run (delegate/dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    default_template = REPLACE(default_template,
	'- After launching a run (dynamic_workflow/workflow_run), block on `workflow_wait` until it changes state — never spawn sleep/timer delegations to pace polling.',
	'- After launching a delegation or a run (delegate/dynamic_workflow/workflow_run), simply end your turn — the server sends you a message when it completes, fails, or parks for plan input/approval. Never poll, never block on waits, never spawn timer delegations; act only when notified.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
