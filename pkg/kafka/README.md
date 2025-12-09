# herald/pkg/kafka

Kafka provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/pkg/kafka
```

## Usage

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
        Topic: "orders",
    }

    // Create provider for publishing
    provider := kafka.New("orders", kafka.WithWriter(writer))
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

### Subscribing

```go
// Create Kafka reader
reader := kafkago.NewReader(kafkago.ReaderConfig{
    Brokers: []string{"localhost:9092"},
    Topic:   "orders",
    GroupID: "order-processor",
})

// Create provider for subscribing
provider := kafka.New("orders", kafka.WithReader(reader))
defer provider.Close()

// Subscribe to Kafka and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from Kafka
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithWriter(w Writer)` | Set Kafka writer for publishing |
| `WithReader(r Reader)` | Set Kafka reader for subscribing |

## Metadata

Kafka headers are mapped bidirectionally to herald metadata:

- **Publish**: `herald.Metadata` → Kafka message headers
- **Subscribe**: Kafka message headers → `herald.Metadata`

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Commits message offset |
| Nack | Does not commit; message redelivered on next consumer |

## Requirements

- Go 1.23+
- [segmentio/kafka-go](https://github.com/segmentio/kafka-go)
