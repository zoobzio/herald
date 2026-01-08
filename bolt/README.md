# herald/bolt

BoltDB (bbolt) provider for [herald](https://github.com/zoobzio/herald).

Embedded key-value store for local message queues without external dependencies.

## Installation

```bash
go get github.com/zoobzio/herald/bolt
```

## Usage

```go
package main

import (
    "context"
    "log"

    "go.etcd.io/bbolt"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldbolt "github.com/zoobzio/herald/bolt"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Open BoltDB
    db, err := bbolt.Open("messages.db", 0600, nil)
    if err != nil {
        log.Fatal(err)
    }

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    provider := heraldbolt.New("orders", heraldbolt.WithDB(db))
    defer provider.Close()

    // Publish capitan events to BoltDB
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically stored in BoltDB
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
// Create provider for subscribing
provider := heraldbolt.New("orders",
    heraldbolt.WithDB(db),
    heraldbolt.WithPollInterval(100*time.Millisecond),
    heraldbolt.WithBatchSize(10),
)
defer provider.Close()

// Subscribe to BoltDB and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithDB(db *bbolt.DB)` | Set BoltDB connection | - |
| `WithPollInterval(d time.Duration)` | Polling interval | 100ms |
| `WithBatchSize(n int)` | Messages per poll | 10 |

## Metadata

BoltDB does not support metadata. Metadata is ignored on publish and not populated on subscribe.

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Deletes key from bucket |
| Nack | Key remains for retry on next poll |

## Storage

- Each topic maps to a BoltDB bucket
- Messages are stored with sequential uint64 keys for ordering
- Bucket is created automatically on first publish

## Use Cases

- Local development and testing
- Single-node applications
- Embedded systems
- Offline-first applications with sync

## Requirements

- Go 1.23+
- [go.etcd.io/bbolt](https://github.com/etcd-io/bbolt)
