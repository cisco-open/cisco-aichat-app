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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	// Database drivers
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	// Migrate drivers (required for golang-migrate)
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
)

var (
	// Prometheus metrics for database operations
	dbOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "aichat",
			Name:      "db_operations_total",
			Help:      "Total number of database operations",
		},
		[]string{"operation", "status"},
	)

	dbOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "aichat",
			Name:      "db_operation_duration_seconds",
			Help:      "Duration of database operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
)

// DBStorage implements Storage interface using SQL databases (SQLite or PostgreSQL)
type DBStorage struct {
	db     *sql.DB
	dbType string // "sqlite" or "postgres"
	logger log.Logger
	mu     sync.RWMutex // For connection state protection
}

// serializeSummarizedIDs converts a string slice to JSON for SQLite storage
func serializeSummarizedIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	data, _ := json.Marshal(ids)
	return string(data)
}

// deserializeSummarizedIDs converts a JSON string or null to string slice
func deserializeSummarizedIDs(data sql.NullString) []string {
	if !data.Valid || data.String == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(data.String), &ids); err != nil {
		return nil
	}
	return ids
}

// NewDBStorage creates a new database-backed storage instance
// dbURL format:
//   - SQLite: "file:/path/to/db.db" or "/path/to/db.db"
//   - PostgreSQL: "postgres://user:pass@host:port/dbname" or "postgresql://..."
func NewDBStorage(dbURL string, logger log.Logger) (*DBStorage, error) {
	// Detect database type from URL
	dbType, driverName, connStr := parseDBURL(dbURL)
	if dbType == "" {
		return nil, fmt.Errorf("unsupported database URL format: %s", dbURL)
	}

	logger.Info("Initializing database storage", "dbType", dbType)

	// Open database connection
	db, err := sql.Open(driverName, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool based on database type
	switch dbType {
	case "sqlite":
		// SQLite: Single writer, avoid SQLITE_BUSY errors
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(0) // Don't close idle connections
	case "postgres":
		// PostgreSQL: Allow concurrent connections
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run migrations
	migratorURL := buildMigratorURL(dbType, dbURL)
	migrator, err := NewMigrator(dbType, migratorURL, logger)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	if err := migrator.RunMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DBStorage{
		db:     db,
		dbType: dbType,
		logger: logger,
	}, nil
}

// parseDBURL determines database type and returns driver name and connection string
func parseDBURL(dbURL string) (dbType, driverName, connStr string) {
	switch {
	case strings.HasPrefix(dbURL, "postgres://") || strings.HasPrefix(dbURL, "postgresql://"):
		return "postgres", "pgx", dbURL
	case strings.HasPrefix(dbURL, "file:"):
		// SQLite file: URL - add WAL mode and foreign keys
		connStr = dbURL
		if !strings.Contains(connStr, "_pragma") {
			if strings.Contains(connStr, "?") {
				connStr += "&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
			} else {
				connStr += "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
			}
		}
		return "sqlite", "sqlite", connStr
	case strings.HasSuffix(dbURL, ".db") || strings.HasSuffix(dbURL, ".sqlite"):
		// Plain file path for SQLite
		connStr = "file:" + dbURL + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
		return "sqlite", "sqlite", connStr
	default:
		return "", "", ""
	}
}

// buildMigratorURL constructs the URL format expected by golang-migrate
func buildMigratorURL(dbType, dbURL string) string {
	switch dbType {
	case "sqlite":
		// golang-migrate expects "sqlite://path" format
		path := strings.TrimPrefix(dbURL, "file:")
		// Remove any query parameters for the base path
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		return "sqlite://" + path
	case "postgres":
		return dbURL
	default:
		return dbURL
	}
}

// recordMetric records operation metrics
func (ds *DBStorage) recordMetric(operation string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	dbOperationDuration.WithLabelValues(operation).Observe(duration)

	status := "success"
	if err != nil {
		status = "error"
	}
	dbOperationsTotal.WithLabelValues(operation, status).Inc()
}

type operationMetric struct {
	ds        *DBStorage
	operation string
	start     time.Time
	once      sync.Once
}

func (ds *DBStorage) newOperationMetric(operation string, start time.Time) *operationMetric {
	return &operationMetric{
		ds:        ds,
		operation: operation,
		start:     start,
	}
}

func (m *operationMetric) Record(err error) {
	m.once.Do(func() {
		m.ds.recordMetric(m.operation, m.start, err)
	})
}

// GetSessions returns all sessions for a user (without messages for performance)
func (ds *DBStorage) GetSessions(ctx context.Context, userID string) ([]ChatSession, error) {
	start := time.Now()
	metric := ds.newOperationMetric("get_sessions", start)
	defer func() { metric.Record(nil) }()

	query := `SELECT id, user_id, name, created_at, updated_at, total_tokens, is_active
	          FROM aichat_sessions
	          WHERE user_id = $1
	          ORDER BY updated_at DESC`

	rows, err := ds.db.QueryContext(ctx, query, userID)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []ChatSession
	for rows.Next() {
		var s ChatSession
		var createdAt, updatedAt int64
		var isActive int

		if ds.dbType == "postgres" {
			// PostgreSQL: timestamps are TIMESTAMPTZ, need to scan as time.Time
			var createdTime, updatedTime time.Time
			var isActiveBool bool
			err = rows.Scan(&s.ID, &s.UserID, &s.Name, &createdTime, &updatedTime, &s.TotalTokens, &isActiveBool)
			if err != nil {
				metric.Record(err)
				return nil, fmt.Errorf("failed to scan session: %w", err)
			}
			createdAt = createdTime.UnixMilli()
			updatedAt = updatedTime.UnixMilli()
			if isActiveBool {
				isActive = 1
			}
		} else {
			// SQLite: timestamps are INTEGER (milliseconds)
			err = rows.Scan(&s.ID, &s.UserID, &s.Name, &createdAt, &updatedAt, &s.TotalTokens, &isActive)
			if err != nil {
				metric.Record(err)
				return nil, fmt.Errorf("failed to scan session: %w", err)
			}
		}

		s.CreatedAt = createdAt
		s.UpdatedAt = updatedAt
		s.IsActive = isActive != 0
		s.Messages = []ChatMessage{} // Lazy load messages
		sessions = append(sessions, s)
	}

	if err := rows.Err(); err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// GetSession returns a specific session with all its messages
func (ds *DBStorage) GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error) {
	start := time.Now()
	metric := ds.newOperationMetric("get_session", start)
	defer func() { metric.Record(nil) }()

	// Get session
	sessionQuery := `SELECT id, user_id, name, created_at, updated_at, total_tokens, is_active
	                 FROM aichat_sessions
	                 WHERE id = $1 AND user_id = $2`

	var session ChatSession
	var createdAt, updatedAt int64
	var isActive int

	if ds.dbType == "postgres" {
		var createdTime, updatedTime time.Time
		var isActiveBool bool
		err := ds.db.QueryRowContext(ctx, sessionQuery, sessionID, userID).Scan(
			&session.ID, &session.UserID, &session.Name, &createdTime, &updatedTime, &session.TotalTokens, &isActiveBool,
		)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to query session: %w", err)
		}
		createdAt = createdTime.UnixMilli()
		updatedAt = updatedTime.UnixMilli()
		if isActiveBool {
			isActive = 1
		}
	} else {
		err := ds.db.QueryRowContext(ctx, sessionQuery, sessionID, userID).Scan(
			&session.ID, &session.UserID, &session.Name, &createdAt, &updatedAt, &session.TotalTokens, &isActive,
		)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to query session: %w", err)
		}
	}

	session.CreatedAt = createdAt
	session.UpdatedAt = updatedAt
	session.IsActive = isActive != 0

	// Get messages
	msgQuery := `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
	             FROM aichat_messages
	             WHERE session_id = $1
	             ORDER BY timestamp ASC`

	rows, err := ds.db.QueryContext(ctx, msgQuery, sessionID)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	session.Messages = []ChatMessage{}
	for rows.Next() {
		var msg ChatMessage
		var isPinned, isSummary, summaryDepth int
		var summarizedIDs sql.NullString

		if ds.dbType == "postgres" {
			var isPinnedBool, isSummaryBool bool
			var pgSummarizedIDs pq.StringArray
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinnedBool, &isSummaryBool, &pgSummarizedIDs, &summaryDepth)
			if isPinnedBool {
				isPinned = 1
			}
			if isSummaryBool {
				isSummary = 1
			}
			msg.SummarizedIDs = pgSummarizedIDs
		} else {
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinned, &isSummary, &summarizedIDs, &summaryDepth)
			msg.SummarizedIDs = deserializeSummarizedIDs(summarizedIDs)
		}

		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		msg.IsPinned = isPinned != 0
		msg.IsSummary = isSummary != 0
		msg.SummaryDepth = summaryDepth
		session.Messages = append(session.Messages, msg)
	}

	session.Messages = FilterCompactedMessages(session.Messages)

	return &session, nil
}

// CreateSession creates a new session
func (ds *DBStorage) CreateSession(ctx context.Context, userID string, session *ChatSession) error {
	start := time.Now()
	metric := ds.newOperationMetric("create_session", start)
	defer func() { metric.Record(nil) }()

	session.UserID = userID

	var query string
	var args []interface{}

	if ds.dbType == "postgres" {
		query = `INSERT INTO aichat_sessions (id, user_id, name, created_at, updated_at, total_tokens, is_active)
		         VALUES ($1, $2, $3, to_timestamp($4::bigint / 1000.0), to_timestamp($5::bigint / 1000.0), $6, $7)`
		args = []interface{}{session.ID, userID, session.Name, session.CreatedAt, session.UpdatedAt, session.TotalTokens, session.IsActive}
	} else {
		query = `INSERT INTO aichat_sessions (id, user_id, name, created_at, updated_at, total_tokens, is_active)
		         VALUES ($1, $2, $3, $4, $5, $6, $7)`
		isActive := 0
		if session.IsActive {
			isActive = 1
		}
		args = []interface{}{session.ID, userID, session.Name, session.CreatedAt, session.UpdatedAt, session.TotalTokens, isActive}
	}

	_, err := ds.db.ExecContext(ctx, query, args...)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// UpdateSession updates an existing session
func (ds *DBStorage) UpdateSession(ctx context.Context, userID string, session *ChatSession) error {
	start := time.Now()
	metric := ds.newOperationMetric("update_session", start)
	defer func() { metric.Record(nil) }()

	var query string
	var args []interface{}

	if ds.dbType == "postgres" {
		query = `UPDATE aichat_sessions
		         SET name = $1, updated_at = to_timestamp($2::bigint / 1000.0), total_tokens = $3, is_active = $4
		         WHERE id = $5 AND user_id = $6`
		args = []interface{}{session.Name, session.UpdatedAt, session.TotalTokens, session.IsActive, session.ID, userID}
	} else {
		query = `UPDATE aichat_sessions
		         SET name = $1, updated_at = $2, total_tokens = $3, is_active = $4
		         WHERE id = $5 AND user_id = $6`
		isActive := 0
		if session.IsActive {
			isActive = 1
		}
		args = []interface{}{session.Name, session.UpdatedAt, session.TotalTokens, isActive, session.ID, userID}
	}

	result, err := ds.db.ExecContext(ctx, query, args...)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	return nil
}

// DeleteSession deletes a session and its messages (via CASCADE)
func (ds *DBStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	start := time.Now()
	metric := ds.newOperationMetric("delete_session", start)
	defer func() { metric.Record(nil) }()

	query := `DELETE FROM aichat_sessions WHERE id = $1 AND user_id = $2`
	result, err := ds.db.ExecContext(ctx, query, sessionID, userID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// SetActiveSession sets a session as active and deactivates others for the user
func (ds *DBStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	start := time.Now()
	metric := ds.newOperationMetric("set_active_session", start)
	defer func() { metric.Record(nil) }()

	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate all sessions for user
	var deactivateQuery string
	if ds.dbType == "postgres" {
		deactivateQuery = `UPDATE aichat_sessions SET is_active = false WHERE user_id = $1`
	} else {
		deactivateQuery = `UPDATE aichat_sessions SET is_active = 0 WHERE user_id = $1`
	}

	_, err = tx.ExecContext(ctx, deactivateQuery, userID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to deactivate sessions: %w", err)
	}

	// Activate the specified session
	var activateQuery string
	if ds.dbType == "postgres" {
		activateQuery = `UPDATE aichat_sessions SET is_active = true WHERE id = $1 AND user_id = $2`
	} else {
		activateQuery = `UPDATE aichat_sessions SET is_active = 1 WHERE id = $1 AND user_id = $2`
	}

	result, err := tx.ExecContext(ctx, activateQuery, sessionID, userID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to activate session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if err := tx.Commit(); err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// AddMessage adds a message to a session
func (ds *DBStorage) AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	start := time.Now()
	metric := ds.newOperationMetric("add_message", start)
	defer func() { metric.Record(nil) }()

	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify session exists and belongs to user
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM aichat_sessions WHERE id = $1 AND user_id = $2)`
	err = tx.QueryRowContext(ctx, checkQuery, sessionID, userID).Scan(&exists)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Insert message
	var insertQuery string
	var args []interface{}

	if ds.dbType == "postgres" {
		insertQuery = `INSERT INTO aichat_messages (id, session_id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth)
		               VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		               ON CONFLICT (id) DO NOTHING`
		var summarizedIDs interface{}
		if len(message.SummarizedIDs) > 0 {
			summarizedIDs = pq.Array(message.SummarizedIDs)
		}
		args = []interface{}{message.ID, sessionID, message.Role, message.Content, message.Timestamp,
			message.TokenCount, message.IsPinned, message.IsSummary, summarizedIDs, message.SummaryDepth}
	} else {
		insertQuery = `INSERT OR IGNORE INTO aichat_messages (id, session_id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth)
		               VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
		isPinned := 0
		if message.IsPinned {
			isPinned = 1
		}
		isSummary := 0
		if message.IsSummary {
			isSummary = 1
		}
		args = []interface{}{message.ID, sessionID, message.Role, message.Content, message.Timestamp,
			message.TokenCount, isPinned, isSummary, serializeSummarizedIDs(message.SummarizedIDs), message.SummaryDepth}
	}

	insertResult, err := tx.ExecContext(ctx, insertQuery, args...)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to insert message: %w", err)
	}

	insertedRows, err := insertResult.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get insert rows affected: %w", err)
	}

	// Idempotent behavior: duplicate message IDs are ignored and should not mutate session totals.
	if insertedRows == 0 {
		if err := tx.Commit(); err != nil {
			metric.Record(err)
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		return nil
	}

	// Update session's updated_at and total_tokens
	var updateQuery string
	if ds.dbType == "postgres" {
		updateQuery = `UPDATE aichat_sessions
		               SET updated_at = to_timestamp($1::bigint / 1000.0), total_tokens = total_tokens + $2
		               WHERE id = $3`
	} else {
		updateQuery = `UPDATE aichat_sessions
		               SET updated_at = $1, total_tokens = total_tokens + $2
		               WHERE id = $3`
	}

	_, err = tx.ExecContext(ctx, updateQuery, message.Timestamp, message.TokenCount, sessionID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateMessage updates a message's content
func (ds *DBStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	start := time.Now()
	metric := ds.newOperationMetric("update_message", start)
	defer func() { metric.Record(nil) }()

	// Verify session belongs to user
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM aichat_sessions WHERE id = $1 AND user_id = $2)`
	err := ds.db.QueryRowContext(ctx, checkQuery, sessionID, userID).Scan(&exists)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update message content and reset token count so lazy counting recalculates on next token stats read.
	updateQuery := `UPDATE aichat_messages SET content = $1, token_count = 0 WHERE id = $2 AND session_id = $3`
	result, err := ds.db.ExecContext(ctx, updateQuery, content, messageID, sessionID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update message: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	// Keep session ordering stable by bumping updated_at for message edits.
	nowMs := time.Now().UnixMilli()
	var touchSessionQuery string
	if ds.dbType == "postgres" {
		touchSessionQuery = `UPDATE aichat_sessions SET updated_at = to_timestamp($1::bigint / 1000.0) WHERE id = $2 AND user_id = $3`
	} else {
		touchSessionQuery = `UPDATE aichat_sessions SET updated_at = $1 WHERE id = $2 AND user_id = $3`
	}

	if _, err := ds.db.ExecContext(ctx, touchSessionQuery, nowMs, sessionID, userID); err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update session timestamp: %w", err)
	}

	return nil
}

// ClearAllHistory deletes all sessions and messages for a user
func (ds *DBStorage) ClearAllHistory(ctx context.Context, userID string) error {
	start := time.Now()
	metric := ds.newOperationMetric("clear_all_history", start)
	defer func() { metric.Record(nil) }()

	query := `DELETE FROM aichat_sessions WHERE user_id = $1`
	_, err := ds.db.ExecContext(ctx, query, userID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to clear history: %w", err)
	}

	return nil
}

// DeleteExpiredSessions deletes sessions older than retentionDays
func (ds *DBStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	start := time.Now()
	metric := ds.newOperationMetric("delete_expired_sessions", start)
	defer func() { metric.Record(nil) }()

	var query string
	var args []interface{}

	if ds.dbType == "postgres" {
		query = `DELETE FROM aichat_sessions WHERE updated_at < NOW() - INTERVAL '1 day' * $1`
		args = []interface{}{retentionDays}
	} else {
		// SQLite: Calculate cutoff in milliseconds
		cutoffMs := (time.Now().Unix() - int64(retentionDays*24*60*60)) * 1000
		query = `DELETE FROM aichat_sessions WHERE updated_at < $1`
		args = []interface{}{cutoffMs}
	}

	result, err := ds.db.ExecContext(ctx, query, args...)
	if err != nil {
		metric.Record(err)
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	if deleted > 0 {
		ds.logger.Info("Deleted expired sessions", "count", deleted, "retentionDays", retentionDays)
	}

	return deleted, nil
}

// Ping checks database connectivity
func (ds *DBStorage) Ping(ctx context.Context) error {
	start := time.Now()
	err := ds.db.PingContext(ctx)
	ds.recordMetric("ping", start, err)
	return err
}

// SaveMessage stores a message with token count field (alias for AddMessage)
func (ds *DBStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	return ds.AddMessage(ctx, userID, sessionID, message)
}

// SaveMessages stores multiple messages in a batch with partial success tracking
func (ds *DBStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error) {
	start := time.Now()
	metric := ds.newOperationMetric("save_messages", start)
	defer func() { metric.Record(nil) }()

	if len(messages) == 0 {
		return []SaveResult{}, nil
	}

	results := make([]SaveResult, len(messages))

	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		metric.Record(err)
		// Mark all as failed
		for i, msg := range messages {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "failed to begin transaction"}
		}
		return results, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify session ownership once
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM aichat_sessions WHERE id = $1 AND user_id = $2)`
	err = tx.QueryRowContext(ctx, checkQuery, sessionID, userID).Scan(&exists)
	if err != nil {
		metric.Record(err)
		for i, msg := range messages {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "failed to verify session"}
		}
		return results, fmt.Errorf("failed to verify session: %w", err)
	}
	if !exists {
		for i, msg := range messages {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "session not found"}
		}
		return results, fmt.Errorf("session not found: %s", sessionID)
	}

	// Insert messages and track results
	var totalTokens int
	var lastTimestamp int64
	for i, msg := range messages {
		var insertQuery string
		var args []interface{}

		if ds.dbType == "postgres" {
			insertQuery = `INSERT INTO aichat_messages (id, session_id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth)
			               VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
			var summarizedIDs interface{}
			if len(msg.SummarizedIDs) > 0 {
				summarizedIDs = pq.Array(msg.SummarizedIDs)
			}
			args = []interface{}{msg.ID, sessionID, msg.Role, msg.Content, msg.Timestamp,
				msg.TokenCount, msg.IsPinned, msg.IsSummary, summarizedIDs, msg.SummaryDepth}
		} else {
			insertQuery = `INSERT INTO aichat_messages (id, session_id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth)
			               VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
			isPinned := 0
			if msg.IsPinned {
				isPinned = 1
			}
			isSummary := 0
			if msg.IsSummary {
				isSummary = 1
			}
			args = []interface{}{msg.ID, sessionID, msg.Role, msg.Content, msg.Timestamp,
				msg.TokenCount, isPinned, isSummary, serializeSummarizedIDs(msg.SummarizedIDs), msg.SummaryDepth}
		}

		_, err = tx.ExecContext(ctx, insertQuery, args...)
		if err != nil {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: err.Error()}
			// Continue with other messages - partial success allowed
		} else {
			results[i] = SaveResult{MessageID: msg.ID, Success: true}
			totalTokens += msg.TokenCount
			if msg.Timestamp > lastTimestamp {
				lastTimestamp = msg.Timestamp
			}
		}
	}

	// Update session's updated_at and total_tokens if any messages succeeded
	if totalTokens > 0 || lastTimestamp > 0 {
		var updateQuery string
		if ds.dbType == "postgres" {
			updateQuery = `UPDATE aichat_sessions
			               SET updated_at = to_timestamp($1::bigint / 1000.0), total_tokens = total_tokens + $2
			               WHERE id = $3`
		} else {
			updateQuery = `UPDATE aichat_sessions
			               SET updated_at = $1, total_tokens = total_tokens + $2
			               WHERE id = $3`
		}
		_, err = tx.ExecContext(ctx, updateQuery, lastTimestamp, totalTokens, sessionID)
		if err != nil {
			metric.Record(err)
			return results, fmt.Errorf("failed to update session: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		metric.Record(err)
		return results, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return results, nil
}

// GetSessionMessages returns paginated messages for a session with cursor support
func (ds *DBStorage) GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error) {
	start := time.Now()
	metric := ds.newOperationMetric("get_session_messages", start)
	defer func() { metric.Record(nil) }()

	// Validate session ownership
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM aichat_sessions WHERE id = $1 AND user_id = $2)`
	err := ds.db.QueryRowContext(ctx, checkQuery, params.SessionID, userID).Scan(&exists)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to verify session: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	// Apply defaults
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	order := params.Order
	if order != "asc" && order != "desc" {
		order = "desc" // Default to newest first
	}

	// Build query
	var query string
	var args []interface{}

	// Get cursor timestamp if provided
	var cursorTimestamp int64
	if params.Cursor != "" {
		cursorQuery := `SELECT timestamp FROM aichat_messages WHERE id = $1 AND session_id = $2`
		err := ds.db.QueryRowContext(ctx, cursorQuery, params.Cursor, params.SessionID).Scan(&cursorTimestamp)
		if err != nil {
			// Invalid cursor - return empty result
			return &MessagesPage{
				Messages: []ChatMessage{},
				PageInfo: PageInfo{HasNextPage: false, EndCursor: ""},
			}, nil
		}
	}

	// Build main query - fetch limit+1 to detect if there's a next page
	if params.Cursor != "" {
		if order == "desc" {
			query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
			         FROM aichat_messages
			         WHERE session_id = $1 AND timestamp < $2
			         ORDER BY timestamp DESC
			         LIMIT $3`
		} else {
			query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
			         FROM aichat_messages
			         WHERE session_id = $1 AND timestamp > $2
			         ORDER BY timestamp ASC
			         LIMIT $3`
		}
		args = []interface{}{params.SessionID, cursorTimestamp, limit + 1}
	} else {
		if order == "desc" {
			query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
			         FROM aichat_messages
			         WHERE session_id = $1
			         ORDER BY timestamp DESC
			         LIMIT $2`
		} else {
			query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
			         FROM aichat_messages
			         WHERE session_id = $1
			         ORDER BY timestamp ASC
			         LIMIT $2`
		}
		args = []interface{}{params.SessionID, limit + 1}
	}

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		var isPinned, isSummary, summaryDepth int
		var summarizedIDs sql.NullString

		if ds.dbType == "postgres" {
			var isPinnedBool, isSummaryBool bool
			var pgSummarizedIDs pq.StringArray
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinnedBool, &isSummaryBool, &pgSummarizedIDs, &summaryDepth)
			if isPinnedBool {
				isPinned = 1
			}
			if isSummaryBool {
				isSummary = 1
			}
			msg.SummarizedIDs = pgSummarizedIDs
		} else {
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinned, &isSummary, &summarizedIDs, &summaryDepth)
			msg.SummarizedIDs = deserializeSummarizedIDs(summarizedIDs)
		}

		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		msg.IsPinned = isPinned != 0
		msg.IsSummary = isSummary != 0
		msg.SummaryDepth = summaryDepth
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	// Determine pagination info
	hasNextPage := len(messages) > limit
	if hasNextPage {
		messages = messages[:limit] // Trim to requested limit
	}

	var endCursor string
	if len(messages) > 0 {
		endCursor = messages[len(messages)-1].ID
	}

	return &MessagesPage{
		Messages: messages,
		PageInfo: PageInfo{
			HasNextPage: hasNextPage,
			EndCursor:   endCursor,
		},
	}, nil
}

// GetMessagesByTokenBudget retrieves messages fitting within token limit
// Pinned messages are always included first, then unpinned newest-to-oldest until budget exhausted
// Returns messages in chronological order (oldest first) for LLM context
func (ds *DBStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error) {
	start := time.Now()
	metric := ds.newOperationMetric("get_messages_by_token_budget", start)
	defer func() { metric.Record(nil) }()

	// Get all messages ordered by timestamp DESC (newest first)
	query := `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
	          FROM aichat_messages
	          WHERE session_id = $1
	          ORDER BY timestamp DESC`

	rows, err := ds.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var allMessages []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		var isPinned, isSummary, summaryDepth int
		var summarizedIDs sql.NullString

		if ds.dbType == "postgres" {
			var isPinnedBool, isSummaryBool bool
			var pgSummarizedIDs pq.StringArray
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinnedBool, &isSummaryBool, &pgSummarizedIDs, &summaryDepth)
			if isPinnedBool {
				isPinned = 1
			}
			if isSummaryBool {
				isSummary = 1
			}
			msg.SummarizedIDs = pgSummarizedIDs
		} else {
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinned, &isSummary, &summarizedIDs, &summaryDepth)
			msg.SummarizedIDs = deserializeSummarizedIDs(summarizedIDs)
		}

		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		msg.IsPinned = isPinned != 0
		msg.IsSummary = isSummary != 0
		msg.SummaryDepth = summaryDepth
		allMessages = append(allMessages, msg)
	}

	if err := rows.Err(); err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	allMessages = FilterCompactedMessages(allMessages)

	// First pass: include all pinned messages (always preserved)
	var selected []ChatMessage
	remainingBudget := budget

	for _, msg := range allMessages {
		if msg.IsPinned {
			selected = append(selected, msg)
			remainingBudget -= msg.TokenCount
		}
	}

	// Second pass: add unpinned messages newest-to-oldest until budget exhausted
	for _, msg := range allMessages {
		if !msg.IsPinned && remainingBudget >= msg.TokenCount {
			selected = append(selected, msg)
			remainingBudget -= msg.TokenCount
		}
	}

	// Sort by timestamp ascending (oldest first) for LLM context
	for i := 0; i < len(selected)-1; i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[j].Timestamp < selected[i].Timestamp {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}

	return selected, nil
}

// getCoveredMessageIDs returns IDs that are already represented by summary messages.
func (ds *DBStorage) getCoveredMessageIDs(ctx context.Context, sessionID string) (map[string]struct{}, error) {
	var query string
	if ds.dbType == "postgres" {
		query = `SELECT summarized_ids FROM aichat_messages WHERE session_id = $1 AND is_summary = true`
	} else {
		query = `SELECT summarized_ids FROM aichat_messages WHERE session_id = $1 AND is_summary = 1`
	}

	rows, err := ds.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query covered message ids: %w", err)
	}
	defer rows.Close()

	covered := make(map[string]struct{})
	for rows.Next() {
		if ds.dbType == "postgres" {
			var pgSummarizedIDs pq.StringArray
			if err := rows.Scan(&pgSummarizedIDs); err != nil {
				return nil, fmt.Errorf("failed to scan covered ids: %w", err)
			}
			for _, id := range pgSummarizedIDs {
				if id == "" {
					continue
				}
				covered[id] = struct{}{}
			}
			continue
		}

		var summarizedIDs sql.NullString
		if err := rows.Scan(&summarizedIDs); err != nil {
			return nil, fmt.Errorf("failed to scan covered ids: %w", err)
		}
		for _, id := range deserializeSummarizedIDs(summarizedIDs) {
			if id == "" {
				continue
			}
			covered[id] = struct{}{}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating covered ids: %w", err)
	}

	return covered, nil
}

// UpdateMessageTokenCount updates the token count for a message
func (ds *DBStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	start := time.Now()
	metric := ds.newOperationMetric("update_message_token_count", start)
	defer func() { metric.Record(nil) }()

	query := `UPDATE aichat_messages SET token_count = $1 WHERE id = $2`
	result, err := ds.db.ExecContext(ctx, query, tokenCount, messageID)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update token count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// GetOldestNonSummaryMessages retrieves oldest messages that aren't summaries, up to token limit
// Returns messages in timestamp order (oldest first) for summarization
func (ds *DBStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error) {
	start := time.Now()
	metric := ds.newOperationMetric("get_oldest_non_summary_messages", start)
	defer func() { metric.Record(nil) }()

	coveredIDs, err := ds.getCoveredMessageIDs(ctx, sessionID)
	if err != nil {
		metric.Record(err)
		return nil, err
	}

	var query string
	if ds.dbType == "postgres" {
		query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
		         FROM aichat_messages
		         WHERE session_id = $1 AND is_summary = false
		         ORDER BY timestamp ASC`
	} else {
		query = `SELECT id, role, content, timestamp, token_count, is_pinned, is_summary, summarized_ids, summary_depth
		         FROM aichat_messages
		         WHERE session_id = $1 AND is_summary = 0
		         ORDER BY timestamp ASC`
	}

	rows, err := ds.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	totalTokens := 0

	for rows.Next() {
		var msg ChatMessage
		var isPinned, isSummary, summaryDepth int
		var summarizedIDs sql.NullString

		if ds.dbType == "postgres" {
			var isPinnedBool, isSummaryBool bool
			var pgSummarizedIDs pq.StringArray
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinnedBool, &isSummaryBool, &pgSummarizedIDs, &summaryDepth)
			if isPinnedBool {
				isPinned = 1
			}
			if isSummaryBool {
				isSummary = 1
			}
			msg.SummarizedIDs = pgSummarizedIDs
		} else {
			err = rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.Timestamp, &msg.TokenCount,
				&isPinned, &isSummary, &summarizedIDs, &summaryDepth)
			msg.SummarizedIDs = deserializeSummarizedIDs(summarizedIDs)
		}

		if err != nil {
			metric.Record(err)
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		msg.IsPinned = isPinned != 0
		msg.IsSummary = isSummary != 0
		msg.SummaryDepth = summaryDepth

		if _, covered := coveredIDs[msg.ID]; covered {
			continue
		}

		// Check if adding this message exceeds limit
		if totalTokens+msg.TokenCount > tokenLimit && len(messages) > 0 {
			break
		}

		messages = append(messages, msg)
		totalTokens += msg.TokenCount
	}

	if err := rows.Err(); err != nil {
		metric.Record(err)
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// SaveSummary stores a summary message and links it to original messages
// The summary message replaces the original messages in context while preserving full history
func (ds *DBStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error {
	start := time.Now()
	metric := ds.newOperationMetric("save_summary", start)
	defer func() { metric.Record(nil) }()

	// Set summary metadata
	summary.IsSummary = true
	summary.SummarizedIDs = originalIDs

	// Calculate summary depth from original messages
	// Find max depth among originals and add 1
	if len(originalIDs) > 0 {
		var maxDepth int
		var query string
		if ds.dbType == "postgres" {
			query = `SELECT COALESCE(MAX(summary_depth), 0) FROM aichat_messages WHERE id = ANY($1)`
			err := ds.db.QueryRowContext(ctx, query, pq.Array(originalIDs)).Scan(&maxDepth)
			if err != nil && err != sql.ErrNoRows {
				metric.Record(err)
				return fmt.Errorf("failed to get max summary depth: %w", err)
			}
		} else {
			// SQLite: Use IN clause with placeholders
			placeholders := make([]string, len(originalIDs))
			args := make([]interface{}, len(originalIDs))
			for i, id := range originalIDs {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
				args[i] = id
			}
			query = fmt.Sprintf(`SELECT COALESCE(MAX(summary_depth), 0) FROM aichat_messages WHERE id IN (%s)`,
				strings.Join(placeholders, ","))
			err := ds.db.QueryRowContext(ctx, query, args...).Scan(&maxDepth)
			if err != nil && err != sql.ErrNoRows {
				metric.Record(err)
				return fmt.Errorf("failed to get max summary depth: %w", err)
			}
		}
		summary.SummaryDepth = maxDepth + 1
	}

	// Save the summary message using existing AddMessage logic
	return ds.AddMessage(ctx, userID, sessionID, summary)
}

// sanitizeSearchQuery removes FTS special operators to prevent injection
// Removes: * " - + ~ : ^ @ AND OR NOT NEAR
func sanitizeSearchQuery(query string) string {
	// Remove FTS operators and special characters
	replacer := strings.NewReplacer(
		"*", "",
		"\"", "",
		"-", " ",
		"+", " ",
		"~", "",
		":", "",
		"^", "",
		"@", "",
		"(", "",
		")", "",
	)
	query = replacer.Replace(query)

	// Remove boolean operators (case-insensitive)
	words := strings.Fields(query)
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		upper := strings.ToUpper(word)
		if upper != "AND" && upper != "OR" && upper != "NOT" && upper != "NEAR" {
			filtered = append(filtered, word)
		}
	}

	return strings.Join(filtered, " ")
}

// SearchMessages searches for messages matching query across all user sessions
// Uses FTS5 MATCH for SQLite and tsvector @@ for PostgreSQL
func (ds *DBStorage) SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	start := time.Now()
	metric := ds.newOperationMetric("search_messages", start)
	defer func() { metric.Record(nil) }()

	// Apply defaults and limits
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Sanitize query to prevent FTS injection
	sanitizedQuery := sanitizeSearchQuery(params.Query)
	if strings.TrimSpace(sanitizedQuery) == "" {
		return []SearchResult{}, nil
	}

	var results []SearchResult
	var err error

	if ds.dbType == "sqlite" {
		results, err = ds.searchMessagesSQLite(ctx, params.UserID, sanitizedQuery, params.Limit, params.Offset)
	} else {
		results, err = ds.searchMessagesPostgres(ctx, params.UserID, sanitizedQuery, params.Limit, params.Offset)
	}

	if err != nil {
		metric.Record(err)
		return nil, err
	}

	return results, nil
}

// searchMessagesSQLite performs FTS5 search for SQLite
func (ds *DBStorage) searchMessagesSQLite(ctx context.Context, userID, query string, limit, offset int) ([]SearchResult, error) {
	// FTS5 query with snippet() for highlighted results
	// Join with sessions to get session name and filter by user
	// Order by bm25() for relevance ranking (lower is better match)
	sqlQuery := `
		SELECT
			m.session_id,
			s.name AS session_name,
			m.id AS message_id,
			snippet(aichat_messages_fts, 0, '<mark>', '</mark>', '...', 32) AS content,
			m.timestamp,
			m.role
		FROM aichat_messages_fts AS fts
		JOIN aichat_messages AS m ON fts.rowid = m.rowid
		JOIN aichat_sessions AS s ON m.session_id = s.id
		WHERE s.user_id = $1
		  AND aichat_messages_fts MATCH $2
		ORDER BY bm25(aichat_messages_fts)
		LIMIT $3 OFFSET $4
	`

	rows, err := ds.db.QueryContext(ctx, sqlQuery, userID, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		err := rows.Scan(&r.SessionID, &r.SessionName, &r.MessageID, &r.Content, &r.Timestamp, &r.Role)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// searchMessagesPostgres performs tsvector search for PostgreSQL
func (ds *DBStorage) searchMessagesPostgres(ctx context.Context, userID, query string, limit, offset int) ([]SearchResult, error) {
	// tsvector query with ts_headline() for highlighted results
	// Join with sessions to get session name and filter by user
	// Order by ts_rank() for relevance ranking (higher is better match)
	sqlQuery := `
		SELECT
			m.session_id,
			s.name AS session_name,
			m.id AS message_id,
			ts_headline('english', m.content, plainto_tsquery('english', $2),
						'StartSel=<mark>, StopSel=</mark>, MaxWords=35, MinWords=15') AS content,
			m.timestamp,
			m.role
		FROM aichat_messages AS m
		JOIN aichat_sessions AS s ON m.session_id = s.id
		WHERE s.user_id = $1
		  AND m.content_search @@ plainto_tsquery('english', $2)
		ORDER BY ts_rank(m.content_search, plainto_tsquery('english', $2)) DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := ds.db.QueryContext(ctx, sqlQuery, userID, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		err := rows.Scan(&r.SessionID, &r.SessionName, &r.MessageID, &r.Content, &r.Timestamp, &r.Role)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// UpdateMessagePinned updates the pinned state of a message (Phase 14)
func (ds *DBStorage) UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error {
	start := time.Now()
	metric := ds.newOperationMetric("update_message_pinned", start)
	defer func() { metric.Record(nil) }()

	// Verify session belongs to user
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM aichat_sessions WHERE id = $1 AND user_id = $2)`
	err := ds.db.QueryRowContext(ctx, checkQuery, sessionID, userID).Scan(&exists)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update pin state
	var updateQuery string
	var args []interface{}

	if ds.dbType == "postgres" {
		updateQuery = `UPDATE aichat_messages SET is_pinned = $1 WHERE id = $2 AND session_id = $3`
		args = []interface{}{isPinned, messageID, sessionID}
	} else {
		pinnedInt := 0
		if isPinned {
			pinnedInt = 1
		}
		updateQuery = `UPDATE aichat_messages SET is_pinned = $1 WHERE id = $2 AND session_id = $3`
		args = []interface{}{pinnedInt, messageID, sessionID}
	}

	result, err := ds.db.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to update pin state: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		metric.Record(err)
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// Close closes the database connection
func (ds *DBStorage) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.db != nil {
		return ds.db.Close()
	}
	return nil
}
