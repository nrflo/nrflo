-- Real executor work packages against a mid-size codebase run 25-30 min and
-- flirt with the 30-min budget (observed: a ~30-min package killed at
-- result=fail/timeout after $26 of work, forcing a re-delegation that starts
-- over from the brief). Raise the executor tier's seeded timeout to 60 min.
-- Matching the exact seeded value leaves any operator-edited timeout alone
-- (same guard 000212 used).

UPDATE system_agent_definitions SET timeout = 60, updated_at = datetime('now')
 WHERE id = '_t1_executor' AND timeout = 30;
