package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const DBFileName = "nrworkflow.data"

// DefaultDBPath returns the default database path in the nrworkflow installation directory.
// Uses NRWORKFLOW_HOME env var if set, otherwise defaults to ~/projects/2026/nrworkflow.
// This is a single centralized database for all projects.
func DefaultDBPath() string {
	// Check for NRWORKFLOW_HOME environment variable
	if nrHome := os.Getenv("NRWORKFLOW_HOME"); nrHome != "" {
		return filepath.Join(nrHome, DBFileName)
	}

	// Default to ~/projects/2026/nrworkflow
	home, err := os.UserHomeDir()
	if err != nil {
		return DBFileName
	}
	return filepath.Join(home, "projects", "2026", "nrworkflow", DBFileName)
}

// DB wraps the database connection
type DB struct {
	*sql.DB
	Path string
}

// GetDBPath returns the database path from flag or default
func GetDBPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	return DefaultDBPath()
}

// Open opens an existing database (uses custom path if provided, otherwise default)
func Open(customPath string) (*DB, error) {
	dbPath := GetDBPath(customPath)
	return OpenPath(dbPath)
}

// OpenPath opens a database at a specific path
func OpenPath(path string) (*DB, error) {
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

	wrappedDB := &DB{DB: db, Path: path}

	// Run migrations for existing databases
	if err := wrappedDB.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return wrappedDB, nil
}

// runMigrations ensures all tables exist (for existing databases)
func (db *DB) runMigrations() error {
	// Migration: Add projects table if it doesn't exist
	_, err := db.Exec(`
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
	_, err = db.Exec(`
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
	_, _ = db.Exec(`ALTER TABLE agent_sessions ADD COLUMN message_stats TEXT`)
	// Ignore error if column already exists

	// Migration: Add spawn_command and prompt_context columns if they don't exist
	_, _ = db.Exec(`ALTER TABLE agent_sessions ADD COLUMN spawn_command TEXT`)
	_, _ = db.Exec(`ALTER TABLE agent_sessions ADD COLUMN prompt_context TEXT`)
	// Ignore errors if columns already exist

	return nil
}

// Create creates a new database (uses custom path if provided, otherwise default)
func Create(customPath string) (*DB, error) {
	dbPath := GetDBPath(customPath)

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if already exists
	if _, err := os.Stat(dbPath); err == nil {
		return nil, fmt.Errorf("database already exists at %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Enable WAL mode for concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	return &DB{DB: db, Path: dbPath}, nil
}

// OpenOrCreate opens an existing database or creates a new one
func OpenOrCreate(customPath string) (*DB, error) {
	dbPath := GetDBPath(customPath)

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Create new database
		database, err := Create(customPath)
		if err != nil {
			return nil, err
		}
		if err := database.InitSchema(); err != nil {
			database.Close()
			return nil, err
		}
		return database, nil
	}

	// Open existing database
	return Open(customPath)
}

// InitSchema initializes the database schema
func (db *DB) InitSchema() error {
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
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// SetConfig sets a configuration value
func (db *DB) SetConfig(key, value string) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)",
		key, value,
	)
	return err
}

// GetConfig gets a configuration value
func (db *DB) GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}
