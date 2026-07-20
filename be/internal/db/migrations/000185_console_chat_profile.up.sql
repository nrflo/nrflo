-- Persist the console-chat profile (t0-decider, t0-hands, or '' for a chat
-- created before profiles existed / with no profile) on the session row, so
-- catalog/detail and sibling-flow gating don't need the in-memory session map.
ALTER TABLE agent_sessions ADD COLUMN console_profile TEXT NOT NULL DEFAULT '';
