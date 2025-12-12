package sql

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

func setupMySQL(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()

	container, err := mysql.Run(ctx, "mysql:8",
		mysql.WithDatabase("herald_test"),
		mysql.WithUsername("herald"),
		mysql.WithPassword("herald"),
	)
	if err != nil {
		t.Fatalf("failed to start mysql: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := sql.Open("mysql", connStr)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	return db
}

func TestMySQLDB_InsertFetchDelete(t *testing.T) {
	db := setupMySQL(t)

	mysqlDB, err := NewMySQL(db)
	if err != nil {
		t.Fatalf("failed to create MySQLDB: %v", err)
	}

	ctx := context.Background()

	// Insert a message
	err = mysqlDB.Insert(ctx, "test-topic", []byte(`{"hello":"world"}`), map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Fetch messages
	messages, err := mysqlDB.Fetch(ctx, "test-topic", 10, 0)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if string(msg.Data) != `{"hello":"world"}` {
		t.Errorf("unexpected data: %s", msg.Data)
	}
	if msg.Metadata["key"] != "value" {
		t.Errorf("unexpected metadata: %v", msg.Metadata)
	}
	if msg.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Delete the message
	err = mysqlDB.Delete(ctx, msg.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Verify deletion
	messages, err = mysqlDB.Fetch(ctx, "test-topic", 10, 0)
	if err != nil {
		t.Fatalf("failed to fetch after delete: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(messages))
	}
}

func TestMySQLDB_FetchOrdering(t *testing.T) {
	db := setupMySQL(t)

	mysqlDB, err := NewMySQL(db)
	if err != nil {
		t.Fatalf("failed to create MySQLDB: %v", err)
	}

	ctx := context.Background()

	// Insert multiple messages
	for i := 0; i < 5; i++ {
		err = mysqlDB.Insert(ctx, "test-topic", []byte{byte('a' + i)}, nil)
		if err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	// Fetch with limit
	messages, err := mysqlDB.Fetch(ctx, "test-topic", 3, 0)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Verify ordering (oldest first)
	for i, msg := range messages {
		expected := byte('a' + i)
		if msg.Data[0] != expected {
			t.Errorf("message %d: expected %c, got %c", i, expected, msg.Data[0])
		}
	}
}

func TestMySQLDB_TopicIsolation(t *testing.T) {
	db := setupMySQL(t)

	mysqlDB, err := NewMySQL(db)
	if err != nil {
		t.Fatalf("failed to create MySQLDB: %v", err)
	}

	ctx := context.Background()

	// Insert to different topics
	err = mysqlDB.Insert(ctx, "topic-a", []byte("message-a"), nil)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	err = mysqlDB.Insert(ctx, "topic-b", []byte("message-b"), nil)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Fetch from topic-a only
	messages, err := mysqlDB.Fetch(ctx, "topic-a", 10, 0)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if string(messages[0].Data) != "message-a" {
		t.Errorf("unexpected data: %s", messages[0].Data)
	}
}

func TestMySQLDB_CustomTable(t *testing.T) {
	db := setupMySQL(t)

	mysqlDB, err := NewMySQL(db, WithMySQLTable("custom_messages"))
	if err != nil {
		t.Fatalf("failed to create MySQLDB: %v", err)
	}

	ctx := context.Background()

	// Insert should work with custom table
	err = mysqlDB.Insert(ctx, "test", []byte("data"), nil)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	// Verify table exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM custom_messages").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query custom table: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in custom_messages, got %d", count)
	}
}

func TestMySQLDB_ProviderIntegration(t *testing.T) {
	db := setupMySQL(t)

	mysqlDB, err := NewMySQL(db)
	if err != nil {
		t.Fatalf("failed to create MySQLDB: %v", err)
	}

	// Use with Provider
	provider := New("orders", WithDB(mysqlDB))

	ctx := context.Background()

	// Publish through provider
	err = provider.Publish(ctx, []byte(`{"order_id":"123"}`), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Verify message was inserted
	messages, err := mysqlDB.Fetch(ctx, "orders", 10, 0)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].Metadata["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type metadata, got %v", messages[0].Metadata)
	}
}
