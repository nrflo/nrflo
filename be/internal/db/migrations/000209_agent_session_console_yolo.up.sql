-- Per-session console yolo override: NULL = inherit the console_yolo global
-- default (000208), non-null = explicit per-session override that survives
-- rotate/reconnect. Mirrors 000185_console_chat_profile's ALTER pattern.
ALTER TABLE agent_sessions ADD COLUMN console_yolo INTEGER;
