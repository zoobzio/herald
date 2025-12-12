# herald

[![CI Status](https://github.com/zoobzio/herald/workflows/CI/badge.svg)](https://github.com/zoobzio/herald/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/herald/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/herald)](https://goreportcard.com/report/github.com/zoobzio/herald)
[![CodeQL](https://github.com/zoobzio/herald/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/herald/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/herald.svg)](https://pkg.go.dev/github.com/zoobzio/herald)
[![License](https://img.shields.io/github/license/zoobzio/herald)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/herald)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/herald)](https://github.com/zoobzio/herald/releases)

Bidirectional bindings between [capitan](https://github.com/zoobzio/capitan) events and message brokers.

Emit a [capitan](https://github.com/zoobzio/capitan) event, herald publishes it. Herald receives a message, capitan emits it. Same types, same signals, automatic serialization.

## Two Directions

```go
// capitan → broker: Publish events to external systems
pub := herald.NewPublisher(provider, signal, key, nil)
pub.Start()

// broker → capitan: Subscribe to external messages as events
sub := herald.NewSubscriber(provider, signal, key, nil)
sub.Start(ctx)
```

One provider, one signal, one key. Herald handles serialization, acknowledgment, and error routing.

## Installation

```bash
go get github.com/zoobzio/herald
```

Requires Go 1.23+.

## Publishing

```go
package main

import (
    "context"

    kafkago "github.com/segmentio/kafka-go"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    "github.com/zoobzio/herald/pkg/kafka"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    orderCreated := capitan.NewSignal("order.created", "New order")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    writer := &kafkago.Writer{
        Addr:  kafkago.TCP("localhost:9092"),
        Topic: "orders",
    }
    provider := kafka.New("orders", kafka.WithWriter(writer))
    defer provider.Close()

    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()
    defer pub.Close()

    // Events automatically published to Kafka
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORD-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
}
```

## Subscribing

```go
// Create provider with reader
reader := kafkago.NewReader(kafkago.ReaderConfig{
    Brokers: []string{"localhost:9092"},
    Topic:   "orders",
    GroupID: "order-processor",
})
provider := kafka.New("orders", kafka.WithReader(reader))
defer provider.Close()

// Subscribe: broker messages become capitan events
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)
defer sub.Close()

// Handle with standard capitan hooks
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

Messages are automatically deserialized, emitted as capitan events, and acknowledged on success.

## Why herald?

- **Type-safe** — Generic publishers and subscribers with compile-time checking
- **Bidirectional** — Publish to brokers or subscribe from brokers
- **12 providers** — Kafka, NATS, JetStream, Pub/Sub, Redis, SQS, RabbitMQ, SNS, SQL, BoltDB, Firestore, io
- **Reliable** — Pipeline middleware for retry, backoff, timeout, circuit breaker, rate limiting
- **Observable** — Errors flow through [capitan](https://github.com/zoobzio/capitan)

## Providers

| Provider | Package | Use Case |
|----------|---------|----------|
| Kafka | [`pkg/kafka`](pkg/kafka) | High-throughput streaming |
| NATS | [`pkg/nats`](pkg/nats) | Lightweight cloud messaging |
| JetStream | [`pkg/jetstream`](pkg/jetstream) | NATS with persistence and headers |
| Google Pub/Sub | [`pkg/pubsub`](pkg/pubsub) | GCP managed messaging |
| Redis Streams | [`pkg/redis`](pkg/redis) | In-memory with persistence |
| AWS SQS | [`pkg/sqs`](pkg/sqs) | AWS managed queues |
| RabbitMQ/AMQP | [`pkg/amqp`](pkg/amqp) | Traditional message broker |
| AWS SNS | [`pkg/sns`](pkg/sns) | Pub/sub fanout |
| SQL | [`pkg/sql`](pkg/sql) | Database-backed queues |
| BoltDB | [`pkg/bolt`](pkg/bolt) | Embedded local queues |
| Firestore | [`pkg/firestore`](pkg/firestore) | Firebase/GCP document store |
| io | [`pkg/io`](pkg/io) | Testing with io.Reader/Writer |

## Processing Hooks

Add processing steps via pipz primitives:

```go
pub := herald.NewPublisher(provider, signal, key, []herald.Option[Order]{
    herald.WithApply[Order]("validate", func(ctx context.Context, order Order) (Order, error) {
        if order.Total < 0 {
            return order, errors.New("invalid total")
        }
        return order, nil
    }),
    herald.WithEffect[Order]("log", func(ctx context.Context, order Order) error {
        log.Printf("order %s", order.ID)
        return nil
    }),
    herald.WithTransform[Order]("enrich", func(ctx context.Context, order Order) Order {
        order.ProcessedAt = time.Now()
        return order
    }),
})
```

- `WithApply` — Transform with possible error
- `WithEffect` — Side effect, no transform
- `WithTransform` — Pure transform, cannot fail

## Pipeline Options

Add reliability features via [pipz](https://github.com/zoobzio/pipz):

```go
pub := herald.NewPublisher(provider, signal, key, []herald.Option[Order]{
    herald.WithRetry[Order](3),
    herald.WithBackoff[Order](3, 100*time.Millisecond),
    herald.WithTimeout[Order](5*time.Second),
    herald.WithCircuitBreaker[Order](5, 30*time.Second),
    herald.WithRateLimit[Order](100, 10),
})
```

See [Reliability Guide](docs/3.guides/1.reliability.md) for middleware and pipeline details.

## Acknowledgment

Herald handles message acknowledgment automatically:

| Outcome | Action |
|---------|--------|
| Message processed successfully | `Ack()` — Message acknowledged |
| Deserialization fails | `Nack()` — Message returned for redelivery |
| Provider doesn't support ack | No-op (e.g., NATS core, SNS) |

## Error Handling

All errors flow through capitan:

```go
capitan.Hook(herald.ErrorSignal, func(ctx context.Context, e *capitan.Event) {
    err, _ := herald.ErrorKey.From(e)
    log.Printf("[herald] %s: %v", err.Operation, err.Err)
})
```

See [Error Handling Guide](docs/3.guides/3.errors.md) for details.

## Documentation

Full documentation is available in the [docs/](docs/) directory:

### Learn
- [Overview](docs/1.overview.md) — Architecture and philosophy
- [Publishing](docs/2.learn/1.publishing.md) — Forward capitan events to brokers
- [Subscribing](docs/2.learn/2.subscribing.md) — Consume broker messages as capitan events
- [Providers](docs/2.learn/3.providers.md) — Available broker implementations

### Guides
- [Reliability](docs/3.guides/1.reliability.md) — Retry, backoff, circuit breaker, rate limiting
- [Codecs](docs/3.guides/2.codecs.md) — Custom serialization formats
- [Error Handling](docs/3.guides/3.errors.md) — Centralized error management
- [Testing](docs/3.guides/4.testing.md) — Testing herald-based applications

### Reference
- [API Reference](docs/4.reference/1.api.md) — Complete function and type documentation
- [Providers Reference](docs/4.reference/2.providers.md) — Provider configuration details

## Contributing

Contributions welcome! Please ensure:
- Tests pass: `go test ./...`
- Code is formatted: `go fmt ./...`
- No lint errors: `golangci-lint run`

## License

MIT License — see [LICENSE](LICENSE) for details.
