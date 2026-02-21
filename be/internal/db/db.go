package db

import (
	"database/sql"
	"fmt"
	"net/url"
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

// Querier is the common interface satisfied by both *DB and *Pool.
// Repos that don't need pool-specific features should accept this.
type Querier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Begin() (*sql.Tx, error)
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

// OpenPath opens a database at a specific path, sets PRAGMAs, and runs migrations.
func OpenPath(path string) (*DB, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := setDatabasePragmas(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{DB: db, Path: path}, nil
}

// OpenPathExisting opens an already-migrated database, skipping migrations.
// Use when copying from a template DB in tests.
func OpenPathExisting(path string) (*DB, error) {
	db, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := setDatabasePragmas(db); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{DB: db, Path: path}, nil
}

// OpenOrCreate opens an existing database or creates a new one.
func OpenOrCreate(customPath string) (*DB, error) {
	dbPath := GetDBPath(customPath)

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	return OpenPath(dbPath)
}

// buildDSN returns a DSN with per-connection pragmas (busy_timeout, foreign_keys).
// These are set via _pragma in the DSN so every pooled connection gets them,
// not just the first one (which is what happens with Exec-based PRAGMA calls).
func buildDSN(path string) string {
	v := url.Values{}
	v.Add("_pragma", "busy_timeout(10000)")
	v.Add("_pragma", "foreign_keys(1)")
	return "file:" + path + "?" + v.Encode()
}

// setDatabasePragmas sets database-level pragmas (WAL mode) that only need to run once.
func setDatabasePragmas(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	return nil
}

// WrapAsPool wraps a DB as a Pool for use with services that expect *Pool.
// This is useful when HTTP handlers need to use service layer methods.
func WrapAsPool(database *DB) *Pool {
	return &Pool{DB: database.DB, Path: database.Path}
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
