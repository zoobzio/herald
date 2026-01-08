# herald/sns

AWS SNS provider for [herald](https://github.com/zoobzio/herald).

**Note:** SNS is publish-only. Use [SQS](../sqs) with an SNS subscription for consuming messages.

## Installation

```bash
go get github.com/zoobzio/herald/sns
```

## Usage

```go
package main

import (
    "context"
    "log"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/sns"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldsns "github.com/zoobzio/herald/sns"
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

    // Create SNS client
    client := sns.NewFromConfig(cfg)

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    topicARN := "arn:aws:sns:us-east-1:123456789:orders"
    provider := heraldsns.New(topicARN, heraldsns.WithClient(client))
    defer provider.Close()

    // Publish capitan events to SNS
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically published to SNS
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORDER-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
    pub.Close()
}
```

### Fan-out Pattern (SNS → SQS)

```go
// Publisher sends to SNS topic
snsProvider := heraldsns.New(topicARN, heraldsns.WithClient(snsClient))
pub := herald.NewPublisher(snsProvider, orderCreated, orderKey, nil)

// Multiple subscribers receive via SQS queues subscribed to the SNS topic
sqsProvider := heraldsqs.New(queueURL, heraldsqs.WithClient(sqsClient))
sub := herald.NewSubscriber(sqsProvider, orderCreated, orderKey, nil)
```

## Options

| Option | Description |
|--------|-------------|
| `WithClient(c *sns.Client)` | Set SNS client |

## Metadata

SNS message attributes are mapped from herald metadata:

- **Publish**: `herald.Metadata` → SNS message attributes (String type)

## Subscribe

SNS does not support direct subscription. The `Subscribe` method returns a closed channel.

For consuming SNS messages:
1. Create an SQS queue
2. Subscribe the queue to the SNS topic
3. Use the [SQS provider](../sqs) to consume messages

## Requirements

- Go 1.23+
- [aws-sdk-go-v2/service/sns](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/sns)
- AWS credentials configured
