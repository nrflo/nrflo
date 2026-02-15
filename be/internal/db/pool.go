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

// NewPoolPath creates a new connection pool at a specific path.
// Handles both fresh and existing databases via golang-migrate.
func NewPoolPath(path string, config PoolConfig) (*Pool, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

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

	pool := &Pool{DB: db, Path: path}
	pool.applyConfig(config)

	return pool, nil
}

// applyConfig applies pool configuration
func (p *Pool) applyConfig(config PoolConfig) {
	p.SetMaxOpenConns(config.MaxOpenConns)
	p.SetMaxIdleConns(config.MaxIdleConns)
	p.SetConnMaxLifetime(config.ConnMaxLifetime)
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
