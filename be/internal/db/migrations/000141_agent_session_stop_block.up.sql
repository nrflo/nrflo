-- Number of times the Stop hook has blocked (force-continued) an autonomous
-- session that ended a turn without calling a completion tool. Once it exceeds
-- the server cap, the session is failed (unresponsive_after_stop_blocks) instead.
ALTER TABLE agent_sessions ADD COLUMN stop_block_count INTEGER NOT NULL DEFAULT 0;
