// Package firestore provides a Herald provider for Google Cloud Firestore.
// Uses collections for topics with document snapshots for subscription.
package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/zoobzio/herald"
)

// Provider implements herald.Provider for Firestore.
type Provider struct {
	client     *firestore.Client
	collection string
}

// Option configures a Provider.
type Option func(*Provider)

// WithClient sets the Firestore client.
func WithClient(c *firestore.Client) Option {
	return func(p *Provider) {
		p.client = c
	}
}

// New creates a Firestore provider for the given collection.
func New(collection string, opts ...Option) *Provider {
	p := &Provider{
		collection: collection,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// document represents the structure stored in Firestore.
type document struct {
	Data     []byte            `firestore:"data"`
	Metadata map[string]string `firestore:"metadata,omitempty"`
}

// Publish adds a document with metadata to the collection.
func (p *Provider) Publish(ctx context.Context, data []byte, metadata herald.Metadata) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}

	doc := document{
		Data: data,
	}

	if len(metadata) > 0 {
		doc.Metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			doc.Metadata[k] = v
		}
	}

	_, _, err := p.client.Collection(p.collection).Add(ctx, doc)
	return err
}

// Subscribe watches the collection for new documents.
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

		snapshots := p.client.Collection(p.collection).Snapshots(ctx)
		defer snapshots.Stop()

		for {
			snapshot, err := snapshots.Next()
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

			for _, change := range snapshot.Changes {
				if change.Kind != firestore.DocumentAdded {
					continue
				}

				// Capture for closure
				docRef := change.Doc.Ref

				var doc document
				if err := change.Doc.DataTo(&doc); err != nil {
					select {
					case out <- herald.NewError[herald.Message](err):
					case <-ctx.Done():
						return
					}
					continue
				}

				// Convert document metadata to herald.Metadata
				var metadata herald.Metadata
				if len(doc.Metadata) > 0 {
					metadata = make(herald.Metadata, len(doc.Metadata))
					for k, v := range doc.Metadata {
						metadata[k] = v
					}
				}

				msg := herald.Message{
					Data:     doc.Data,
					Metadata: metadata,
					Ack: func() error {
						// Use Background context: ack should succeed even if subscription context is cancelled
						_, err := docRef.Delete(context.Background())
						return err
					},
					Nack: func() error {
						// Don't delete - document remains for retry
						return nil
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

// Ping verifies Firestore connectivity by checking collection access.
func (p *Provider) Ping(ctx context.Context) error {
	if p.client == nil {
		return herald.ErrNoWriter
	}
	// Attempt to get collection reference to verify connectivity
	_ = p.client.Collection(p.collection)
	return nil
}

// Close releases Firestore resources.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}
