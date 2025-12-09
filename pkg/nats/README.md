# herald/pkg/nats

NATS provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/pkg/nats
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/nats-io/nats.go"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldnats "github.com/zoobzio/herald/pkg/nats"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Connect to NATS
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        log.Fatal(err)
    }

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    provider := heraldnats.New("orders", heraldnats.WithConn(nc))
    defer provider.Close()

    // Publish capitan events to NATS
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start(ctx)

    // Emit event - automatically published to NATS
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
// Create provider for subscribing (same connection)
provider := heraldnats.New("orders", heraldnats.WithConn(nc))
defer provider.Close()

// Subscribe to NATS and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from NATS
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithConn(c *nats.Conn)` | Set NATS connection |

## Metadata

NATS core does not support message headers. Metadata is ignored on publish and not populated on subscribe.

For header support, use NATS JetStream.

## Acknowledgment

NATS core uses at-most-once delivery. Ack and Nack are no-ops.

| Operation | Behavior |
|-----------|----------|
| Ack | No-op |
| Nack | No-op |

## Requirements

- Go 1.23+
- [nats-io/nats.go](https://github.com/nats-io/nats.go)
