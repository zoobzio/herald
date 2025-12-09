# herald

[![CI Status](https://github.com/zoobzio/herald/workflows/CI/badge.svg)](https://github.com/zoobzio/herald/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/herald/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/herald)](https://goreportcard.com/report/github.com/zoobzio/herald)
[![CodeQL](https://github.com/zoobzio/herald/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/herald/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/herald.svg)](https://pkg.go.dev/github.com/zoobzio/herald)
[![License](https://img.shields.io/github/license/zoobzio/herald)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/herald)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/herald)](https://github.com/zoobzio/herald/releases)

Bidirectional bindings between capitan events and message brokers.

Herald bridges in-process event coordination ([capitan](https://github.com/zoobzio/capitan)) with external message brokers, enabling seamless integration with distributed systems.

## The Power of Simplicity

At its core, herald provides just two operations:

```go
// Publish capitan events to a broker
pub := herald.NewPublisher(provider, signal, key, nil)
pub.Start(ctx)

// Subscribe from a broker to capitan events
sub := herald.NewSubscriber(provider, signal, key, nil)
sub.Start(ctx)
```

**That's it.** Type-safe, bidirectional message bridging with automatic serialization and acknowledgment.

## Quick Start

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

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create Kafka writer
    writer := &kafkago.Writer{
        Addr:  kafkago.TCP("localhost:9092"),
        Topic: "orders-topic",
    }

    // Create Kafka provider
    provider := kafka.New("orders-topic", kafka.WithWriter(writer))
    defer provider.Close()

    // Publish capitan events to Kafka
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start(ctx)

    // Emit event - automatically published to Kafka
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORDER-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
    pub.Close()
}
```

## Why herald?

- **Type-safe**: Generic publishers and subscribers with compile-time checking
- **Bidirectional**: Publish to brokers or subscribe from brokers
- **11 providers**: Kafka, NATS, Pub/Sub, Redis, SQS, RabbitMQ, SNS, SQL, BoltDB, Firestore, io
- **Reliable**: Built-in Ack/Nack semantics for message acknowledgment
- **Resilient**: Pipeline middleware for retry, backoff, timeout, circuit breaker, rate limiting
- **Composable**: Pipeline middleware via [pipz](https://github.com/zoobzio/pipz)
- **Observable**: Errors flow through [capitan](https://github.com/zoobzio/capitan), observable via [shotel](https://github.com/zoobzio/shotel)

## Installation

```bash
go get github.com/zoobzio/herald
```

Install providers as needed:

```bash
go get github.com/zoobzio/herald/pkg/kafka
go get github.com/zoobzio/herald/pkg/nats
go get github.com/zoobzio/herald/pkg/sqs
# ...
```

Requirements: Go 1.23+

## Core Concepts

**Providers** implement broker-specific communication:
```go
provider := kafka.New("orders", kafka.WithWriter(writer))
provider := nats.New("orders", nats.WithConn(conn))
provider := sqs.New(queueURL, sqs.WithClient(client))
```

**Publishers** observe capitan signals and publish to brokers:
```go
pub := herald.NewPublisher(provider, signal, key, nil)
pub.Start(ctx)
// capitan events on 'signal' are now published to the broker
```

**Subscribers** consume from brokers and emit to capitan:
```go
sub := herald.NewSubscriber(provider, signal, key, nil)
sub.Start(ctx)
// Broker messages are now emitted to capitan on 'signal'
```

**Keys** define the typed message contract:
```go
type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

orderKey := capitan.NewKey[Order]("order", "app.Order")
```

### Best Practice: One Direction Per Node

A node should be either a Publisher OR Subscriber for a given signal, never both. This prevents event loops in distributed topologies.

```go
// Service A: Publishes order events to Kafka
pub := herald.NewPublisher(kafkaProvider, orderCreated, orderKey, nil)

// Service B: Subscribes to order events from Kafka
sub := herald.NewSubscriber(kafkaProvider, orderCreated, orderKey, nil)
```

## Real-World Example

Here's a realistic example showing bidirectional communication between services:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    kafkago "github.com/segmentio/kafka-go"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    "github.com/zoobzio/herald/pkg/kafka"
)

// Message contracts
type Order struct {
    ID     string  `json:"id"`
    UserID string  `json:"user_id"`
    Total  float64 `json:"total"`
}

type OrderShipped struct {
    OrderID   string `json:"order_id"`
    TrackingN string `json:"tracking_number"`
}

// Signals
var (
    orderCreated = capitan.NewSignal("order.created", "New order placed")
    orderShipped = capitan.NewSignal("order.shipped", "Order has shipped")
)

// Keys
var (
    orderKey   = capitan.NewKey[Order]("order", "app.Order")
    shippedKey = capitan.NewKey[OrderShipped]("shipped", "app.OrderShipped")
)

func main() {
    ctx := context.Background()

    // Kafka writer for publishing
    writer := &kafkago.Writer{
        Addr:  kafkago.TCP("localhost:9092"),
        Topic: "orders",
    }

    // Kafka reader for subscribing to shipping updates
    reader := kafkago.NewReader(kafkago.ReaderConfig{
        Brokers: []string{"localhost:9092"},
        Topic:   "shipments",
        GroupID: "order-service",
    })

    // Providers
    orderProvider := kafka.New("orders", kafka.WithWriter(writer))
    shipmentProvider := kafka.New("shipments", kafka.WithReader(reader))
    defer orderProvider.Close()
    defer shipmentProvider.Close()

    // Publish order events to Kafka with retry
    pub := herald.NewPublisher(orderProvider, orderCreated, orderKey,
        []herald.Option[Order]{
            herald.WithRetry[Order](3),
            herald.WithTimeout[Order](5 * time.Second),
        },
    )
    pub.Start(ctx)

    // Subscribe to shipping updates from Kafka
    sub := herald.NewSubscriber(shipmentProvider, orderShipped, shippedKey, nil)
    sub.Start(ctx)

    // Handle shipping updates locally
    capitan.Hook(orderShipped, func(ctx context.Context, e *capitan.Event) {
        shipped, _ := shippedKey.From(e)
        fmt.Printf("Order %s shipped with tracking: %s\n",
            shipped.OrderID, shipped.TrackingN)
    })

    // Hook into herald errors for monitoring
    capitan.Hook(herald.ErrorSignal, func(ctx context.Context, e *capitan.Event) {
        err, _ := herald.ErrorKey.From(e)
        log.Printf("[HERALD ERROR] %s on %s: %s", err.Operation, err.Signal, err.Err)
    })

    // Emit an order event - automatically published to Kafka
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:     "ORDER-123",
        UserID: "user_456",
        Total:  99.99,
    }))

    // Keep running...
    select {}
}
```

## Pipeline Options

Add reliability features to publishers and subscribers:

```go
pub := herald.NewPublisher(provider, signal, key, []herald.Option[Order]{
    herald.WithRetry[Order](3),                              // Retry up to 3 times
    herald.WithBackoff[Order](3, 100*time.Millisecond),      // Exponential backoff
    herald.WithTimeout[Order](5*time.Second),                // Timeout per operation
    herald.WithCircuitBreaker[Order](5, 30*time.Second),     // Open after 5 failures
    herald.WithRateLimit[Order](100, 10),                    // 100 ops/sec, burst 10
})
```

| Option | Description |
|--------|-------------|
| `WithRetry[T](n)` | Retry failed operations up to n times |
| `WithBackoff[T](n, delay)` | Retry with exponential backoff (delay doubles each attempt) |
| `WithTimeout[T](d)` | Cancel operations exceeding duration |
| `WithCircuitBreaker[T](failures, recovery)` | Open circuit after consecutive failures |
| `WithRateLimit[T](rate, burst)` | Limit operations per second |
| `WithErrorHandler[T](handler)` | Custom error handling pipeline |
| `WithPipeline[T](custom)` | Full custom pipeline control |

## Custom Codecs

By default, herald uses JSON serialization. To use an alternative format:

```go
type MsgpackCodec struct{}

func (MsgpackCodec) Marshal(v any) ([]byte, error) {
    return msgpack.Marshal(v)
}

func (MsgpackCodec) Unmarshal(data []byte, v any) error {
    return msgpack.Unmarshal(data, v)
}

func (MsgpackCodec) ContentType() string {
    return "application/msgpack"
}

// Use with publisher
pub := herald.NewPublisher(provider, signal, key, nil,
    herald.WithPublisherCodec[Order](MsgpackCodec{}))

// Use with subscriber
sub := herald.NewSubscriber(provider, signal, key, nil,
    herald.WithSubscriberCodec[Order](MsgpackCodec{}))
```

The codec's `ContentType()` is automatically added to message metadata unless already present.

## Metadata

Metadata flows through the system for cross-cutting concerns like tracing and correlation:

```go
// Attach metadata to context before emitting
ctx := herald.ContextWithMetadata(ctx, herald.Metadata{
    "correlation-id": "abc-123",
    "trace-id":       "xyz-789",
})
capitan.Emit(ctx, signal, key.Field(value))

// Extract metadata in handlers (populated by subscriber)
capitan.Hook(signal, func(ctx context.Context, e *capitan.Event) {
    meta := herald.MetadataFromContext(ctx)
    correlationID := meta["correlation-id"]
})
```

Metadata maps to broker-native headers (Kafka headers, AMQP properties, SQS attributes, etc.).

## Error Handling

All operational errors flow through capitan's event system:

```go
// Hook into herald errors
capitan.Hook(herald.ErrorSignal, func(ctx context.Context, e *capitan.Event) {
    err, _ := herald.ErrorKey.From(e)

    log.Printf("Operation: %s", err.Operation)  // "publish", "subscribe", "unmarshal"
    log.Printf("Signal: %s", err.Signal)        // User's signal name
    log.Printf("Error: %s", err.Err)            // Error message
    log.Printf("Nack'd: %t", err.Nack)          // Was message nack'd for redelivery?

    if err.Raw != nil {
        log.Printf("Raw payload: %s", err.Raw)  // For unmarshal errors
    }
})
```

## Acknowledgment Semantics

Each provider implements broker-appropriate acknowledgment:

| Provider | Ack | Nack |
|----------|-----|------|
| Kafka | Commit offset | Don't commit (redelivered) |
| NATS | No-op | No-op |
| Pub/Sub | `msg.Ack()` | `msg.Nack()` |
| Redis | `XACK` (consumer groups) | Remains pending |
| SQS | Delete message | Returns after visibility timeout |
| AMQP | `Ack(false)` | `Nack(false, true)` with requeue |
| SNS | N/A (publish-only) | N/A |
| SQL | Delete row | Remains for retry |
| BoltDB | Delete key | Remains for retry |
| Firestore | Delete document | Remains for retry |
| io | No-op | No-op |

## Multiple Capitan Instances

Use custom capitan instances for isolation:

```go
c := capitan.New(capitan.WithBufferSize(256))

pub := herald.NewPublisher(provider, signal, key, nil,
    herald.WithPublisherCapitan[Order](c))

sub := herald.NewSubscriber(provider, signal, key, nil,
    herald.WithSubscriberCapitan[Order](c))
```

## Providers

| Provider | Package | Use Case |
|----------|---------|----------|
| Kafka | [`herald/pkg/kafka`](pkg/kafka) | High-throughput streaming |
| NATS | [`herald/pkg/nats`](pkg/nats) | Lightweight cloud messaging |
| Google Pub/Sub | [`herald/pkg/pubsub`](pkg/pubsub) | GCP managed messaging |
| Redis Streams | [`herald/pkg/redis`](pkg/redis) | In-memory with persistence |
| AWS SQS | [`herald/pkg/sqs`](pkg/sqs) | AWS managed queues |
| RabbitMQ/AMQP | [`herald/pkg/amqp`](pkg/amqp) | Traditional message broker |
| AWS SNS | [`herald/pkg/sns`](pkg/sns) | Pub/sub fanout |
| SQL | [`herald/pkg/sql`](pkg/sql) | Database-backed queues |
| BoltDB | [`herald/pkg/bolt`](pkg/bolt) | Embedded local queues |
| Firestore | [`herald/pkg/firestore`](pkg/firestore) | Firebase/GCP document store |
| io | [`herald/pkg/io`](pkg/io) | Testing with io.Reader/Writer |

## Custom Providers

Implement the `Provider` interface for custom brokers:

```go
type Provider interface {
    Publish(ctx context.Context, data []byte, metadata Metadata) error
    Subscribe(ctx context.Context) <-chan Result[Message]
    Close() error
}

type Message struct {
    Data     []byte
    Metadata Metadata
    Ack      func() error
    Nack     func() error
}
```

## Testing

Run tests:
```bash
go test -v ./...
```

Run with coverage:
```bash
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Contributing

Contributions welcome! Please ensure:
- Tests pass: `go test ./...`
- Code is formatted: `go fmt ./...`
- No lint errors: `golangci-lint run`

## License

MIT License - see [LICENSE](LICENSE) file for details.
