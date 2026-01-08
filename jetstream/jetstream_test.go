package jetstream

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/zoobzio/herald"
)

func startTestServer(t *testing.T) (*server.Server, *nats.Conn, jetstream.JetStream) {
	t.Helper()

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("server not ready")
	}
	t.Cleanup(s.Shutdown)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("failed to create jetstream: %v", err)
	}

	return s, nc, js
}

func TestProvider_Publish(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	provider := New("test.subject", WithJetStream(js))

	err = provider.Publish(ctx, []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify message was published by creating a consumer and reading
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	msg, err := consumer.Next()
	if err != nil {
		t.Fatalf("failed to fetch message: %v", err)
	}

	if string(msg.Data()) != `{"test":"data"}` {
		t.Errorf("unexpected data: %s", msg.Data())
	}
}

func TestProvider_PublishWithMetadata(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	provider := New("test.subject", WithJetStream(js))

	metadata := herald.Metadata{
		"trace-id":      "abc123",
		"correlation-id": "xyz789",
	}

	err = provider.Publish(ctx, []byte(`{"test":"data"}`), metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify message and headers
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	msg, err := consumer.Next()
	if err != nil {
		t.Fatalf("failed to fetch message: %v", err)
	}

	headers := msg.Headers()
	if headers.Get("trace-id") != "abc123" {
		t.Errorf("unexpected trace-id header: %s", headers.Get("trace-id"))
	}
	if headers.Get("correlation-id") != "xyz789" {
		t.Errorf("unexpected correlation-id header: %s", headers.Get("correlation-id"))
	}
}

func TestProvider_PublishNoJetStream(t *testing.T) {
	provider := New("test.subject")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil jetstream")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Create consumer
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	provider := New("test.subject", WithJetStream(js), WithConsumer(consumer))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := provider.Subscribe(ctx)

	// Publish messages
	_, err = js.Publish(ctx, "test.subject", []byte(`{"order":"1"}`))
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	_, err = js.Publish(ctx, "test.subject", []byte(`{"order":"2"}`))
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			msg := result.Value()
			received = append(received, msg.Data)
			if err := msg.Ack(); err != nil {
				t.Fatalf("failed to ack: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	cancel()
	provider.Close()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}

	if string(received[0]) != `{"order":"1"}` {
		t.Errorf("unexpected first message: %s", received[0])
	}
}

func TestProvider_SubscribeWithMetadata(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Create consumer
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	provider := New("test.subject", WithJetStream(js), WithConsumer(consumer))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := provider.Subscribe(ctx)

	// Publish message with headers
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    []byte(`{"test":"data"}`),
		Header:  nats.Header{"trace-id": []string{"abc123"}},
	}
	_, err = js.PublishMsg(ctx, msg)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		heraldMsg := result.Value()
		if heraldMsg.Metadata["trace-id"] != "abc123" {
			t.Errorf("unexpected metadata: %v", heraldMsg.Metadata)
		}
		heraldMsg.Ack()
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProvider_SubscribeNoConsumer(t *testing.T) {
	provider := New("test.subject")

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
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Create consumer
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	provider := New("test.subject", WithJetStream(js), WithConsumer(consumer))

	ctx, cancel := context.WithCancel(context.Background())
	_ = provider.Subscribe(ctx)
	cancel()

	err = provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test.subject")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil resources: %v", err)
	}
}

func TestProvider_Ping(t *testing.T) {
	_, nc, js := startTestServer(t)
	_ = nc // keep connection alive

	provider := New("test.subject", WithJetStream(js))

	err := provider.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_PingNoConnection(t *testing.T) {
	provider := New("test.subject")

	err := provider.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error with nil connection")
	}
}

func TestProvider_SubscribeCancellation(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Create consumer
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	provider := New("test.subject", WithJetStream(js), WithConsumer(consumer))

	ctx, cancel := context.WithCancel(ctx)
	ch := provider.Subscribe(ctx)

	// Cancel context - should stop subscriber without blocking
	cancel()

	// Give subscriber time to notice cancellation
	time.Sleep(50 * time.Millisecond)

	// Channel should be closed or draining
	select {
	case _, ok := <-ch:
		if ok {
			// Message received is fine, channel should close soon
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not respond to context cancellation")
	}

	provider.Close()
}

func TestProvider_MultiValueHeaders(t *testing.T) {
	_, _, js := startTestServer(t)

	ctx := context.Background()

	// Create stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.subject"},
	})
	if err != nil {
		t.Fatalf("failed to create stream: %v", err)
	}

	// Create consumer
	consumer, err := js.CreateConsumer(ctx, "TEST", jetstream.ConsumerConfig{
		Durable:   "test-consumer",
		AckPolicy: jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	provider := New("test.subject", WithJetStream(js), WithConsumer(consumer))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := provider.Subscribe(ctx)

	// Publish message with multi-value header
	msg := &nats.Msg{
		Subject: "test.subject",
		Data:    []byte(`{"test":"data"}`),
		Header:  nats.Header{"Accept": []string{"application/json", "text/plain"}},
	}
	_, err = js.PublishMsg(ctx, msg)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	select {
	case result := <-ch:
		if result.IsError() {
			t.Fatalf("unexpected error: %v", result.Error())
		}
		heraldMsg := result.Value()
		// Multi-value headers should be comma-joined per RFC 7230
		accept := heraldMsg.Metadata["Accept"]
		if accept != "application/json,text/plain" {
			t.Errorf("expected comma-joined headers, got %q", accept)
		}
		heraldMsg.Ack()
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
