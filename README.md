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

## Bidirectional Event Distribution

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

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    kafkago "github.com/segmentio/kafka-go"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    "github.com/zoobzio/herald/kafka"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Define signal and typed key
    orderCreated := capitan.NewSignal("order.created", "New order")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create Kafka provider
    writer := &kafkago.Writer{Addr: kafkago.TCP("localhost:9092"), Topic: "orders"}
    reader := kafkago.NewReader(kafkago.ReaderConfig{
        Brokers: []string{"localhost:9092"},
        Topic:   "orders",
        GroupID: "order-processor",
    })
    provider := kafka.New("orders", kafka.WithWriter(writer), kafka.WithReader(reader))
    defer provider.Close()

    // Publish: capitan events → Kafka
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()
    defer pub.Close()

    // Subscribe: Kafka → capitan events
    sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
    sub.Start(ctx)
    defer sub.Close()

    // Handle incoming messages with standard capitan hooks
    capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
        order, _ := orderKey.From(e)
        fmt.Printf("Received order: %s\n", order.ID)
    })

    // Emit an event — automatically published to Kafka
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{ID: "ORD-123", Total: 99.99}))

    capitan.Shutdown()
}
```

## Capabilities

| Feature              | Description                                                                                         | Docs                                                                                     |
| -------------------- | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| Bidirectional Flow   | Publish capitan events to brokers or subscribe broker messages as events                            | [Publishing](docs/2.learn/1.publishing.md), [Subscribing](docs/2.learn/2.subscribing.md) |
| Type-Safe Generics   | Compile-time checked publishers and subscribers                                                     | [Overview](docs/1.overview.md)                                                           |
| 11 Providers         | Kafka, NATS, JetStream, Pub/Sub, Redis, SQS, AMQP, SNS, Bolt, Firestore, io                         | [Providers](docs/2.learn/3.providers.md)                                                 |
| Pipeline Middleware  | Validation, transformation, and side effects via processors                                         | [Reliability](docs/3.guides/1.reliability.md)                                            |
| Reliability Patterns | Retry, backoff, timeout, circuit breaker, rate limiting via [pipz](https://github.com/zoobzio/pipz) | [Reliability](docs/3.guides/1.reliability.md)                                            |
| Auto Acknowledgment  | Messages acked/nacked based on processing outcome                                                   | [Subscribing](docs/2.learn/2.subscribing.md)                                             |
| Custom Codecs        | Pluggable serialization (JSON default, custom supported)                                            | [Codecs](docs/3.guides/2.codecs.md)                                                      |
| Error Observability  | All errors emit as [capitan](https://github.com/zoobzio/capitan) events                             | [Error Handling](docs/3.guides/3.errors.md)                                              |

## Why herald?

- **Type-safe** — Generic publishers and subscribers with compile-time checking
- **Bidirectional** — Publish to brokers or subscribe from brokers
- **11 providers** — Kafka, NATS, JetStream, Pub/Sub, Redis, SQS, RabbitMQ, SNS, BoltDB, Firestore, io
- **Reliable** — Pipeline middleware for retry, backoff, timeout, circuit breaker, rate limiting
- **Observable** — Errors flow through [capitan](https://github.com/zoobzio/capitan)

## Unified Event Flow

Herald enables a pattern: **internal events become external messages, external messages become internal events**.

Your application emits [capitan](https://github.com/zoobzio/capitan) events as usual. Herald publishes them to any broker. Other services publish to brokers. Herald subscribes and emits them as capitan events. Same signals, same keys, same hooks — the boundary between internal and external disappears.

```go
// Service A: emit locally, publish externally
capitan.Emit(ctx, orderCreated, orderKey.Field(order))

// Service B: subscribe externally, handle locally
capitan.Hook(orderCreated, processOrder)
```

Two services, one event type, zero coupling. The broker is just a transport.

## Providers

| Provider       | Package                  | Use Case                          |
| -------------- | ------------------------ | --------------------------------- |
| Kafka          | [`kafka`](kafka)         | High-throughput streaming         |
| NATS           | [`nats`](nats)           | Lightweight cloud messaging       |
| JetStream      | [`jetstream`](jetstream) | NATS with persistence and headers |
| Google Pub/Sub | [`pubsub`](pubsub)       | GCP managed messaging             |
| Redis Streams  | [`redis`](redis)         | In-memory with persistence        |
| AWS SQS        | [`sqs`](sqs)             | AWS managed queues                |
| RabbitMQ/AMQP  | [`amqp`](amqp)           | Traditional message broker        |
| AWS SNS        | [`sns`](sns)             | Pub/sub fanout                    |
| BoltDB         | [`bolt`](bolt)           | Embedded local queues             |
| Firestore      | [`firestore`](firestore) | Firebase/GCP document store       |
| io             | [`io`](io)               | Testing with io.Reader/Writer     |

## Processing Hooks

Add processing steps via middleware processors:

```go
pub := herald.NewPublisher(provider, signal, key, []herald.Option[Order]{
    herald.WithMiddleware(
        herald.UseApply[Order]("validate", func(ctx context.Context, env *herald.Envelope[Order]) (*herald.Envelope[Order], error) {
            if env.Value.Total < 0 {
                return env, errors.New("invalid total")
            }
            return env, nil
        }),
        herald.UseEffect[Order]("log", func(ctx context.Context, env *herald.Envelope[Order]) error {
            log.Printf("order %s", env.Value.ID)
            return nil
        }),
        herald.UseTransform[Order]("enrich", func(ctx context.Context, env *herald.Envelope[Order]) *herald.Envelope[Order] {
            env.Value.ProcessedAt = time.Now()
            return env
        }),
    ),
})
```

- `UseApply` — Transform envelope with possible error
- `UseEffect` — Side effect, envelope passes through unchanged
- `UseTransform` — Pure transform, cannot fail

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

| Outcome                        | Action                                     |
| ------------------------------ | ------------------------------------------ |
| Message processed successfully | `Ack()` — Message acknowledged             |
| Deserialization fails          | `Nack()` — Message returned for redelivery |
| Provider doesn't support ack   | No-op (e.g., NATS core, SNS)               |

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

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.
