# herald/pkg/firestore

Google Cloud Firestore provider for [herald](https://github.com/zoobzio/herald).

Uses Firestore collections as message queues with real-time document watching.

## Installation

```bash
go get github.com/zoobzio/herald/pkg/firestore
```

## Usage

```go
package main

import (
    "context"
    "log"

    "cloud.google.com/go/firestore"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldfirestore "github.com/zoobzio/herald/pkg/firestore"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Create Firestore client
    client, err := firestore.NewClient(ctx, "my-project")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    provider := heraldfirestore.New("orders", heraldfirestore.WithClient(client))
    defer provider.Close()

    // Publish capitan events to Firestore
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically stored in Firestore
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
provider := heraldfirestore.New("orders", heraldfirestore.WithClient(client))
defer provider.Close()

// Subscribe to Firestore and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithClient(c *firestore.Client)` | Set Firestore client |

## Metadata

Metadata is stored in the document alongside the data:

```go
type document struct {
    Data     []byte            `firestore:"data"`
    Metadata map[string]string `firestore:"metadata,omitempty"`
}
```

- **Publish**: `herald.Metadata` → Document `metadata` field
- **Subscribe**: Document `metadata` field → `herald.Metadata`

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Deletes document from collection |
| Nack | Document remains for retry |

## Real-time Watching

The provider uses Firestore's `Snapshots()` for real-time updates:

- Watches collection for `DocumentAdded` changes
- No polling required
- Low latency message delivery

## Use Cases

- Firebase applications
- Serverless architectures (Cloud Functions)
- Mobile app backends
- Cross-platform sync

## Requirements

- Go 1.23+
- [cloud.google.com/go/firestore](https://pkg.go.dev/cloud.google.com/go/firestore)
- GCP project with Firestore enabled
