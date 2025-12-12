package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MySQLDB implements DB for MySQL/MariaDB.
type MySQLDB struct {
	db    *sql.DB
	table string
}

// MySQLOption configures a MySQLDB.
type MySQLOption func(*MySQLDB)

// WithMySQLTable sets the table name (default: "herald_messages").
func WithMySQLTable(table string) MySQLOption {
	return func(m *MySQLDB) {
		m.table = table
	}
}

// NewMySQL creates a MySQLDB with the given connection.
// The table is created if it does not exist.
func NewMySQL(db *sql.DB, opts ...MySQLOption) (*MySQLDB, error) {
	m := &MySQLDB{
		db:    db,
		table: "herald_messages",
	}
	for _, opt := range opts {
		opt(m)
	}

	if err := m.createTable(); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return m, nil
}

// createTable creates the messages table if it does not exist.
func (m *MySQLDB) createTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id CHAR(36) PRIMARY KEY,
			seq BIGINT AUTO_INCREMENT UNIQUE,
			topic VARCHAR(255) NOT NULL,
			data BLOB NOT NULL,
			metadata JSON,
			created_at TIMESTAMP(6) DEFAULT CURRENT_TIMESTAMP(6),
			claimed_at TIMESTAMP(6) NULL,
			INDEX idx_topic_seq (topic, seq)
		)
	`, m.table)

	_, err := m.db.Exec(query)
	return err
}

// Insert adds a message to the queue table.
func (m *MySQLDB) Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error {
	var metaJSON any
	if len(metadata) > 0 {
		jsonBytes, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = jsonBytes
	}

	id := uuid.New().String()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, topic, data, metadata)
		VALUES (?, ?, ?, ?)
	`, m.table)

	_, err := m.db.ExecContext(ctx, query, id, topic, data, metaJSON)
	return err
}

// Fetch retrieves pending messages from the queue table.
// If visibilityTimeout > 0, messages are atomically claimed using a transaction
// with SELECT FOR UPDATE to ensure concurrent safety.
func (m *MySQLDB) Fetch(ctx context.Context, topic string, limit int, visibilityTimeout time.Duration) ([]Message, error) {
	if visibilityTimeout > 0 {
		return m.fetchWithClaim(ctx, topic, limit, visibilityTimeout)
	}
	return m.fetchSimple(ctx, topic, limit)
}

// fetchSimple retrieves messages without claiming (original behavior).
func (m *MySQLDB) fetchSimple(ctx context.Context, topic string, limit int) ([]Message, error) {
	query := fmt.Sprintf(`
		SELECT id, data, metadata
		FROM %s
		WHERE topic = ?
		ORDER BY seq ASC
		LIMIT ?
	`, m.table)

	rows, err := m.db.QueryContext(ctx, query, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return m.scanMessages(rows)
}

// fetchWithClaim retrieves and atomically claims messages using SELECT FOR UPDATE.
func (m *MySQLDB) fetchWithClaim(ctx context.Context, topic string, limit int, visibilityTimeout time.Duration) ([]Message, error) {
	now := time.Now()
	claimUntil := now.Add(visibilityTimeout)

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Select and lock unclaimed or expired messages
	selectQuery := fmt.Sprintf(`
		SELECT id, data, metadata
		FROM %s
		WHERE topic = ? AND (claimed_at IS NULL OR claimed_at < ?)
		ORDER BY seq ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED
	`, m.table)

	rows, err := tx.QueryContext(ctx, selectQuery, topic, now, limit)
	if err != nil {
		return nil, err
	}

	messages, err := m.scanMessages(rows)
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
	`, m.table, mysqlPlaceholders(len(ids)))

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
func (m *MySQLDB) scanMessages(rows *sql.Rows) ([]Message, error) {
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

// mysqlPlaceholders generates SQL placeholders for IN clause.
func mysqlPlaceholders(n int) string {
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
func (m *MySQLDB) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, m.table)
	_, err := m.db.ExecContext(ctx, query, id)
	return err
}

// Release makes a claimed message visible again by clearing claimed_at.
func (m *MySQLDB) Release(ctx context.Context, id string) error {
	query := fmt.Sprintf(`UPDATE %s SET claimed_at = NULL WHERE id = ?`, m.table)
	_, err := m.db.ExecContext(ctx, query, id)
	return err
}

// Ping verifies database connectivity.
func (m *MySQLDB) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// Close closes the database connection.
func (m *MySQLDB) Close() error {
	return m.db.Close()
}

// Ensure MySQLDB implements DB.
var _ DB = (*MySQLDB)(nil)
