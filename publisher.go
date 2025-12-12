package herald

import (
	"context"
	"sync"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Publisher observes a Capitan signal and publishes events to a broker.
// T is the struct type representing the message contract.
type Publisher[T any] struct {
	provider Provider
	signal   capitan.Signal
	key      capitan.GenericKey[T]
	capitan  *capitan.Capitan
	codec    Codec
	pipeline pipz.Chainable[T]
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
	var pipeline pipz.Chainable[T] = newPublishTerminal[T](provider, p.codec)
	for _, opt := range pipelineOpts {
		pipeline = opt(pipeline)
	}
	p.pipeline = pipeline

	return p
}

// newPublishTerminal creates the terminal operation that marshals and publishes to the broker.
func newPublishTerminal[T any](provider Provider, codec Codec) pipz.Chainable[T] {
	return pipz.Apply("publish", func(ctx context.Context, value T) (T, error) {
		data, err := codec.Marshal(value)
		if err != nil {
			return value, err
		}
		// Build metadata: copy from context to avoid mutation, or create new
		metadata := copyMetadata(MetadataFromContext(ctx))
		// Set Content-Type if not already present
		if _, exists := metadata["Content-Type"]; !exists {
			metadata["Content-Type"] = codec.ContentType()
		}
		err = provider.Publish(ctx, data, metadata)
		return value, err
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

		// Process through pipeline (includes publish)
		_, err := p.pipeline.Process(ctx, value)
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
