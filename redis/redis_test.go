package redis

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	redisTC "github.com/testcontainers/testcontainers-go/modules/redis"
)

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()

	ctx := context.Background()

	container, err := redisTC.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis: %v", err)
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

	opts, err := redis.ParseURL(connStr)
	if err != nil {
		t.Fatalf("failed to parse connection string: %v", err)
	}

	client := redis.NewClient(opts)
	t.Cleanup(func() { client.Close() })

	return client
}

func TestProvider_Publish(t *testing.T) {
	client := setupRedis(t)
	streamName := "herald-test-publish"
	t.Cleanup(func() { client.Del(context.Background(), streamName) })

	provider := New(streamName, WithClient(client))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify by reading
	streams, err := client.XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{streamName, "0"},
		Count:   1,
	}).Result()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	if len(streams) != 1 || len(streams[0].Messages) != 1 {
		t.Fatal("expected 1 message")
	}

	data := streams[0].Messages[0].Values["data"]
	if data != `{"test":"data"}` {
		t.Errorf("unexpected data: %v", data)
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	client := setupRedis(t)
	streamName := "herald-test-publish-meta"
	t.Cleanup(func() { client.Del(context.Background(), streamName) })

	provider := New(streamName, WithClient(client))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	streams, err := client.XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{streamName, "0"},
		Count:   1,
	}).Result()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	traceID := streams[0].Messages[0].Values["trace-id"]
	if traceID != "abc123" {
		t.Errorf("unexpected trace-id: %v", traceID)
	}
}

func TestProvider_PublishNoClient(t *testing.T) {
	provider := New("test-stream")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil client")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	client := setupRedis(t)
	streamName := "herald-test-subscribe"
	t.Cleanup(func() { client.Del(context.Background(), streamName) })

	// Pre-populate messages
	client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{"data": `{"order":"1"}`},
	})
	client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{"data": `{"order":"2"}`},
	})

	provider := New(streamName, WithClient(client))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	cancel()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}
}

func TestProvider_SubscribeNoClient(t *testing.T) {
	provider := New("test-stream")

	ctx := context.Background()
	ch := provider.Subscribe(ctx)

	result, ok := <-ch
	if !ok {
		t.Fatal("expected error result before close")
	}
	if !result.IsError() {
		t.Error("expected error result")
	}

	_, ok = <-ch
	if ok {
		t.Error("expected closed channel after error")
	}
}

func TestProvider_Close(t *testing.T) {
	client := setupRedis(t)
	provider := New("test-stream", WithClient(client))

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test-stream")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil client: %v", err)
	}
}
