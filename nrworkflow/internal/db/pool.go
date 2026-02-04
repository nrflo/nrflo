package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Pool wraps a connection-pooled database
type Pool struct {
	*sql.DB
	Path string
}

// PoolConfig configures the connection pool
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultPoolConfig returns the default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// NewPool creates a new connection pool
func NewPool(customPath string, config PoolConfig) (*Pool, error) {
	dbPath := GetDBPath(customPath)
	return NewPoolPath(dbPath, config)
}

// NewPoolPath creates a new connection pool at a specific path
func NewPoolPath(path string, config PoolConfig) (*Pool, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Check if database exists, create if not
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create new database
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("failed to create database: %w", err)
		}

		// Enable WAL mode for concurrent access
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}

		// Enable foreign keys
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}

		pool := &Pool{DB: db, Path: path}

		// Initialize schema
		if err := pool.initSchema(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}

		// Apply pool settings
		pool.applyConfig(config)

		return pool, nil
	}

	// Open existing database
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	pool := &Pool{DB: db, Path: path}

	// Run migrations
	if err := pool.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Apply pool settings
	pool.applyConfig(config)

	return pool, nil
}

// applyConfig applies pool configuration
func (p *Pool) applyConfig(config PoolConfig) {
	p.SetMaxOpenConns(config.MaxOpenConns)
	p.SetMaxIdleConns(config.MaxIdleConns)
	p.SetConnMaxLifetime(config.ConnMaxLifetime)
}

// initSchema creates the initial database schema
func (p *Pool) initSchema() error {
	schema := `
-- Enable foreign keys
PRAGMA foreign_keys = ON;

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
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'timeout')),
    last_messages TEXT,
    message_stats TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);

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
`
	_, err := p.Exec(schema)
	return err
}

// runMigrations ensures all tables exist (for existing databases)
func (p *Pool) runMigrations() error {
	// Migration: Add projects table if it doesn't exist
	_, err := p.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			root_path TEXT,
			default_workflow TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Migration: Add agent_sessions table if it doesn't exist
	_, err = p.Exec(`
		CREATE TABLE IF NOT EXISTS agent_sessions (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			ticket_id TEXT NOT NULL,
			phase TEXT NOT NULL,
			workflow TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			model_id TEXT,
			status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'timeout')),
			last_messages TEXT,
			message_stats TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
		CREATE INDEX IF NOT EXISTS idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
	`)
	if err != nil {
		return err
	}

	// Migration: Add message_stats column if it doesn't exist
	_, _ = p.Exec(`ALTER TABLE agent_sessions ADD COLUMN message_stats TEXT`)
	// Ignore error if column already exists

	// Migration: Add spawn_command and prompt_context columns if they don't exist
	_, _ = p.Exec(`ALTER TABLE agent_sessions ADD COLUMN spawn_command TEXT`)
	_, _ = p.Exec(`ALTER TABLE agent_sessions ADD COLUMN prompt_context TEXT`)
	// Ignore errors if columns already exist

	return nil
}

// SetConfig sets a configuration value
func (p *Pool) SetConfig(key, value string) error {
	_, err := p.Exec(
		"INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)",
		key, value,
	)
	return err
}

// GetConfig gets a configuration value
func (p *Pool) GetConfig(key string) (string, error) {
	var value string
	err := p.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Health checks if the database is healthy
func (p *Pool) Health() error {
	return p.Ping()
}
