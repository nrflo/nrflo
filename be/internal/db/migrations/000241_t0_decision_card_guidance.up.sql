-- Audit of four kdre console sessions: roughly half of every human turn was
-- repair, not direction — the owner pasting the decider's own sentence back to
-- say which buried question they were answering, or asking what an unexplained
-- label meant. The t0 profiles now keep the AskUserQuestion native tool
-- (console_engine_claude.go), so the ask has a real surface; tell them to use
-- it instead of trailing prose. Appended after 000235's verification bullet.

UPDATE default_templates
SET template = REPLACE(template,
	'Skip the second pass only for low-stakes lookups.',
	'Skip the second pass only for low-stakes lookups.
- When you need a decision from the owner, ask with the AskUserQuestion tool — never as prose at the end of a long message, which forces them to quote your own sentence back to answer it. One tool call per decision point, options phrased so the tradeoff is legible without rereading the message above.
- Never put a bare label in front of the owner. Any candidate, ticket, or option you name is unusable until you have said what it is in one sentence; if it is not worth a sentence, it is not worth listing.'),
    default_template = REPLACE(default_template,
	'Skip the second pass only for low-stakes lookups.',
	'Skip the second pass only for low-stakes lookups.
- When you need a decision from the owner, ask with the AskUserQuestion tool — never as prose at the end of a long message, which forces them to quote your own sentence back to answer it. One tool call per decision point, options phrased so the tradeoff is legible without rereading the message above.
- Never put a bare label in front of the owner. Any candidate, ticket, or option you name is unusable until you have said what it is in one sentence; if it is not worth a sentence, it is not worth listing.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
