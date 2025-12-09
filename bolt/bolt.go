// Package bolt provides a Herald provider for BoltDB (bbolt).
// Uses buckets for topics with sequential keys for ordering.
package bolt

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/zoobzio/herald"
	"go.etcd.io/bbolt"
)

// Provider implements herald.Provider for BoltDB.
type Provider struct {
	db           *bbolt.DB
	bucket       string
	pollInterval time.Duration
	batchSize    int
}

// Option configures a Provider.
type Option func(*Provider)

// WithDB sets the BoltDB connection.
func WithDB(db *bbolt.DB) Option {
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

// New creates a BoltDB provider for the given bucket (topic).
func New(bucket string, opts ...Option) *Provider {
	p := &Provider{
		bucket:       bucket,
		pollInterval: 100 * time.Millisecond,
		batchSize:    10,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish stores a message in the bucket.
// Note: BoltDB does not support metadata natively; metadata is ignored.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.db == nil {
		return herald.ErrNoWriter
	}

	return p.db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(p.bucket))
		if err != nil {
			return err
		}

		id, err := b.NextSequence()
		if err != nil {
			return err
		}

		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, id)

		return b.Put(key, data)
	})
}

// Subscribe polls the bucket for new messages.
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
				var messages []struct {
					key  []byte
					data []byte
				}

				err := p.db.View(func(tx *bbolt.Tx) error {
					b := tx.Bucket([]byte(p.bucket))
					if b == nil {
						return nil
					}

					c := b.Cursor()
					count := 0
					for k, v := c.First(); k != nil && count < p.batchSize; k, v = c.Next() {
						// Copy key and value since they're only valid during transaction
						keyCopy := make([]byte, len(k))
						copy(keyCopy, k)
						dataCopy := make([]byte, len(v))
						copy(dataCopy, v)

						messages = append(messages, struct {
							key  []byte
							data []byte
						}{keyCopy, dataCopy})
						count++
					}
					return nil
				})

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

				for _, msg := range messages {
					// Capture for closure
					key := msg.key
					db := p.db
					bucket := p.bucket

					heraldMsg := herald.Message{
						Data: msg.data,
						Ack: func() error {
							return db.Update(func(tx *bbolt.Tx) error {
								b := tx.Bucket([]byte(bucket))
								if b == nil {
									return nil
								}
								return b.Delete(key)
							})
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

// Close releases BoltDB resources.
func (p *Provider) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
