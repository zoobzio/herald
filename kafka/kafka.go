// Package kafka provides a Herald provider for Apache Kafka.
package kafka

import (
	"context"

	"github.com/segmentio/kafka-go"
	"github.com/zoobzio/herald"
)

// Writer defines the interface for Kafka message production.
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// Reader defines the interface for Kafka message consumption.
type Reader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// Provider implements herald.Provider for Kafka.
type Provider struct {
	writer Writer
	reader Reader
	topic  string
}

// Option configures a Provider.
type Option func(*Provider)

// WithWriter sets the Kafka writer for publishing.
func WithWriter(w Writer) Option {
	return func(p *Provider) {
		p.writer = w
	}
}

// WithReader sets the Kafka reader for subscribing.
func WithReader(r Reader) Option {
	return func(p *Provider) {
		p.reader = r
	}
}

// New creates a Kafka provider for the given topic.
func New(topic string, opts ...Option) *Provider {
	p := &Provider{
		topic: topic,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to Kafka.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.writer == nil {
		return herald.ErrNoWriter
	}

	msg := kafka.Message{
		Topic: p.topic,
		Value: data,
	}

	// Convert metadata to Kafka headers
	if len(metadata) > 0 {
		msg.Headers = make([]kafka.Header, 0, len(metadata))
		for k, v := range metadata {
			msg.Headers = append(msg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(v),
			})
		}
	}

	return p.writer.WriteMessages(ctx, msg)
}

// Subscribe returns a stream of messages from Kafka.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.reader == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := p.reader.FetchMessage(ctx)
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

			// Capture msg for closure
			kafkaMsg := msg

			// Convert Kafka headers to metadata
			var metadata herald.Metadata
			if len(msg.Headers) > 0 {
				metadata = make(herald.Metadata, len(msg.Headers))
				for _, h := range msg.Headers {
					metadata[h.Key] = string(h.Value)
				}
			}

			heraldMsg := herald.Message{
				Data:     msg.Value,
				Metadata: metadata,
				Ack: func() error {
					return p.reader.CommitMessages(ctx, kafkaMsg)
				},
				Nack: func() error {
					// Kafka: don't commit = message will be redelivered on next consumer
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

// Close releases Kafka resources.
func (p *Provider) Close() error {
	var firstErr error
	if p.writer != nil {
		if err := p.writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if p.reader != nil {
		if err := p.reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
