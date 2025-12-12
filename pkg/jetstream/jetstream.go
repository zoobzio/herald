// Package jetstream provides a Herald provider for NATS JetStream.
// Unlike NATS core, JetStream supports headers, persistence, and acknowledgments.
package jetstream

import (
	"context"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for NATS JetStream.
type Provider struct {
	js       jetstream.JetStream
	consumer jetstream.Consumer
	messages jetstream.MessagesContext
	stream   string
	subject  string
}

// Option configures a Provider.
type Option func(*Provider)

// WithJetStream sets the JetStream context.
func WithJetStream(js jetstream.JetStream) Option {
	return func(p *Provider) {
		p.js = js
	}
}

// WithConsumer sets the JetStream consumer for subscribing.
func WithConsumer(c jetstream.Consumer) Option {
	return func(p *Provider) {
		p.consumer = c
	}
}

// WithStream sets the stream name for publishing.
func WithStream(stream string) Option {
	return func(p *Provider) {
		p.stream = stream
	}
}

// New creates a JetStream provider for the given subject.
func New(subject string, opts ...Option) *Provider {
	p := &Provider{
		subject: subject,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to JetStream.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.js == nil {
		return herald.ErrNoWriter
	}

	msg := &nats.Msg{
		Subject: p.subject,
		Data:    data,
	}

	// Convert metadata to NATS headers
	if len(metadata) > 0 {
		msg.Header = make(nats.Header, len(metadata))
		for k, v := range metadata {
			msg.Header.Set(k, v)
		}
	}

	_, err := p.js.PublishMsg(ctx, msg)
	return err
}

// Subscribe returns a stream of messages from JetStream.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.consumer == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	// Create a MessagesContext for iteration with context support
	messages, err := p.consumer.Messages()
	if err != nil {
		go func() {
			out <- herald.NewError[herald.Message](err)
			close(out)
		}()
		return out
	}
	p.messages = messages

	go func() {
		defer close(out)
		defer messages.Stop()

		for {
			// Use NextContext for proper cancellation support
			msg, err := messages.Next(jetstream.NextContext(ctx))
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// ErrMsgIteratorClosed means we're shutting down
				if err == jetstream.ErrMsgIteratorClosed {
					return
				}
				select {
				case out <- herald.NewError[herald.Message](err):
				case <-ctx.Done():
					return
				}
				continue
			}

			// Capture msg for closure
			jsMsg := msg

			// Convert NATS headers to metadata, joining multi-value headers with comma
			var metadata herald.Metadata
			if len(msg.Headers()) > 0 {
				metadata = make(herald.Metadata, len(msg.Headers()))
				for k, vals := range msg.Headers() {
					if len(vals) > 0 {
						metadata[k] = strings.Join(vals, ",")
					}
				}
			}

			heraldMsg := herald.Message{
				Data:     msg.Data(),
				Metadata: metadata,
				Ack: func() error {
					return jsMsg.Ack()
				},
				Nack: func() error {
					return jsMsg.Nak()
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

// Ping verifies JetStream connectivity by checking account info.
func (p *Provider) Ping(ctx context.Context) error {
	if p.js == nil {
		return herald.ErrNoWriter
	}
	_, err := p.js.AccountInfo(ctx)
	return err
}

// Close releases JetStream resources.
func (p *Provider) Close() error {
	if p.messages != nil {
		p.messages.Stop()
	}
	return nil
}
