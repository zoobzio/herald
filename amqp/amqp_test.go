package amqp

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func setupRabbitMQ(t *testing.T) (*amqp.Connection, *amqp.Channel) {
	t.Helper()

	ctx := context.Background()

	container, err := rabbitmq.Run(ctx, "rabbitmq:3-management-alpine")
	if err != nil {
		t.Fatalf("failed to start rabbitmq: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	connStr, err := container.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("failed to get amqp url: %v", err)
	}

	conn, err := amqp.Dial(connStr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open channel: %v", err)
	}

	return conn, ch
}

func setupTestQueue(t *testing.T, ch *amqp.Channel, name string) {
	t.Helper()

	_, err := ch.QueueDeclare(name, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}
	t.Cleanup(func() { ch.QueueDelete(name, false, false, false) })
}

func TestProvider_Publish(t *testing.T) {
	_, ch := setupRabbitMQ(t)
	defer ch.Close()

	queueName := "herald-test-publish"
	setupTestQueue(t, ch, queueName)

	provider := New(queueName, WithChannel(ch), WithRoutingKey(queueName))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify by consuming
	msg, ok, err := ch.Get(queueName, true)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}
	if !ok {
		t.Fatal("expected message in queue")
	}

	if string(msg.Body) != `{"test":"data"}` {
		t.Errorf("unexpected data: %s", msg.Body)
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	_, ch := setupRabbitMQ(t)
	defer ch.Close()

	queueName := "herald-test-publish-meta"
	setupTestQueue(t, ch, queueName)

	provider := New(queueName, WithChannel(ch), WithRoutingKey(queueName))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, ok, err := ch.Get(queueName, true)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}
	if !ok {
		t.Fatal("expected message in queue")
	}

	if msg.Headers["trace-id"] != "abc123" {
		t.Errorf("unexpected header: %v", msg.Headers)
	}
}

func TestProvider_PublishNoChannel(t *testing.T) {
	provider := New("test-queue")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil channel")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	_, ch := setupRabbitMQ(t)
	defer ch.Close()

	queueName := "herald-test-subscribe"
	setupTestQueue(t, ch, queueName)

	provider := New(queueName, WithChannel(ch))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := provider.Subscribe(ctx)

	// Publish messages
	ch.PublishWithContext(ctx, "", queueName, false, false, amqp.Publishing{Body: []byte(`{"order":"1"}`)})
	ch.PublishWithContext(ctx, "", queueName, false, false, amqp.Publishing{Body: []byte(`{"order":"2"}`)})

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case msg := <-result:
			if msg.IsError() {
				t.Fatalf("unexpected error: %v", msg.Error())
			}
			if err := msg.Value().Ack(); err != nil {
				t.Errorf("ack failed: %v", err)
			}
			received = append(received, msg.Value().Data)
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

func TestProvider_SubscribeNoChannel(t *testing.T) {
	provider := New("test-queue")

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
	_, ch := setupRabbitMQ(t)

	provider := New("test-queue", WithChannel(ch))

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ch.IsClosed() {
		t.Error("channel not closed")
	}
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test-queue")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil channel: %v", err)
	}
}
