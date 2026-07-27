-- Convert the delegate tier seeds' `timeout` from seconds to minutes.
--
-- system_agent_definitions.timeout / agent_definitions.timeout is MINUTES:
-- spawner_prepare and spawner_script both turn it into the child process
-- budget with time.Minute, and the agent form labels the input "Timeout
-- (minutes)". Two spawn wrappers open-coded the conversion in seconds
-- instead (orchestrator/planner.go, spawner/delegate.go), so their context
-- deadline fired 60x earlier than the process budget derived from the same
-- value -- the outer deadline wins, and the child was killed mid-turn. Both
-- now go through spawner.SpawnDeadline, which is minutes.
--
-- 000182 authored these two values against the seconds reading (300 = 5min,
-- 1800 = 30min), so they need rescaling to survive the fix with their
-- intended wall time. Matching on the exact seeded value leaves any
-- operator-edited timeout alone: that was typed into a form labelled
-- minutes, so the fix already gives it the meaning its author intended.
--
-- planner-system / planner-system-api (10) and the dynamic-planner
-- agent_definitions row (30) need no rescaling -- those were authored as
-- minutes and were simply being misread.

UPDATE system_agent_definitions SET timeout = 5,  updated_at = datetime('now')
 WHERE id = '_t2_extractor' AND timeout = 300;

UPDATE system_agent_definitions SET timeout = 30, updated_at = datetime('now')
 WHERE id = '_t1_executor' AND timeout = 1800;
