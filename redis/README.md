# herald/redis

Redis Streams provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/redis
```

## Usage

```go
package main

import (
    "context"

    "github.com/redis/go-redis/v9"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldredis "github.com/zoobzio/herald/redis"
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
    pub.Start()

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
// Create consumer group (idempotent - safe to call on every startup)
err := client.XGroupCreateMkStream(ctx, "orders", "order-processors", "0").Err()
if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
    log.Fatal(err)
}

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

**Note:** `XGroupCreateMkStream` creates both the stream and consumer group if they don't exist. The `BUSYGROUP` error indicates the group already exists—this is expected on subsequent startups and can be safely ignored.

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
