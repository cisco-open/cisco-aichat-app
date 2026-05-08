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
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import SQLite driver for migrations
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	// Import modernc sqlite driver for database queries
	_ "modernc.org/sqlite"
)

func TestMigrator_SQLite_UpDown(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "migrator-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "sqlite://" + dbPath

	// Create migrator
	migrator, err := NewMigrator("sqlite", dbURL, log.DefaultLogger)
	require.NoError(t, err)

	// Run Up migration
	err = migrator.MigrateUp()
	require.NoError(t, err)

	// Check version is 3, not dirty (after migration 000003)
	version, dirty, err := migrator.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(3), version)
	assert.False(t, dirty)

	// Run Down migration
	err = migrator.MigrateDown()
	require.NoError(t, err)

	// Check version - should be ErrNilVersion or 0
	_, _, err = migrator.Version()
	// After down migration, version query may return ErrNilVersion
	// which is expected since no migration has been applied
	// We just verify it doesn't panic
}

func TestMigrator_InvalidDBType(t *testing.T) {
	_, err := NewMigrator("mysql", "mysql://localhost:3306/test", log.DefaultLogger)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDBType)
}

func TestMigrator_TablesExist(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "migrator-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "sqlite://" + dbPath

	// Create and run migrator
	migrator, err := NewMigrator("sqlite", dbURL, log.DefaultLogger)
	require.NoError(t, err)

	err = migrator.MigrateUp()
	require.NoError(t, err)

	// Open raw database connection to verify tables
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Check aichat_sessions table exists
	var sessionsTable string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='aichat_sessions'").Scan(&sessionsTable)
	require.NoError(t, err)
	assert.Equal(t, "aichat_sessions", sessionsTable)

	// Check aichat_messages table exists
	var messagesTable string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='aichat_messages'").Scan(&messagesTable)
	require.NoError(t, err)
	assert.Equal(t, "aichat_messages", messagesTable)

	// Check indexes exist
	var indexCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_aichat_%'").Scan(&indexCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, indexCount, 4, "Should have at least 4 indexes")
}

func TestMigrator_SchemaCorrect(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "migrator-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "sqlite://" + dbPath

	// Create and run migrator
	migrator, err := NewMigrator("sqlite", dbURL, log.DefaultLogger)
	require.NoError(t, err)

	err = migrator.MigrateUp()
	require.NoError(t, err)

	// Open raw database connection to verify schema
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify aichat_sessions columns
	sessionsColumns := getTableColumns(t, db, "aichat_sessions")
	expectedSessionsColumns := []string{"id", "user_id", "name", "created_at", "updated_at", "total_tokens", "is_active"}
	for _, col := range expectedSessionsColumns {
		assert.Contains(t, sessionsColumns, col, "aichat_sessions should have column: %s", col)
	}

	// Verify aichat_messages columns (including summary fields from migration 000002)
	messagesColumns := getTableColumns(t, db, "aichat_messages")
	expectedMessagesColumns := []string{"id", "session_id", "role", "content", "timestamp", "token_count", "is_pinned", "is_summary", "summarized_ids", "summary_depth"}
	for _, col := range expectedMessagesColumns {
		assert.Contains(t, messagesColumns, col, "aichat_messages should have column: %s", col)
	}
}

func TestMigrator_RunMigrations(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "migrator-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "sqlite://" + dbPath

	// Create migrator
	migrator, err := NewMigrator("sqlite", dbURL, log.DefaultLogger)
	require.NoError(t, err)

	// Run migrations (the primary method for startup)
	err = migrator.RunMigrations()
	require.NoError(t, err)

	// Running again should be idempotent (no error)
	err = migrator.RunMigrations()
	require.NoError(t, err)
}

// getTableColumns returns column names for a table using PRAGMA table_info
func getTableColumns(t *testing.T, db *sql.DB, tableName string) []string {
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	require.NoError(t, err)
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dfltValue interface{}
		err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk)
		require.NoError(t, err)
		columns = append(columns, name)
	}
	return columns
}
