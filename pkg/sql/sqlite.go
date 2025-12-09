package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

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
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
func (s *SQLiteDB) Fetch(ctx context.Context, topic string, limit int) ([]Message, error) {
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

// Delete removes a message from the queue table.
func (s *SQLiteDB) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, s.table)
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// Close closes the database connection.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// Ensure SQLiteDB implements DB.
var _ DB = (*SQLiteDB)(nil)
