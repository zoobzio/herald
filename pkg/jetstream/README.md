# herald/pkg/jetstream

JetStream provider for [herald](https://github.com/zoobzio/herald).

JetStream is NATS' persistence layer, providing at-least-once delivery, message replay, and header support. Use this provider when you need persistence and metadata support beyond NATS core.

## Installation

```bash
go get github.com/zoobzio/herald/pkg/jetstream
```

## Usage

```go
package main

import (
    "context"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    herald_js "github.com/zoobzio/herald/pkg/jetstream"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Connect to NATS
    nc, _ := nats.Connect("nats://localhost:4222")
    defer nc.Close()

    // Create JetStream context
    js, _ := jetstream.New(nc)

    // Create or get stream
    js.CreateStream(ctx, jetstream.StreamConfig{
        Name:     "ORDERS",
        Subjects: []string{"orders.>"},
    })

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider for publishing
    provider := herald_js.New("orders.created", herald_js.WithJetStream(js))
    defer provider.Close()

    // Publish capitan events to JetStream
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically published to JetStream
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORDER-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
    pub.Close()
}
```

### Subscribing

```go
// Create consumer
consumer, _ := js.CreateConsumer(ctx, "ORDERS", jetstream.ConsumerConfig{
    Durable:   "order-processor",
    AckPolicy: jetstream.AckExplicitPolicy,
})

// Create provider for subscribing
provider := herald_js.New("orders.created",
    herald_js.WithJetStream(js),
    herald_js.WithConsumer(consumer),
)
defer provider.Close()

// Subscribe to JetStream and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from JetStream
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithJetStream(js jetstream.JetStream)` | Set JetStream context for publishing |
| `WithConsumer(c jetstream.Consumer)` | Set consumer for subscribing |
| `WithStream(name string)` | Set stream name (optional, for publishing) |

## Metadata

JetStream headers are mapped bidirectionally to herald metadata:

- **Publish**: `herald.Metadata` → NATS message headers
- **Subscribe**: NATS message headers → `herald.Metadata`

Unlike NATS core, JetStream fully supports message headers.

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | `msg.Ack()` — Message acknowledged, won't be redelivered |
| Nack | `msg.Nak()` — Message returned for redelivery |

## JetStream vs NATS Core

| Feature | NATS Core | JetStream |
|---------|-----------|-----------|
| Persistence | No | Yes |
| Headers/Metadata | No | Yes |
| Ack/Nack | No-op | Full support |
| Replay | No | Yes |
| At-least-once | No | Yes |

Use NATS core (`pkg/nats`) for simple fire-and-forget messaging. Use JetStream when you need durability, acknowledgments, or metadata.

## Requirements

- Go 1.23+
- [nats-io/nats.go](https://github.com/nats-io/nats.go) v1.47+
- NATS server with JetStream enabled
