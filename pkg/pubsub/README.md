# herald/pkg/pubsub

Google Cloud Pub/Sub provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/pkg/pubsub
```

## Usage

```go
package main

import (
    "context"
    "log"

    "cloud.google.com/go/pubsub"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldpubsub "github.com/zoobzio/herald/pkg/pubsub"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Create Pub/Sub client
    client, err := pubsub.NewClient(ctx, "my-project")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Get topic for publishing
    topic := client.Topic("orders")

    // Create provider
    provider := heraldpubsub.New(heraldpubsub.WithTopic(topic))
    defer provider.Close()

    // Publish capitan events to Pub/Sub
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start(ctx)

    // Emit event - automatically published to Pub/Sub
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
// Get subscription for consuming
subscription := client.Subscription("orders-sub")

// Create provider for subscribing
provider := heraldpubsub.New(heraldpubsub.WithSubscription(subscription))
defer provider.Close()

// Subscribe to Pub/Sub and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from Pub/Sub
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithTopic(t *pubsub.Topic)` | Set Pub/Sub topic for publishing |
| `WithSubscription(s *pubsub.Subscription)` | Set Pub/Sub subscription for subscribing |

## Metadata

Pub/Sub attributes are mapped bidirectionally to herald metadata:

- **Publish**: `herald.Metadata` → Pub/Sub message attributes
- **Subscribe**: Pub/Sub message attributes → `herald.Metadata`

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Calls `msg.Ack()` |
| Nack | Calls `msg.Nack()`; message redelivered |

## Requirements

- Go 1.23+
- [cloud.google.com/go/pubsub](https://pkg.go.dev/cloud.google.com/go/pubsub)
- GCP project with Pub/Sub enabled
