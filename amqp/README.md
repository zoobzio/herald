# herald/amqp

RabbitMQ (AMQP 0-9-1) provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/amqp
```

## Usage

```go
package main

import (
    "context"
    "log"

    amqp "github.com/rabbitmq/amqp091-go"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldamqp "github.com/zoobzio/herald/amqp"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Connect to RabbitMQ
    conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Create channel
    ch, err := conn.Channel()
    if err != nil {
        log.Fatal(err)
    }

    // Declare queue
    q, err := ch.QueueDeclare("orders", true, false, false, false, nil)
    if err != nil {
        log.Fatal(err)
    }

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider for publishing
    provider := heraldamqp.New(q.Name, heraldamqp.WithChannel(ch))
    defer provider.Close()

    // Publish capitan events to RabbitMQ
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically published to RabbitMQ
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
provider := heraldamqp.New(q.Name, heraldamqp.WithChannel(ch))
defer provider.Close()

// Subscribe to RabbitMQ and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events from RabbitMQ
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

### Publishing to an Exchange

```go
provider := heraldamqp.New("orders",
    heraldamqp.WithChannel(ch),
    heraldamqp.WithExchange("orders-exchange"),
    heraldamqp.WithRoutingKey("orders.created"),
)
```

## Options

| Option | Description |
|--------|-------------|
| `WithChannel(ch *amqp.Channel)` | Set AMQP channel |
| `WithExchange(exchange string)` | Set exchange for publishing (default: "") |
| `WithRoutingKey(key string)` | Set routing key for publishing |

## Metadata

AMQP headers are mapped bidirectionally to herald metadata:

- **Publish**: `herald.Metadata` → AMQP message headers
- **Subscribe**: AMQP message headers → `herald.Metadata` (string values only)

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | `delivery.Ack(false)` |
| Nack | `delivery.Nack(false, true)` with requeue |

## Requirements

- Go 1.23+
- [rabbitmq/amqp091-go](https://github.com/rabbitmq/amqp091-go)
- RabbitMQ server
