# herald/pkg/redis

Redis Streams provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/pkg/redis
```

## Usage

```go
package main

import (
    "context"

    "github.com/redis/go-redis/v9"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldredis "github.com/zoobzio/herald/pkg/redis"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Create Redis client
    client := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    provider := heraldredis.New("orders", heraldredis.WithClient(client))
    defer provider.Close()

    // Publish capitan events to Redis Stream
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start(ctx)

    // Emit event - automatically published to Redis
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORDER-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
    pub.Close()
}
```

### Subscribing with Consumer Groups

```go
// Create consumer group first (run once)
client.XGroupCreateMkStream(ctx, "orders", "order-processors", "0")

// Create provider with consumer group
provider := heraldredis.New("orders",
    heraldredis.WithClient(client),
    heraldredis.WithGroup("order-processors"),
    heraldredis.WithConsumer("worker-1"),
)
defer provider.Close()

// Subscribe to Redis Stream and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from Redis
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithClient(c *redis.Client)` | Set Redis client |
| `WithGroup(group string)` | Set consumer group for acknowledgment |
| `WithConsumer(consumer string)` | Set consumer name within group (default: "herald") |

## Metadata

Metadata is stored as additional stream fields alongside the `data` field:

- **Publish**: `herald.Metadata` → Stream field values
- **Subscribe**: Stream field values → `herald.Metadata`

## Acknowledgment

| Mode | Ack | Nack |
|------|-----|------|
| Consumer Group | `XACK` acknowledges message | Message remains pending for reclaim |
| Simple Read | No-op | No-op |

## Requirements

- Go 1.23+
- [redis/go-redis](https://github.com/redis/go-redis)
- Redis 5.0+ (Streams support)
