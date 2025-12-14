// Package herald provides bidirectional bindings between Capitan events and distributed messaging systems.
//
// Herald bridges in-process event coordination (Capitan) with external message brokers,
// enabling seamless integration with distributed systems. Each broker is typed to a specific
// struct that represents the message contract.
//
// Publishers observe Capitan signals and forward them to broker topics.
// Subscribers consume from broker topics and emit to Capitan signals.
//
// A node should be either a Publisher OR Subscriber for a given signal, never both,
// preventing event loops in distributed topologies.
package herald

import (
	"context"
	"errors"

	"github.com/zoobzio/capitan"
)

// Sentinel errors for provider misconfiguration.
var (
	// ErrNoWriter is returned when Publish is called on a provider without a writer configured.
	ErrNoWriter = errors.New("herald: no writer configured for publishing")

	// ErrNoReader is returned when Subscribe is called on a provider without a reader configured.
	ErrNoReader = errors.New("herald: no reader configured for subscribing")
)

// Metadata holds message headers/attributes for cross-cutting concerns.
// Used for correlation IDs, tracing context, content types, and routing hints.
type Metadata map[string]string

// Envelope wraps a value with metadata for pipeline processing.
// Provides type-safe access to message headers in middleware.
type Envelope[T any] struct {
	// Value is the typed message payload.
	Value T

	// Metadata contains message headers/attributes.
	// For publishers: set headers to send with the message.
	// For subscribers: read headers received from the broker.
	Metadata Metadata
}

// Message represents a message received from a broker with acknowledgment controls.
// Ack confirms successful processing; Nack signals failure and typically triggers redelivery.
type Message struct {
	// Data is the raw message payload.
	Data []byte

	// Metadata contains message headers/attributes.
	// Maps to broker-native headers (Kafka headers, AMQP properties, SQS attributes, etc.)
	Metadata Metadata

	// Ack acknowledges successful processing.
	// The broker will not redeliver this message.
	Ack func() error

	// Nack signals processing failure.
	// The broker will typically redeliver the message (behavior varies by broker).
	Nack func() error
}

// Provider defines the interface for message broker implementations.
// Each provider handles broker-specific connection and message semantics.
//
// Message ordering depends on the underlying broker implementation.
// Most brokers (Kafka, NATS, etc.) provide ordering guarantees within a partition
// or subject, but not globally. Consult your provider's documentation for specifics.
type Provider interface {
	// Publish sends raw bytes with metadata to the broker.
	// Metadata is mapped to broker-native headers (Kafka headers, AMQP properties, etc.)
	Publish(ctx context.Context, data []byte, metadata Metadata) error

	// Subscribe returns a stream of messages from the broker.
	// Each message includes Ack/Nack functions for explicit acknowledgment.
	// Metadata is populated from broker-native headers.
	Subscribe(ctx context.Context) <-chan Result[Message]

	// Ping verifies broker connectivity.
	// Returns nil if the connection is healthy, error otherwise.
	// Use this for health checks and readiness probes.
	Ping(ctx context.Context) error

	// Close releases broker resources.
	Close() error
}

// Error signals and types for observability.
// Hook into ErrorSignal to receive notifications of operational failures.
var (
	// ErrorSignal is emitted when herald encounters an operational error.
	// This includes publish failures, subscribe errors, and unmarshal failures.
	ErrorSignal = capitan.NewSignal("herald.error", "Herald operational error")

	// ErrorKey extracts Error from events on ErrorSignal.
	ErrorKey = capitan.NewKey[Error]("error", "herald.Error")

	// MetadataKey extracts Metadata from events emitted by subscribers.
	// Use this in Capitan hooks to access broker message headers.
	MetadataKey = capitan.NewKey[Metadata]("metadata", "herald.Metadata")
)

// Result represents either a successful value or an error.
// Used for stream-based message consumption where errors and values
// flow through the same channel.
type Result[T any] struct {
	value T
	err   error
}

// NewSuccess creates a successful Result containing the given value.
func NewSuccess[T any](value T) Result[T] {
	return Result[T]{value: value}
}

// NewError creates a failed Result containing the given error.
func NewError[T any](err error) Result[T] {
	return Result[T]{err: err}
}

// IsError returns true if this Result contains an error.
func (r Result[T]) IsError() bool {
	return r.err != nil
}

// IsSuccess returns true if this Result contains a successful value.
func (r Result[T]) IsSuccess() bool {
	return r.err == nil
}

// Value returns the successful value.
// Returns the zero value if this is an error Result.
func (r Result[T]) Value() T {
	return r.value
}

// Error returns the error, or nil if this is a successful Result.
func (r Result[T]) Error() error {
	return r.err
}

// Error represents an operational error in herald.
type Error struct {
	// Operation is the operation that failed: "publish", "subscribe", or "unmarshal"
	Operation string `json:"operation"`

	// Signal is the name of the user's signal involved in the error.
	Signal string `json:"signal"`

	// Err is the error message.
	Err string `json:"error"`

	// Nack is true if the message was nack'd for redelivery.
	Nack bool `json:"nack"`

	// Raw contains the original message bytes, if available.
	// Populated for unmarshal errors to aid debugging.
	Raw []byte `json:"raw,omitempty"`
}

// copyMetadata returns a shallow copy of the metadata, or a new map if nil.
func copyMetadata(m Metadata) Metadata {
	if m == nil {
		return make(Metadata)
	}
	copied := make(Metadata, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return copied
}
