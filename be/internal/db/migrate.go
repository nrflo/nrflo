package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"be/internal/db/migrations"
)

// RunMigrations applies all pending migrations to the database.
//
// IMPORTANT: Do NOT call m.Close() — it closes the underlying *sql.DB which we still need.
func RunMigrations(sqlDB *sql.DB) error {
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
