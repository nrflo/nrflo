-- Extractor tiers run cheap models and can miss things, but nothing in the
-- t0 templates nudged the caller to verify audit-critical claims — a single
-- extraction fanout was routinely treated as the final answer. Append a
-- verification-pass bullet after 000234's delegation-contract bullet.

UPDATE default_templates
SET template = REPLACE(template,
	'Never poll, never block on waits, never spawn timer delegations; act only when notified.',
	'Never poll, never block on waits, never spawn timer delegations; act only when notified.
- For audit/verification briefs, do not treat one extraction fanout as the final answer: follow it with a second pass that adversarially re-checks each positive claim (a fresh extractor per claim, briefed to refute it), or delegate the synthesis-critical claims to an executor-tier worker. Skip the second pass only for low-stakes lookups.'),
    default_template = REPLACE(default_template,
	'Never poll, never block on waits, never spawn timer delegations; act only when notified.',
	'Never poll, never block on waits, never spawn timer delegations; act only when notified.
- For audit/verification briefs, do not treat one extraction fanout as the final answer: follow it with a second pass that adversarially re-checks each positive claim (a fresh extractor per claim, briefed to refute it), or delegate the synthesis-critical claims to an executor-tier worker. Skip the second pass only for low-stakes lookups.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
