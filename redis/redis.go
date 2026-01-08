// Package redis provides a Herald provider for Redis Streams.
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for Redis Streams.
type Provider struct {
	client   *redis.Client
	stream   string
	group    string
	consumer string
}

// Option configures a Provider.
type Option func(*Provider)

// WithClient sets the Redis client.
func WithClient(c *redis.Client) Option {
	return func(p *Provider) {
		p.client = c
	}
}

// WithGroup sets the consumer group for acknowledgment.
func WithGroup(group string) Option {
	return func(p *Provider) {
		p.group = group
	}
}

// WithConsumer sets the consumer name within the group.
func WithConsumer(consumer string) Option {
	return func(p *Provider) {
		p.consumer = consumer
	}
}

// New creates a Redis provider for the given stream.
func New(stream string, opts ...Option) *Provider {
	p := &Provider{
		stream:   stream,
		consumer: "herald",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to Redis Stream.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}

	values := map[string]any{
		"data": data,
	}

	// Add metadata as additional stream fields
	for k, v := range metadata {
		values[k] = v
	}

	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: values,
	}).Err()
}

// Subscribe returns a stream of messages from Redis Stream.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.client == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		lastID := "0"
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var streams []redis.XStream
			var err error

			if p.group != "" {
				// Consumer group mode
				streams, err = p.client.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    p.group,
					Consumer: p.consumer,
					Streams:  []string{p.stream, ">"},
					Count:    10,
					Block:    0,
				}).Result()
			} else {
				// Simple read mode
				streams, err = p.client.XRead(ctx, &redis.XReadArgs{
					Streams: []string{p.stream, lastID},
					Count:   10,
					Block:   0,
				}).Result()
			}

			if err != nil {
				if err == redis.Nil {
					continue
				}
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

			for _, stream := range streams {
				for _, msg := range stream.Messages {
					lastID = msg.ID

					// Capture for closure
					msgID := msg.ID
					streamName := p.stream
					group := p.group
					client := p.client

					// Extract data and metadata
					var data []byte
					metadata := make(herald.Metadata)
					for k, v := range msg.Values {
						if k == "data" {
							switch d := v.(type) {
							case string:
								data = []byte(d)
							case []byte:
								data = d
							}
						} else {
							if s, ok := v.(string); ok {
								metadata[k] = s
							}
						}
					}

					heraldMsg := herald.Message{
						Data:     data,
						Metadata: metadata,
						Ack: func() error {
							if group != "" {
								// Use Background context: ack should succeed even if subscription context is cancelled
								return client.XAck(context.Background(), streamName, group, msgID).Err()
							}
							return nil
						},
						Nack: func() error {
							// Redis: don't ack = message remains pending for reclaim
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

// Ping verifies Redis connectivity.
func (p *Provider) Ping(ctx context.Context) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}
	return p.client.Ping(ctx).Err()
}

// Close releases Redis resources.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}
