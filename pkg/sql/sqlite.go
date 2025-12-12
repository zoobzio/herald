package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SQLiteDB implements DB for SQLite.
type SQLiteDB struct {
	db    *sql.DB
	table string
}

// SQLiteOption configures a SQLiteDB.
type SQLiteOption func(*SQLiteDB)

// WithSQLiteTable sets the table name (default: "herald_messages").
func WithSQLiteTable(table string) SQLiteOption {
	return func(s *SQLiteDB) {
		s.table = table
	}
}

// NewSQLite creates a SQLiteDB with the given connection.
// The table is created if it does not exist.
func NewSQLite(db *sql.DB, opts ...SQLiteOption) (*SQLiteDB, error) {
	s := &SQLiteDB{
		db:    db,
		table: "herald_messages",
	}
	for _, opt := range opts {
		opt(s)
	}

	if err := s.createTable(); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return s, nil
}

// createTable creates the messages table if it does not exist.
func (s *SQLiteDB) createTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			topic TEXT NOT NULL,
			data BLOB NOT NULL,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			claimed_at DATETIME
		)
	`, s.table)

	_, err := s.db.Exec(query)
	if err != nil {
		return err
	}

	// Create index on topic for efficient filtering
	indexQuery := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_topic ON %s (topic, created_at)
	`, s.table, s.table)

	_, err = s.db.Exec(indexQuery)
	return err
}

// Insert adds a message to the queue table.
func (s *SQLiteDB) Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error {
	var metaJSON []byte
	var err error
	if len(metadata) > 0 {
		metaJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
	}

	id := uuid.New().String()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, topic, data, metadata)
		VALUES (?, ?, ?, ?)
	`, s.table)

	_, err = s.db.ExecContext(ctx, query, id, topic, data, metaJSON)
	return err
}

// Fetch retrieves pending messages from the queue table.
// If visibilityTimeout > 0, messages are atomically claimed and become invisible
// to other consumers until the timeout expires or the message is deleted/released.
func (s *SQLiteDB) Fetch(ctx context.Context, topic string, limit int, visibilityTimeout time.Duration) ([]Message, error) {
	if visibilityTimeout > 0 {
		return s.fetchWithClaim(ctx, topic, limit, visibilityTimeout)
	}
	return s.fetchSimple(ctx, topic, limit)
}

// fetchSimple retrieves messages without claiming (original behavior).
func (s *SQLiteDB) fetchSimple(ctx context.Context, topic string, limit int) ([]Message, error) {
	query := fmt.Sprintf(`
		SELECT id, data, metadata
		FROM %s
		WHERE topic = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, s.table)

	rows, err := s.db.QueryContext(ctx, query, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// fetchWithClaim retrieves and atomically claims messages.
func (s *SQLiteDB) fetchWithClaim(ctx context.Context, topic string, limit int, visibilityTimeout time.Duration) ([]Message, error) {
	now := time.Now()
	claimUntil := now.Add(visibilityTimeout)

	// SQLite doesn't support UPDATE...RETURNING in older versions,
	// so we use a transaction with SELECT FOR UPDATE pattern simulation
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Select unclaimed or expired messages
	selectQuery := fmt.Sprintf(`
		SELECT id, data, metadata
		FROM %s
		WHERE topic = ? AND (claimed_at IS NULL OR claimed_at < ?)
		ORDER BY created_at ASC
		LIMIT ?
	`, s.table)

	rows, err := tx.QueryContext(ctx, selectQuery, topic, now, limit)
	if err != nil {
		return nil, err
	}

	messages, err := s.scanMessages(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return messages, nil
	}

	// Claim the selected messages
	ids := make([]any, len(messages))
	for i, msg := range messages {
		ids[i] = msg.ID
	}

	updateQuery := fmt.Sprintf(`
		UPDATE %s SET claimed_at = ? WHERE id IN (%s)
	`, s.table, placeholders(len(ids)))

	args := append([]any{claimUntil}, ids...)
	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return messages, nil
}

// scanMessages extracts Message structs from rows.
func (s *SQLiteDB) scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var msg Message
		var metaJSON []byte

		if err := rows.Scan(&msg.ID, &msg.Data, &metaJSON); err != nil {
			return nil, err
		}

		if len(metaJSON) > 0 {
			if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// placeholders generates SQL placeholders for IN clause.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}

// Delete removes a message from the queue table.
func (s *SQLiteDB) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, s.table)
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// Release makes a claimed message visible again by clearing claimed_at.
func (s *SQLiteDB) Release(ctx context.Context, id string) error {
	query := fmt.Sprintf(`UPDATE %s SET claimed_at = NULL WHERE id = ?`, s.table)
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// Ping verifies database connectivity.
func (s *SQLiteDB) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Ensure SQLiteDB implements DB.
var _ DB = (*SQLiteDB)(nil)
