package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// PostgresDB implements DB for PostgreSQL.
type PostgresDB struct {
	db    *sql.DB
	table string
}

// PostgresOption configures a PostgresDB.
type PostgresOption func(*PostgresDB)

// WithPostgresTable sets the table name (default: "herald_messages").
func WithPostgresTable(table string) PostgresOption {
	return func(p *PostgresDB) {
		p.table = table
	}
}

// NewPostgres creates a PostgresDB with the given connection.
// The table is created if it does not exist.
func NewPostgres(db *sql.DB, opts ...PostgresOption) (*PostgresDB, error) {
	p := &PostgresDB{
		db:    db,
		table: "herald_messages",
	}
	for _, opt := range opts {
		opt(p)
	}

	if err := p.createTable(); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return p, nil
}

// createTable creates the messages table if it does not exist.
func (p *PostgresDB) createTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			topic VARCHAR(255) NOT NULL,
			data BYTEA NOT NULL,
			metadata JSONB,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`, p.table)

	_, err := p.db.Exec(query)
	if err != nil {
		return err
	}

	// Create index on topic for efficient filtering
	indexQuery := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_topic ON %s (topic, created_at)
	`, p.table, p.table)

	_, err = p.db.Exec(indexQuery)
	return err
}

// Insert adds a message to the queue table.
func (p *PostgresDB) Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error {
	var metaJSON any
	if len(metadata) > 0 {
		jsonBytes, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = jsonBytes
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (topic, data, metadata)
		VALUES ($1, $2, $3)
	`, p.table)

	_, err := p.db.ExecContext(ctx, query, topic, data, metaJSON)
	return err
}

// Fetch retrieves pending messages from the queue table.
func (p *PostgresDB) Fetch(ctx context.Context, topic string, limit int) ([]Message, error) {
	query := fmt.Sprintf(`
		SELECT id, data, metadata
		FROM %s
		WHERE topic = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, p.table)

	rows, err := p.db.QueryContext(ctx, query, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var id uuid.UUID
		var metaJSON []byte

		if err := rows.Scan(&id, &msg.Data, &metaJSON); err != nil {
			return nil, err
		}

		msg.ID = id.String()

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
func (p *PostgresDB) Delete(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, p.table)
	_, err := p.db.ExecContext(ctx, query, id)
	return err
}

// Close closes the database connection.
func (p *PostgresDB) Close() error {
	return p.db.Close()
}

// Ensure PostgresDB implements DB.
var _ DB = (*PostgresDB)(nil)
