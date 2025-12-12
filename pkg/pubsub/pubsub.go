// Package pubsub provides a Herald provider for Google Cloud Pub/Sub.
package pubsub

import (
	"context"

	"cloud.google.com/go/pubsub"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for Google Cloud Pub/Sub.
type Provider struct {
	topic *pubsub.Topic
	sub   *pubsub.Subscription
}

// Option configures a Provider.
type Option func(*Provider)

// WithTopic sets the Pub/Sub topic for publishing.
func WithTopic(t *pubsub.Topic) Option {
	return func(p *Provider) {
		p.topic = t
	}
}

// WithSubscription sets the Pub/Sub subscription for consuming.
func WithSubscription(s *pubsub.Subscription) Option {
	return func(p *Provider) {
		p.sub = s
	}
}

// New creates a Pub/Sub provider.
func New(opts ...Option) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish sends raw bytes with metadata to Pub/Sub.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.topic == nil {
		return herald.ErrNoWriter
	}

	msg := &pubsub.Message{
		Data: data,
	}

	// Convert herald.Metadata to Pub/Sub attributes
	if len(metadata) > 0 {
		msg.Attributes = make(map[string]string, len(metadata))
		for k, v := range metadata {
			msg.Attributes[k] = v
		}
	}

	result := p.topic.Publish(ctx, msg)
	_, err := result.Get(ctx)
	return err
}

// Subscribe returns a stream of messages from Pub/Sub.
func (p *Provider) Subscribe(ctx context.Context) <-chan herald.Result[herald.Message] {
	out := make(chan herald.Result[herald.Message])

	if p.sub == nil {
		go func() {
			out <- herald.NewError[herald.Message](herald.ErrNoReader)
			close(out)
		}()
		return out
	}

	go func() {
		defer close(out)

		err := p.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			// Capture msg for closure
			pubsubMsg := msg

			// Convert Pub/Sub attributes to metadata
			var metadata herald.Metadata
			if len(msg.Attributes) > 0 {
				metadata = make(herald.Metadata, len(msg.Attributes))
				for k, v := range msg.Attributes {
					metadata[k] = v
				}
			}

			heraldMsg := herald.Message{
				Data:     msg.Data,
				Metadata: metadata,
				Ack: func() error {
					pubsubMsg.Ack()
					return nil
				},
				Nack: func() error {
					pubsubMsg.Nack()
					return nil
				},
			}

			select {
			case out <- herald.NewSuccess(heraldMsg):
			case <-ctx.Done():
			}
		})

		if err != nil && ctx.Err() == nil {
			select {
			case out <- herald.NewError[herald.Message](err):
			case <-ctx.Done():
			}
		}
	}()

	return out
}

// Ping verifies Pub/Sub connectivity by checking topic existence.
func (p *Provider) Ping(ctx context.Context) error {
	if p.topic == nil && p.sub == nil {
		return herald.ErrNoWriter
	}
	if p.topic != nil {
		exists, err := p.topic.Exists(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return herald.ErrNoWriter
		}
	}
	return nil
}

// Close releases Pub/Sub resources.
func (p *Provider) Close() error {
	if p.topic != nil {
		p.topic.Stop()
	}
	return nil
}
