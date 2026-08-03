-- Durable console sibling->origin link. Today ChatService.openSibling only
-- pushes a transient console_chat.sibling_opened WS event; this column lets
-- the flow graph reconstruct sibling chats (model_switch/hands_sibling)
-- after the fact, from a sibling row back to the origin session it was
-- opened from. Empty = not a sibling (ordinary chat).
ALTER TABLE agent_sessions ADD COLUMN sibling_origin_session_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_agent_sessions_sibling_origin ON agent_sessions (sibling_origin_session_id);
