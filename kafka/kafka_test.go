package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/testcontainers/testcontainers-go"
	kafkaTC "github.com/testcontainers/testcontainers-go/modules/kafka"
)

func setupKafka(t *testing.T) (string, string) {
	t.Helper()

	ctx := context.Background()

	container, err := kafkaTC.Run(ctx, "confluentinc/confluent-local:7.6.0")
	if err != nil {
		t.Fatalf("failed to start kafka: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	brokers, err := container.Brokers(ctx)
	if err != nil {
		t.Fatalf("failed to get brokers: %v", err)
	}

	if len(brokers) == 0 {
		t.Fatal("no brokers returned")
	}

	// Create topic
	topic := "herald-test-topic"
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("failed to dial kafka: %v", err)
	}
	defer conn.Close()

	err = conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	return brokers[0], topic
}

func TestProvider_Publish(t *testing.T) {
	broker, topic := setupKafka(t)

	writer := &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Balancer: &kafka.LeastBytes{},
	}
	t.Cleanup(func() { writer.Close() })

	provider := New(topic, WithWriter(writer))

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	broker, topic := setupKafka(t)

	writer := &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Balancer: &kafka.LeastBytes{},
	}
	t.Cleanup(func() { writer.Close() })

	provider := New(topic, WithWriter(writer))

	metadata := map[string]string{"trace-id": "abc123"}
	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PublishNoWriter(t *testing.T) {
	provider := New("test-topic")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil writer")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	broker, topic := setupKafka(t)

	// Write messages first
	writer := &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Balancer: &kafka.LeastBytes{},
	}

	ctx := context.Background()
	err := writer.WriteMessages(ctx,
		kafka.Message{Topic: topic, Value: []byte(`{"order":"1"}`)},
		kafka.Message{Topic: topic, Value: []byte(`{"order":"2"}`)},
	)
	if err != nil {
		t.Fatalf("failed to write messages: %v", err)
	}
	writer.Close()

	// Create reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{broker},
		Topic:     topic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	t.Cleanup(func() { reader.Close() })

	// Seek to beginning
	reader.SetOffset(0)

	provider := New(topic, WithReader(reader))

	subCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ch := provider.Subscribe(subCtx)

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
		case <-subCtx.Done():
			t.Fatal("timeout")
		}
	}

	cancel()
	provider.Close()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
}

func TestProvider_SubscribeNoReader(t *testing.T) {
	provider := New("test-topic")

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
	provider := New("test-topic")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
