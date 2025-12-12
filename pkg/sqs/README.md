# herald/pkg/sqs

AWS SQS provider for [herald](https://github.com/zoobzio/herald).

## Installation

```bash
go get github.com/zoobzio/herald/pkg/sqs
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/sqs"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldsqs "github.com/zoobzio/herald/pkg/sqs"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Load AWS config
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Create SQS client
    client := sqs.NewFromConfig(cfg)

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    queueURL := "https://sqs.us-east-1.amazonaws.com/123456789/orders"
    provider := heraldsqs.New(queueURL, heraldsqs.WithClient(client))
    defer provider.Close()

    // Publish capitan events to SQS
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically published to SQS
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
provider := heraldsqs.New(queueURL, heraldsqs.WithClient(client))
defer provider.Close()

// Subscribe to SQS and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start()

// Hook listener to handle events from SQS
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Options

| Option | Description |
|--------|-------------|
| `WithClient(c *sqs.Client)` | Set SQS client |

## Metadata

SQS message attributes are mapped bidirectionally to herald metadata:

- **Publish**: `herald.Metadata` → SQS message attributes (String type)
- **Subscribe**: SQS message attributes → `herald.Metadata`

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Deletes message from queue |
| Nack | Message returns to queue after visibility timeout |

## Long Polling

The provider uses long polling (20 seconds) for efficient message retrieval.

## Requirements

- Go 1.23+
- [aws-sdk-go-v2/service/sqs](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/sqs)
- AWS credentials configured
