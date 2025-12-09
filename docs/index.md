---
title: Herald Documentation
description: Bidirectional bindings between capitan events and message brokers for Go.
author: Herald Team
published: 2025-12-06
tags: [Documentation, Overview, Getting Started]
---

# herald Documentation

Welcome to herald - bidirectional bindings between capitan events and message brokers.

## Start Here

- **[Getting Started](./2.tutorials/2.getting-started.md)** - Build your first broker integration in 10 minutes
- **[Core Concepts](./1.learn/2.core-concepts.md)** - Understand Providers, Publishers, and Subscribers
- **[API Reference](./5.reference/1.api.md)** - Complete API documentation

## Learn

- **[Introduction](./1.learn/1.introduction.md)** - Why herald exists and ecosystem context
- **[Core Concepts](./1.learn/2.core-concepts.md)** - Mental models and fundamentals
- **[Architecture](./1.learn/3.architecture.md)** - Event flow, Ack/Nack, and metadata propagation

## Tutorials

- **[Quickstart](./2.tutorials/1.quickstart.md)** - Your first publisher in 5 minutes
- **[Getting Started](./2.tutorials/2.getting-started.md)** - Complete introduction with examples
- **[First Integration](./2.tutorials/3.first-integration.md)** - Multi-broker workflow example

## Guides

- **[Testing](./3.guides/1.testing.md)** - Testing patterns with the io provider
- **[Providers](./3.guides/2.providers.md)** - All 12 providers with configuration examples
- **[Pipeline Options](./3.guides/3.pipeline-options.md)** - Timeout, backoff, and rate limiting
- **[Metadata](./3.guides/4.metadata.md)** - Header propagation across brokers
- **[Best Practices](./3.guides/5.best-practices.md)** - Production-ready patterns

## Cookbook

- **[Cookbook Overview](./4.cookbook/1.overview.md)** - Recipe collection for common patterns
- **[Multi-Broker Workflows](./4.cookbook/2.multi-broker.md)** - Cross-broker event routing
- **[Error Handling](./4.cookbook/3.error-handling.md)** - Nack patterns and retry strategies

## Reference

- **[API Reference](./5.reference/1.api.md)** - Complete API documentation
- **[Providers Reference](./5.reference/2.providers.md)** - Provider-by-provider configuration
- **[Options Reference](./5.reference/3.options.md)** - Pipeline options and middleware

## Quick Links

- [GitHub Repository](https://github.com/zoobzio/herald)
- [Go Package Documentation](https://pkg.go.dev/github.com/zoobzio/herald)
- [Test Coverage Report](https://codecov.io/gh/zoobzio/herald)

## Ecosystem

Herald is part of the zoobzio event coordination ecosystem:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Application Code                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         capitan                                  │
│              Type-safe event coordination                        │
│                  Emit → Hook/Observe                             │
└─────────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│      herald       │  │      pipz       │  │     shotel      │
│  Broker bridge  │  │    Pipelines    │  │   Observability │
│  Kafka, SQS...  │  │  Retry, Rate... │  │  OTEL bridge    │
└─────────────────┘  └─────────────────┘  └─────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Message Brokers                             │
│         Kafka │ NATS │ SQS │ Pub/Sub │ Redis │ ...              │
└─────────────────────────────────────────────────────────────────┘
```

### How They Connect

- **[capitan](https://github.com/zoobzio/capitan)** - Core event coordination. Emit signals, hook listeners.
- **[herald](https://github.com/zoobzio/herald)** - Bridge capitan events to/from message brokers.
- **[pipz](https://github.com/zoobzio/pipz)** - Pipeline middleware. Herald uses pipz for timeout/retry/rate limiting.
- **[shotel](https://github.com/zoobzio/shotel)** - OpenTelemetry bridge. Observe capitan events as logs/metrics/traces.

## Features

### Type-Safe Broker Integration

Publishers and subscribers are generic, providing compile-time type safety:

```go
type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

orderKey := capitan.NewKey[Order]("order", "app.Order")

// Type-safe publisher
pub := herald.NewPublisher[Order](provider, signal, orderKey, nil)

// Type-safe subscriber
sub := herald.NewSubscriber[Order](provider, signal, orderKey, nil)
```

### 12 Provider Implementations

From cloud-managed services to embedded databases:

| Provider | Use Case |
|----------|----------|
| Kafka | High-throughput streaming |
| NATS | Lightweight cloud messaging |
| Google Pub/Sub | GCP managed messaging |
| Redis Streams | In-memory with persistence |
| AWS SQS | AWS managed queues |
| RabbitMQ/AMQP | Traditional message broker |
| AWS SNS | Pub/sub fanout |
| SQL | Database-backed queues |
| BoltDB | Embedded local queues |
| Firestore | Firebase/GCP document store |
| io | Testing with io.Reader/Writer |

### Reliable Message Handling

Built-in Ack/Nack semantics for at-least-once delivery:

```go
// Messages include acknowledgment controls
type Message struct {
    Data     []byte
    Metadata Metadata
    Ack      func() error  // Confirm processing
    Nack     func() error  // Trigger redelivery
}
```

### Pipeline Middleware

Wrap operations with reliability patterns via pipz:

```go
opts := []herald.Option[Order]{
    herald.WithTimeout[Order](5 * time.Second),
    herald.WithBackoff[Order](3, 100*time.Millisecond),
    herald.WithRateLimit[Order](100, 10),
}

pub := herald.NewPublisher(provider, signal, key, opts)
```

### Metadata Propagation

Headers flow through the pipeline, mapped to broker-native formats:

```go
ctx := herald.ContextWithMetadata(ctx, herald.Metadata{
    "correlation-id": "abc-123",
    "trace-id":       "xyz-789",
})

capitan.Emit(ctx, signal, key.Field(order))
// Metadata becomes Kafka headers, SQS attributes, etc.
```

### Pluggable Codecs

JSON by default, with support for custom serialization formats:

```go
type MsgpackCodec struct{}

func (MsgpackCodec) Marshal(v any) ([]byte, error)     { return msgpack.Marshal(v) }
func (MsgpackCodec) Unmarshal(data []byte, v any) error { return msgpack.Unmarshal(data, v) }
func (MsgpackCodec) ContentType() string                { return "application/msgpack" }

pub := herald.NewPublisher(provider, signal, key, nil,
    herald.WithPublisherCodec[Order](MsgpackCodec{}),
)
```
