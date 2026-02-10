package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"nrworkflow/internal/db/migrations"
)

// RunMigrations applies all pending migrations to the database.
// It handles three cases:
//  1. Fresh DB (no tables) — golang-migrate runs from scratch
//  2. Existing DB already managed by golang-migrate — runs pending migrations
//  3. Legacy DB (tables exist but no schema_migrations) — bootstraps version tracking, then runs
//
// IMPORTANT: Do NOT call m.Close() — it closes the underlying *sql.DB which we still need.
func RunMigrations(sqlDB *sql.DB) error {
	if err := bootstrap(sqlDB); err != nil {
		return fmt.Errorf("migration bootstrap: %w", err)
	}

	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{
		DatabaseName: "main",
		NoTxWrap:     true,
	})
	if err != nil {
		return fmt.Errorf("migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up: %w", err)
	}

	return nil
}

// bootstrap handles existing databases that predate the migration system.
// If tables exist but schema_migrations doesn't, it stamps version 1
// so golang-migrate knows the initial schema is already applied.
func bootstrap(sqlDB *sql.DB) error {
	// Check if schema_migrations table exists (already managed by golang-migrate)
	var smExists int
	err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&smExists)
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if smExists > 0 {
		return nil // already managed
	}

	// Check if any application tables exist (legacy DB)
	var tableCount int
	err = sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('projects', 'tickets', 'agent_sessions')`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("check tables: %w", err)
	}
	if tableCount == 0 {
		return nil // fresh DB, golang-migrate will handle everything
	}

	// Legacy DB: clean up and stamp version 1
	// Drop legacy last_messages column if it exists (ignore errors)
	_, _ = sqlDB.Exec(`ALTER TABLE agent_sessions DROP COLUMN last_messages`)

	// Create schema_migrations table and stamp version 1 (not dirty)
	_, err = sqlDB.Exec(`CREATE TABLE schema_migrations (version uint64 not null primary key, dirty boolean not null)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	_, err = sqlDB.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (1, false)`)
	if err != nil {
		return fmt.Errorf("stamp version: %w", err)
	}

	return nil
}

// MigrationVersion returns the current migration version and dirty state.
func MigrationVersion(sqlDB *sql.DB) (version uint, dirty bool, err error) {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return 0, false, fmt.Errorf("migration source: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{
		DatabaseName: "main",
		NoTxWrap:     true,
	})
	if err != nil {
		return 0, false, fmt.Errorf("migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return 0, false, fmt.Errorf("migration init: %w", err)
	}

	v, d, err := m.Version()
	if err != nil {
		return 0, false, err
	}
	return v, d, nil
}

// MigrateDown rolls back the last migration.
func MigrateDown(sqlDB *sql.DB) error {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{
		DatabaseName: "main",
		NoTxWrap:     true,
	})
	if err != nil {
		return fmt.Errorf("migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("migration down: %w", err)
	}
	return nil
}

// MigrateForce force-sets the migration version without running migrations.
// Useful for fixing a dirty database state.
func MigrateForce(sqlDB *sql.DB, version int) error {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(sqlDB, &sqlite.Config{
		DatabaseName: "main",
		NoTxWrap:     true,
	})
	if err != nil {
		return fmt.Errorf("migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}

	if err := m.Force(version); err != nil {
		return fmt.Errorf("migration force: %w", err)
	}
	return nil
}
