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

## Quick Start

```go
package main

import (
    "context"

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
- **Composable**: Pipeline middleware via [pipz](https://github.com/zoobzio/pipz)
- **Observable**: Events flow through [capitan](https://github.com/zoobzio/capitan), observable via [shotel](https://github.com/zoobzio/shotel)

## Installation

```bash
go get github.com/zoobzio/herald
```

Install providers as needed:

```bash
go get github.com/zoobzio/herald/kafka
go get github.com/zoobzio/herald/nats
go get github.com/zoobzio/herald/sqs
# ...
```

Requirements: Go 1.23+

## Documentation

📚 **[Full Documentation](./docs/index.md)**

- [Introduction](./docs/1.learn/1.introduction.md) - Why herald and ecosystem context
- [Core Concepts](./docs/1.learn/2.core-concepts.md) - Providers, Publishers, Subscribers
- [Quick Start](./docs/2.tutorials/1.quickstart.md) - Your first integration in 5 minutes
- [Provider Guide](./docs/3.guides/2.providers.md) - All 11 providers with examples
- [API Reference](./docs/5.reference/1.api.md) - Complete API documentation

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

## Providers

| Provider | Package | Use Case |
|----------|---------|----------|
| Kafka | `herald/kafka` | High-throughput streaming |
| NATS | `herald/nats` | Lightweight cloud messaging |
| Google Pub/Sub | `herald/pubsub` | GCP managed messaging |
| Redis Streams | `herald/redis` | In-memory with persistence |
| AWS SQS | `herald/sqs` | AWS managed queues |
| RabbitMQ/AMQP | `herald/amqp` | Traditional message broker |
| AWS SNS | `herald/sns` | Pub/sub fanout |
| SQL | `herald/sql` | Database-backed queues |
| BoltDB | `herald/bolt` | Embedded local queues |
| Firestore | `herald/firestore` | Firebase/GCP document store |
| io | `herald/io` | Testing with io.Reader/Writer |

## License

MIT License - see [LICENSE](LICENSE) file for details.
