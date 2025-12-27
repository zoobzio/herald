package herald

import (
	"context"
	"sync"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Internal identities for publisher.
var (
	publishID         = pipz.NewIdentity("herald:publish", "Publishes to broker")
	publishPipelineID = pipz.NewIdentity("herald:publisher", "Publisher pipeline")
)

// Publisher observes a Capitan signal and publishes events to a broker.
// T is the struct type representing the message contract.
type Publisher[T any] struct {
	provider Provider
	signal   capitan.Signal
	key      capitan.GenericKey[T]
	capitan  *capitan.Capitan
	codec    Codec
	pipeline *pipz.Pipeline[*Envelope[T]]
	observer *capitan.Observer
	inflight sync.WaitGroup
}

// PublisherOption configures a Publisher.
type PublisherOption[T any] func(*Publisher[T])

// WithPublisherCapitan sets a custom Capitan instance for the publisher.
func WithPublisherCapitan[T any](c *capitan.Capitan) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.capitan = c
	}
}

// WithPublisherCodec sets a custom codec for the publisher.
// If not specified, JSONCodec is used.
func WithPublisherCodec[T any](c Codec) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.codec = c
	}
}

// NewPublisher creates a Publisher that observes the given signal and publishes T to the broker.
//
// Parameters:
//   - provider: broker implementation (kafka, nats, sqs, etc.)
//   - signal: capitan signal to observe for events
//   - key: typed key for extracting T from events
//   - pipelineOpts: reliability middleware (retry, timeout, circuit breaker); nil for none
//   - opts: publisher configuration (custom codec, custom capitan instance)
func NewPublisher[T any](provider Provider, signal capitan.Signal, key capitan.GenericKey[T], pipelineOpts []Option[T], opts ...PublisherOption[T]) *Publisher[T] {
	p := &Publisher[T]{
		provider: provider,
		signal:   signal,
		key:      key,
		codec:    JSONCodec{},
	}
	for _, opt := range opts {
		opt(p)
	}

	// Guard against nil codec
	if p.codec == nil {
		p.codec = JSONCodec{}
	}

	// Build pipeline: start with terminal, wrap with options
	chain := newPublishTerminal[T](provider, p.codec)
	for _, opt := range pipelineOpts {
		chain = opt(chain)
	}
	p.pipeline = pipz.NewPipeline(publishPipelineID, chain)

	return p
}

// newPublishTerminal creates the terminal operation that marshals and publishes to the broker.
func newPublishTerminal[T any](provider Provider, codec Codec) pipz.Chainable[*Envelope[T]] {
	return pipz.Apply(publishID, func(ctx context.Context, env *Envelope[T]) (*Envelope[T], error) {
		data, err := codec.Marshal(env.Value)
		if err != nil {
			return env, err
		}
		// Build metadata from envelope, set Content-Type if not present
		metadata := copyMetadata(env.Metadata)
		if _, exists := metadata["Content-Type"]; !exists {
			metadata["Content-Type"] = codec.ContentType()
		}
		err = provider.Publish(ctx, data, metadata)
		return env, err
	})
}

// Start begins observing the signal and publishing to the broker.
func (p *Publisher[T]) Start() {
	callback := func(ctx context.Context, e *capitan.Event) {
		p.inflight.Add(1)
		defer p.inflight.Done()

		value, ok := p.key.From(e)
		if !ok {
			return
		}

		// Wrap value in envelope with empty metadata
		env := &Envelope[T]{
			Value:    value,
			Metadata: make(Metadata),
		}

		// Process through pipeline (includes publish)
		_, err := p.pipeline.Process(ctx, env)
		if err != nil {
			p.emitError(ctx, err.Error())
		}
	}

	if p.capitan != nil {
		p.observer = p.capitan.Observe(callback, p.signal)
	} else {
		p.observer = capitan.Observe(callback, p.signal)
	}
}

// emitError emits an error event to ErrorSignal.
func (p *Publisher[T]) emitError(ctx context.Context, errMsg string) {
	e := Error{
		Operation: "publish",
		Signal:    p.signal.Name(),
		Err:       errMsg,
		Nack:      false,
	}
	if p.capitan != nil {
		p.capitan.Emit(ctx, ErrorSignal, ErrorKey.Field(e))
	} else {
		capitan.Emit(ctx, ErrorSignal, ErrorKey.Field(e))
	}
}

// Close stops the publisher, waits for in-flight publishes, and releases resources.
func (p *Publisher[T]) Close() error {
	if p.observer != nil {
		p.observer.Close()
	}
	p.inflight.Wait()
	if p.pipeline != nil {
		return p.pipeline.Close()
	}
	return nil
}
