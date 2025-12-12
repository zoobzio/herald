# herald/pkg/sql

SQL database provider for [herald](https://github.com/zoobzio/herald).

Supports PostgreSQL, MySQL, and SQLite via polling-based message queues.

## Installation

```bash
go get github.com/zoobzio/herald/pkg/sql
```

### Database Drivers

Install the driver for your database:

```bash
# PostgreSQL
go get github.com/lib/pq

# MySQL
go get github.com/go-sql-driver/mysql

# SQLite
go get github.com/mattn/go-sqlite3
```

## Usage

### PostgreSQL

```go
package main

import (
    "context"
    "database/sql"
    "log"

    _ "github.com/lib/pq"
    "github.com/zoobzio/capitan"
    "github.com/zoobzio/herald"
    heraldsql "github.com/zoobzio/herald/pkg/sql"
)

type Order struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    // Connect to PostgreSQL
    db, err := sql.Open("postgres", "postgres://user:pass@localhost/mydb?sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }

    // Create PostgreSQL adapter (creates table if not exists)
    postgresDB, err := heraldsql.NewPostgres(db)
    if err != nil {
        log.Fatal(err)
    }

    // Define signal and key
    orderCreated := capitan.NewSignal("order.created", "New order created")
    orderKey := capitan.NewKey[Order]("order", "app.Order")

    // Create provider
    provider := heraldsql.New("orders", heraldsql.WithDB(postgresDB))
    defer provider.Close()

    // Publish capitan events to database
    pub := herald.NewPublisher(provider, orderCreated, orderKey, nil)
    pub.Start()

    // Emit event - automatically inserted into database
    capitan.Emit(ctx, orderCreated, orderKey.Field(Order{
        ID:    "ORDER-123",
        Total: 99.99,
    }))

    capitan.Shutdown()
    pub.Close()
}
```

### MySQL

```go
import _ "github.com/go-sql-driver/mysql"

db, err := sql.Open("mysql", "user:pass@tcp(localhost:3306)/mydb")
if err != nil {
    log.Fatal(err)
}

mysqlDB, err := heraldsql.NewMySQL(db)
if err != nil {
    log.Fatal(err)
}

provider := heraldsql.New("orders", heraldsql.WithDB(mysqlDB))
```

### SQLite

```go
import _ "github.com/mattn/go-sqlite3"

db, err := sql.Open("sqlite3", "./messages.db")
if err != nil {
    log.Fatal(err)
}

sqliteDB, err := heraldsql.NewSQLite(db)
if err != nil {
    log.Fatal(err)
}

provider := heraldsql.New("orders", heraldsql.WithDB(sqliteDB))
```

### Subscribing

```go
// Create provider for subscribing
provider := heraldsql.New("orders",
    heraldsql.WithDB(postgresDB),
    heraldsql.WithPollInterval(100*time.Millisecond),
    heraldsql.WithBatchSize(10),
)
defer provider.Close()

// Subscribe to database and emit to capitan
sub := herald.NewSubscriber(provider, orderCreated, orderKey, nil)
sub.Start(ctx)

// Hook listener to handle events
capitan.Hook(orderCreated, func(ctx context.Context, e *capitan.Event) {
    order, _ := orderKey.From(e)
    fmt.Printf("Received order: %s\n", order.ID)
})
```

## Provider Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithDB(db DB)` | Set database adapter | - |
| `WithPollInterval(d time.Duration)` | Polling interval | 100ms |
| `WithBatchSize(n int)` | Messages per poll | 10 |

## Database Adapter Options

### PostgreSQL

| Option | Description | Default |
|--------|-------------|---------|
| `WithPostgresTable(name string)` | Table name | "herald_messages" |

### MySQL

| Option | Description | Default |
|--------|-------------|---------|
| `WithMySQLTable(name string)` | Table name | "herald_messages" |

### SQLite

| Option | Description | Default |
|--------|-------------|---------|
| `WithSQLiteTable(name string)` | Table name | "herald_messages" |

## Metadata

Metadata is stored as JSON in the `metadata` column:

- **Publish**: `herald.Metadata` → JSON column
- **Subscribe**: JSON column → `herald.Metadata`

## Acknowledgment

| Operation | Behavior |
|-----------|----------|
| Ack | Deletes row from table |
| Nack | Row remains for retry on next poll |

## Table Schema

The adapters automatically create a table with this structure:

| Column | Type | Description |
|--------|------|-------------|
| id | UUID/TEXT | Primary key |
| topic | VARCHAR/TEXT | Topic name for filtering |
| data | BLOB/BYTEA | Message payload |
| metadata | JSON/JSONB/TEXT | Message metadata |
| created_at | TIMESTAMP | Ordering |

## Custom DB Implementation

Implement the `DB` interface for other databases:

```go
type DB interface {
    Insert(ctx context.Context, topic string, data []byte, metadata map[string]string) error
    Fetch(ctx context.Context, topic string, limit int) ([]Message, error)
    Delete(ctx context.Context, id string) error
    Close() error
}
```

## Requirements

- Go 1.23+
- Database driver for your chosen database
