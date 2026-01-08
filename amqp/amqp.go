// Package amqp provides a Herald provider for RabbitMQ via AMQP 0-9-1.
package amqp

import (
	"context"
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for RabbitMQ.
type Provider struct {
	channel  *amqp.Channel
	exchange string
	queue    string
	key      string
}

// Option configures a Provider.
type Option func(*Provider)

// WithChannel sets the AMQP channel.
func WithChannel(ch *amqp.Channel) Option {
	return func(p *Provider) {
		p.channel = ch
	}
}

// WithExchange sets the exchange name for publishing.
func WithExchange(exchange string) Option {
	return func(p *Provider) {
		p.exchange = exchange
	}
}

// WithRoutingKey sets the routing key for publishing.
func WithRoutingKey(key string) Option {
	return func(p *Provider) {
		p.key = key
	}
}

// New creates a RabbitMQ provider for the given queue.
func New(queue string, opts ...Option) *Provider {
	p := &Provider{
		queue: queue,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to RabbitMQ.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.channel == nil {
		return herald.ErrNoWriter
	}

	// Convert herald.Metadata to AMQP headers
	var headers amqp.Table
	if len(metadata) > 0 {
		headers = make(amqp.Table, len(metadata))
		for k, v := range metadata {
			headers[k] = v
		}
	}

	return p.channel.PublishWithContext(ctx, p.exchange, p.key, false, false, amqp.Publishing{
		Headers: headers,
		Body:    data,
	})
}

// Subscribe returns a stream of messages from RabbitMQ.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.channel == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	deliveries, err := p.channel.ConsumeWithContext(ctx, p.queue, "", false, false, false, false, nil)
	if err != nil {
		go func() {
			defer close(out)
			select {
			case out <- herald.NewError[herald.Message](err):
			case <-ctx.Done():
			}
		}()
		return out
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case delivery, ok := <-deliveries:
				if !ok {
					return
				}

				// Capture for closure
				d := delivery

				// Convert AMQP headers to metadata
				// AMQP headers can be various types; we convert to strings
				// Arrays are joined with commas
				var metadata herald.Metadata
				if len(d.Headers) > 0 {
					metadata = make(herald.Metadata, len(d.Headers))
					for k, v := range d.Headers {
						metadata[k] = headerValueToString(v)
					}
				}

				msg := herald.Message{
					Data:     d.Body,
					Metadata: metadata,
					Ack: func() error {
						return d.Ack(false)
					},
					Nack: func() error {
						return d.Nack(false, true) // requeue
					},
				}

				select {
				case out <- herald.NewSuccess(msg):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Ping verifies AMQP connectivity by checking channel status.
func (p *Provider) Ping(ctx context.Context) error {
	if p.channel == nil {
		return herald.ErrNoWriter
	}
	if p.channel.IsClosed() {
		return herald.ErrNoWriter
	}
	return nil
}

// Close releases AMQP resources.
func (p *Provider) Close() error {
	if p.channel != nil {
		return p.channel.Close()
	}
	return nil
}

// headerValueToString converts AMQP header values to strings.
// Arrays are joined with commas.
func headerValueToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, headerValueToString(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", val)
	}
}
