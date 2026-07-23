-- agent_step_cursors gains a durable per-step rejection counter: a JSON map
-- step_id -> int, incremented by the complete_step builtin whenever a
-- rejection counts toward the evidence cap (missing/invalid evidence, a
-- failed check — never a guard-miss like stale_revision/step_mismatch).
-- This is cursor state, not session state, because it must survive a
-- session rotation/retry mid-step: a fresh session resuming the same
-- (workflow_instance_id, node_id) cursor must not reset the count back to
-- zero and re-earn a full evidence-exhaustion budget.
ALTER TABLE agent_step_cursors ADD COLUMN rejections TEXT NOT NULL DEFAULT '{}';
