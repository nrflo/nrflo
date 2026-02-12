-- Config table
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    root_path TEXT,
    default_workflow TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Tickets table (with project_id)
CREATE TABLE IF NOT EXISTS tickets (
    id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'closed')),
    priority INTEGER NOT NULL DEFAULT 2,
    issue_type TEXT NOT NULL DEFAULT 'task' CHECK (issue_type IN ('bug', 'feature', 'task', 'epic')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    created_by TEXT NOT NULL,
    close_reason TEXT,
    agents_state TEXT,
    PRIMARY KEY (project_id, id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tickets_project ON tickets(project_id);

-- Dependencies table (with project_id)
CREATE TABLE IF NOT EXISTS dependencies (
    project_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    depends_on_id TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'blocks',
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL,
    PRIMARY KEY (project_id, issue_id, depends_on_id),
    FOREIGN KEY (project_id, issue_id) REFERENCES tickets(project_id, id) ON DELETE CASCADE
);

-- Agent sessions table (with project_id)
CREATE TABLE IF NOT EXISTS agent_sessions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    phase TEXT NOT NULL,
    workflow TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    model_id TEXT,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued')),
    context_left INTEGER,
    ancestor_session_id TEXT,
    spawn_command TEXT,
    prompt_context TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);

-- Workflows table (project-scoped)
CREATE TABLE IF NOT EXISTS workflows (
    id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    description TEXT,
    categories TEXT,
    phases TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (project_id, id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_workflows_project ON workflows(project_id);

-- Agent messages table (normalized from last_messages column)
CREATE TABLE IF NOT EXISTS agent_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_agent_messages_session ON agent_messages(session_id, seq);

-- FTS5 for search (includes project_id)
CREATE VIRTUAL TABLE IF NOT EXISTS tickets_fts USING fts5(
    project_id, id, title, description,
    content='tickets', content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS tickets_ai AFTER INSERT ON tickets BEGIN
    INSERT INTO tickets_fts(rowid, project_id, id, title, description)
    VALUES (NEW.rowid, NEW.project_id, NEW.id, NEW.title, NEW.description);
END;

CREATE TRIGGER IF NOT EXISTS tickets_ad AFTER DELETE ON tickets BEGIN
    INSERT INTO tickets_fts(tickets_fts, rowid, project_id, id, title, description)
    VALUES('delete', OLD.rowid, OLD.project_id, OLD.id, OLD.title, OLD.description);
END;

CREATE TRIGGER IF NOT EXISTS tickets_au AFTER UPDATE ON tickets BEGIN
    INSERT INTO tickets_fts(tickets_fts, rowid, project_id, id, title, description)
    VALUES('delete', OLD.rowid, OLD.project_id, OLD.id, OLD.title, OLD.description);
    INSERT INTO tickets_fts(rowid, project_id, id, title, description)
    VALUES (NEW.rowid, NEW.project_id, NEW.id, NEW.title, NEW.description);
END;
