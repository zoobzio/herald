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
	// If visibilityTimeout > 0, messages are claimed and become invisible to other consumers
	// for that duration. Use 0 to disable visibility timeout (not recommended for concurrent consumers).
	Fetch(ctx context.Context, topic string, limit int, visibilityTimeout time.Duration) ([]Message, error)
	// Delete removes a message from the queue table.
	Delete(ctx context.Context, id string) error
	// Release makes a claimed message visible again for reprocessing.
	// Called on Nack when visibility timeout is enabled.
	Release(ctx context.Context, id string) error
	// Ping verifies database connectivity.
	Ping(ctx context.Context) error
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
	db                DB
	topic             string
	pollInterval      time.Duration
	batchSize         int
	visibilityTimeout time.Duration
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

// WithVisibilityTimeout sets how long a message remains invisible after being fetched.
// While invisible, the message won't be delivered to other consumers.
// If the message is not acked before the timeout expires, it becomes visible again.
// Default is 0 (no visibility timeout - not recommended for concurrent consumers).
func WithVisibilityTimeout(d time.Duration) Option {
	return func(p *Provider) {
		p.visibilityTimeout = d
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
// If WithVisibilityTimeout is configured, messages are claimed atomically and become
// invisible to other consumers for that duration. This prevents duplicate processing
// in concurrent consumer scenarios.
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
				msgs, err := p.db.Fetch(ctx, p.topic, p.batchSize, p.visibilityTimeout)
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
					hasVisibilityTimeout := p.visibilityTimeout > 0

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
							// Use Background context: ack should succeed even if subscription context is cancelled
							return db.Delete(context.Background(), id)
						},
						Nack: func() error {
							// If visibility timeout is enabled, release the message immediately
							// so it becomes visible to other consumers without waiting for timeout
							if hasVisibilityTimeout {
								return db.Release(context.Background(), id)
							}
							// Without visibility timeout, message is already visible (no-op)
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

// Ping verifies database connectivity.
func (p *Provider) Ping(ctx context.Context) error {
	if p.db == nil {
		return herald.ErrNoWriter
	}
	return p.db.Ping(ctx)
}

// Close releases database resources.
func (p *Provider) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
