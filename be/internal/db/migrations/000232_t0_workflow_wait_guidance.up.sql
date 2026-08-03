-- workflow_wait joins the t0-bare/t0-decider console catalogues
-- (console/profiles.go, nrworkflow-513592): append guidance so the templates
-- point at it instead of leaving the model to invent sleep-timer delegations
-- as a pacing clock.

UPDATE default_templates
SET template = template || '- After launching a run (dynamic_workflow/workflow_run), block on `workflow_wait` until it changes state — never spawn sleep/timer delegations to pace polling.
',
    default_template = default_template || '- After launching a run (dynamic_workflow/workflow_run), block on `workflow_wait` until it changes state — never spawn sleep/timer delegations to pace polling.
',
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
