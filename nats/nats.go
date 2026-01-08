// Package nats provides a Herald provider for NATS messaging.
package nats

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for NATS.
type Provider struct {
	conn    *nats.Conn
	sub     *nats.Subscription
	subject string
}

// Option configures a Provider.
type Option func(*Provider)

// WithConn sets the NATS connection.
func WithConn(c *nats.Conn) Option {
	return func(p *Provider) {
		p.conn = c
	}
}

// New creates a NATS provider for the given subject.
func New(subject string, opts ...Option) *Provider {
	p := &Provider{
		subject: subject,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes to NATS.
// Note: NATS core does not support headers; metadata is ignored.
// Use JetStream for header support.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.conn == nil {
		return herald.ErrNoWriter
	}
	return p.conn.Publish(p.subject, data)
}

// Subscribe returns a stream of messages from NATS.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.conn == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	sub, err := p.conn.SubscribeSync(p.subject)
	if err != nil {
		go func() {
			out <- herald.NewError[herald.Message](err)
			close(out)
		}()
		return out
	}
	p.sub = sub

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := sub.NextMsg(100 * time.Millisecond)
			if err != nil {
				if err == nats.ErrTimeout {
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

			heraldMsg := herald.Message{
				Data: msg.Data,
				Ack: func() error {
					// NATS core: no ack needed
					return nil
				},
				Nack: func() error {
					// NATS core: no nack mechanism
					return nil
				},
			}

			select {
			case out <- herald.NewSuccess(heraldMsg):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Ping verifies NATS connectivity.
func (p *Provider) Ping(ctx context.Context) error {
	if p.conn == nil {
		return herald.ErrNoWriter
	}
	return p.conn.FlushWithContext(ctx)
}

// Close releases NATS resources.
func (p *Provider) Close() error {
	if p.sub != nil {
		_ = p.sub.Unsubscribe()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	return nil
}
