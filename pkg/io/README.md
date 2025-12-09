# herald/pkg/io

io.Reader/io.Writer provider for [herald](https://github.com/zoobzio/herald).

Useful for testing, CLI piping, and file-based messaging.

## Installation

```bash
go get github.com/zoobzio/herald/pkg/io
```

## Usage

### Testing

```go
package main

import (
    "bytes"
    "context"
    "testing"

    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldio "github.com/zoobzio/herald/pkg/io"
)

type Order struct {
    ID string `json:"id"`
}

func TestPublisher(t *testing.T) {
    var buf bytes.Buffer

    orderCreated := capitan.NewSignal("order.created", "New order")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider with buffer
    provider := heraldio.New(heraldio.WithWriter(&buf))

    // Publish events
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start(context.Background())

    capitan.Emit(context.Background(), orderCreated, orderKey.Field(Order{ID: "123"}))
    capitan.Shutdown()
    pub.Close()

    // Verify output
    if !bytes.Contains(buf.Bytes(), []byte("123")) {
        t.Error("expected order ID in output")
    }
}
```

### CLI Piping

```go
package main

import (
    "context"
    "os"

    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldio "github.com/zoobzio/herald/pkg/io"
)

type Line struct {
    Text string `json:"text"`
}

func main() {
    ctx := context.Background()

    lineReceived := capitan.NewSignal("line.received", "Line from stdin")
    lineKey := capitan.NewKey[Line]("line", "app.Line")

    // Read from stdin
    provider := heraldio.New(heraldio.WithReader(os.Stdin))

    sub := herald.NewSubscriber(provider, lineReceived, lineKey, nil)
    sub.Start(ctx)

    capitan.Hook(lineReceived, func(ctx context.Context, e *capitan.Event) {
        line, _ := lineKey.From(e)
        // Process line...
    })

    // Block until EOF
    select {}
}
```

### File-based Messaging

```go
// Write to file
file, _ := os.Create("messages.jsonl")
provider := heraldio.New(heraldio.WithWriter(file))

// Read from file
file, _ := os.Open("messages.jsonl")
provider := heraldio.New(heraldio.WithReader(file))
```

### Custom Delimiter

```go
// Use null byte as delimiter
provider := heraldio.New(
    heraldio.WithReader(reader),
    heraldio.WithDelimiter(0x00),
)
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithReader(r io.Reader)` | Set reader for subscribing | - |
| `WithWriter(w io.Writer)` | Set writer for publishing | - |
| `WithDelimiter(d byte)` | Message delimiter | `'\n'` |

## Metadata

io streams do not support metadata. Metadata is ignored on publish and not populated on subscribe.

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | No-op (data already consumed) |
| Nack | No-op (cannot rewind stream) |

## Message Format

Messages are written as raw bytes followed by the delimiter (default: newline).

For JSON messages, this produces JSONL (JSON Lines) format:

```
{"id":"1","total":99.99}
{"id":"2","total":149.99}
```

## Use Cases

- Unit testing publishers and subscribers
- Integration testing with mock data
- CLI tools processing stdin/stdout
- File-based message replay
- Debug logging

## Requirements

- Go 1.23+
