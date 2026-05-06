CREATE TABLE agent_messages_backup (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    seq        INTEGER NOT NULL,
    content    TEXT NOT NULL,
    category   TEXT NOT NULL DEFAULT 'text',
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
);
INSERT INTO agent_messages_backup SELECT id, session_id, seq, content, category, created_at FROM agent_messages;
DROP TABLE agent_messages;
ALTER TABLE agent_messages_backup RENAME TO agent_messages;
CREATE INDEX IF NOT EXISTS idx_agent_messages_session ON agent_messages(session_id, seq);
