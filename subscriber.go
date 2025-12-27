package herald

import (
	"context"
	"sync"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Internal identities for subscriber.
var (
	emitID              = pipz.NewIdentity("herald:emit", "Emits to Capitan signal")
	subscribePipelineID = pipz.NewIdentity("herald:subscriber", "Subscriber pipeline")
)

// Subscriber consumes from a broker and emits events to Capitan.
// T is the struct type representing the message contract.
type Subscriber[T any] struct {
	provider Provider
	signal   capitan.Signal
	key      capitan.GenericKey[T]
	capitan  *capitan.Capitan
	codec    Codec
	pipeline *pipz.Pipeline[*Envelope[T]]
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// SubscriberOption configures a Subscriber.
type SubscriberOption[T any] func(*Subscriber[T])

// WithSubscriberCapitan sets a custom Capitan instance for the subscriber.
func WithSubscriberCapitan[T any](c *capitan.Capitan) SubscriberOption[T] {
	return func(s *Subscriber[T]) {
		s.capitan = c
	}
}

// WithSubscriberCodec sets a custom codec for the subscriber.
// If not specified, JSONCodec is used.
func WithSubscriberCodec[T any](c Codec) SubscriberOption[T] {
	return func(s *Subscriber[T]) {
		s.codec = c
	}
}

// NewSubscriber creates a Subscriber that consumes from the broker and emits T to the given signal.
//
// Parameters:
//   - provider: broker implementation (kafka, nats, sqs, etc.)
//   - signal: capitan signal to emit events to
//   - key: typed key for creating fields from T
//   - pipelineOpts: reliability middleware (retry, timeout, circuit breaker); nil for none
//   - opts: subscriber configuration (custom codec, custom capitan instance)
func NewSubscriber[T any](provider Provider, signal capitan.Signal, key capitan.GenericKey[T], pipelineOpts []Option[T], opts ...SubscriberOption[T]) *Subscriber[T] {
	s := &Subscriber[T]{
		provider: provider,
		signal:   signal,
		key:      key,
		codec:    JSONCodec{},
	}
	for _, opt := range opts {
		opt(s)
	}

	// Guard against nil codec
	if s.codec == nil {
		s.codec = JSONCodec{}
	}

	// Build pipeline: start with terminal, wrap with options
	chain := newSubscribeTerminal[T](signal, key, s)
	for _, opt := range pipelineOpts {
		chain = opt(chain)
	}
	s.pipeline = pipz.NewPipeline(subscribePipelineID, chain)

	return s
}

// newSubscribeTerminal creates the terminal operation that emits to Capitan.
func newSubscribeTerminal[T any](signal capitan.Signal, key capitan.GenericKey[T], s *Subscriber[T]) pipz.Chainable[*Envelope[T]] {
	return pipz.Effect(emitID, func(ctx context.Context, env *Envelope[T]) error {
		valueField := key.Field(env.Value)
		metaField := MetadataKey.Field(env.Metadata)
		if s.capitan != nil {
			s.capitan.Emit(ctx, signal, valueField, metaField)
		} else {
			capitan.Emit(ctx, signal, valueField, metaField)
		}
		return nil
	})
}

// Start begins consuming from the broker and emitting to Capitan.
// The provided context controls the subscriber's lifetime; canceling it stops consumption.
func (s *Subscriber[T]) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	messages := s.provider.Subscribe(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-messages:
				if !ok {
					return
				}
				if result.IsError() {
					s.emitError(ctx, "subscribe", result.Error().Error(), false, nil)
					continue
				}
				s.process(ctx, result.Value())
			}
		}
	}()
}

// process deserializes the message, runs through the pipeline, and acks/nacks.
func (s *Subscriber[T]) process(ctx context.Context, msg Message) {
	var value T
	if err := s.codec.Unmarshal(msg.Data, &value); err != nil {
		// Invalid payload - nack and move on
		if msg.Nack != nil {
			if nackErr := msg.Nack(); nackErr != nil {
				s.emitError(ctx, "nack", nackErr.Error(), false, nil)
			}
		}
		s.emitError(ctx, "unmarshal", err.Error(), true, msg.Data)
		return
	}

	// Wrap value in envelope with metadata from broker
	env := &Envelope[T]{
		Value:    value,
		Metadata: copyMetadata(msg.Metadata),
	}

	// Process through pipeline (includes emit)
	_, err := s.pipeline.Process(ctx, env)

	if err != nil {
		// Pipeline failed - nack for redelivery
		if msg.Nack != nil {
			if nackErr := msg.Nack(); nackErr != nil {
				s.emitError(ctx, "nack", nackErr.Error(), false, nil)
			}
		}
		s.emitError(ctx, "subscribe", err.Error(), true, nil)
	} else if msg.Ack != nil {
		// Success - ack
		if ackErr := msg.Ack(); ackErr != nil {
			s.emitError(ctx, "ack", ackErr.Error(), false, nil)
		}
	}
}

// emitError emits an error event to ErrorSignal.
func (s *Subscriber[T]) emitError(ctx context.Context, operation, errMsg string, nack bool, raw []byte) {
	e := Error{
		Operation: operation,
		Signal:    s.signal.Name(),
		Err:       errMsg,
		Nack:      nack,
		Raw:       raw,
	}
	if s.capitan != nil {
		s.capitan.Emit(ctx, ErrorSignal, ErrorKey.Field(e))
	} else {
		capitan.Emit(ctx, ErrorSignal, ErrorKey.Field(e))
	}
}

// Close stops the subscriber and waits for the goroutine to exit.
func (s *Subscriber[T]) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	if s.pipeline != nil {
		return s.pipeline.Close()
	}
	return nil
}
