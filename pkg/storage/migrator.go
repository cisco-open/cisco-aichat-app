// Copyright 2025 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrationsFS embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrationsFS embed.FS

// DBType represents supported database types
type DBType string

const (
	DBTypeSQLite   DBType = "sqlite"
	DBTypePostgres DBType = "postgres"
)

// Migrator handles database migrations using golang-migrate
type Migrator struct {
	dbType DBType
	dbURL  string
	logger log.Logger
}

// ErrInvalidDBType is returned when an unsupported database type is specified
var ErrInvalidDBType = errors.New("invalid database type: must be 'sqlite' or 'postgres'")

// NewMigrator creates a new Migrator instance
func NewMigrator(dbType, dbURL string, logger log.Logger) (*Migrator, error) {
	dt := DBType(dbType)
	if dt != DBTypeSQLite && dt != DBTypePostgres {
		return nil, fmt.Errorf("%w: got '%s'", ErrInvalidDBType, dbType)
	}

	return &Migrator{
		dbType: dt,
		dbURL:  dbURL,
		logger: logger,
	}, nil
}

// getMigrateInstance creates a new migrate instance for the configured database
func (m *Migrator) getMigrateInstance() (*migrate.Migrate, error) {
	var fs embed.FS
	var sourcePath string

	switch m.dbType {
	case DBTypeSQLite:
		fs = sqliteMigrationsFS
		sourcePath = "migrations/sqlite"
	case DBTypePostgres:
		fs = postgresMigrationsFS
		sourcePath = "migrations/postgres"
	default:
		return nil, ErrInvalidDBType
	}

	// Create source from embedded filesystem
	source, err := iofs.New(fs, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	// Create migrate instance
	// Note: Caller must ensure database driver is imported (e.g., _ "github.com/golang-migrate/migrate/v4/database/sqlite")
	instance, err := migrate.NewWithSourceInstance("iofs", source, m.dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return instance, nil
}

// RunMigrations runs all pending migrations
// This is the primary method for automatic migrations on startup
func (m *Migrator) RunMigrations() error {
	m.logger.Info("Starting database migrations", "dbType", m.dbType)

	instance, err := m.getMigrateInstance()
	if err != nil {
		return err
	}
	defer instance.Close()

	if err := instance.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			m.logger.Info("Database schema is up to date")
			return nil
		}
		// Check for dirty state
		version, dirty, verr := instance.Version()
		if verr == nil && dirty {
			m.logger.Error("Database is in dirty state - manual intervention required",
				"version", version,
				"error", err)
			return fmt.Errorf("database is in dirty state at version %d: %w (manual intervention required)", version, err)
		}
		m.logger.Error("Migration failed", "error", err)
		return fmt.Errorf("migration failed: %w", err)
	}

	version, _, _ := instance.Version()
	m.logger.Info("Database migrations completed", "version", version)
	return nil
}

// MigrateUp runs all pending up migrations
func (m *Migrator) MigrateUp() error {
	instance, err := m.getMigrateInstance()
	if err != nil {
		return err
	}
	defer instance.Close()

	if err := instance.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up failed: %w", err)
	}
	return nil
}

// MigrateDown rolls back all migrations
// WARNING: This will drop all tables and data. Use only for testing.
func (m *Migrator) MigrateDown() error {
	instance, err := m.getMigrateInstance()
	if err != nil {
		return err
	}
	defer instance.Close()

	if err := instance.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down failed: %w", err)
	}
	return nil
}

// Version returns the current migration version and dirty state
func (m *Migrator) Version() (uint, bool, error) {
	instance, err := m.getMigrateInstance()
	if err != nil {
		return 0, false, err
	}
	defer instance.Close()

	return instance.Version()
}
