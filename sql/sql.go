// Package sql provides a Herald provider for SQL databases (PostgreSQL, MySQL, SQLite).
// Uses a messages table for pub/sub via polling.
package sql

import (
	"context"
	"time"

	"github.com/zoobzio/herald"
)

// DB defines the interface for SQL database operations.
type DB interface {
	// Insert adds a message to the queue table.
	Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error
	// Fetch retrieves pending messages from the queue table.
	Fetch(ctx context.Context, topic string, limit int) ([]Message, error)
	// Delete removes a message from the queue table.
	Delete(ctx context.Context, id string) error
	// Close closes the database connection.
	Close() error
}

// Message represents a row in the messages table.
type Message struct {
	ID       string
	Data     []byte
	Metadata map[string]string
}

// Provider implements herald.Provider for SQL databases.
type Provider struct {
	db           DB
	topic        string
	pollInterval time.Duration
	batchSize    int
}

// Option configures a Provider.
type Option func(*Provider)

// WithDB sets the database connection.
func WithDB(db DB) Option {
	return func(p *Provider) {
		p.db = db
	}
}

// WithPollInterval sets how often to poll for new messages.
func WithPollInterval(d time.Duration) Option {
	return func(p *Provider) {
		p.pollInterval = d
	}
}

// WithBatchSize sets how many messages to fetch per poll.
func WithBatchSize(n int) Option {
	return func(p *Provider) {
		p.batchSize = n
	}
}

// New creates a SQL provider for the given topic.
func New(topic string, opts ...Option) *Provider {
	p := &Provider{
		topic:        topic,
		pollInterval: 100 * time.Millisecond,
		batchSize:    10,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish inserts a message with metadata into the queue table.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.db == nil {
		return herald.ErrNoWriter
	}

	// Convert herald.Metadata to map[string]string for storage
	var meta map[string]string
	if len(metadata) > 0 {
		meta = make(map[string]string, len(metadata))
		for k, v := range metadata {
			meta[k] = v
		}
	}

	return p.db.Insert(ctx, p.topic, data, meta)
}

// Subscribe polls the database for new messages.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.db == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		ticker := time.NewTicker(p.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				msgs, err := p.db.Fetch(ctx, p.topic, p.batchSize)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					select {
					case out <- herald.NewError[herald.Message](err):
					case <-ctx.Done():
						return
					}
					continue
				}

				for _, msg := range msgs {
					// Capture for closure
					id := msg.ID
					db := p.db

					// Convert stored metadata to herald.Metadata
					var metadata herald.Metadata
					if len(msg.Metadata) > 0 {
						metadata = make(herald.Metadata, len(msg.Metadata))
						for k, v := range msg.Metadata {
							metadata[k] = v
						}
					}

					heraldMsg := herald.Message{
						Data:     msg.Data,
						Metadata: metadata,
						Ack: func() error {
							return db.Delete(ctx, id)
						},
						Nack: func() error {
							// Don't delete - message remains for retry
							return nil
						},
					}

					select {
					case out <- herald.NewSuccess(heraldMsg):
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}

// Close releases database resources.
func (p *Provider) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
