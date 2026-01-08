package nats

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startTestServer(t *testing.T) (*server.Server, *nats.Conn) {
	t.Helper()

	opts := &server.Options{
		Host: "127.0.0.1",
		Port: -1, // random port
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

	return s, nc
}

func TestProvider_Publish(t *testing.T) {
	_, nc := startTestServer(t)
	provider := New("test.subject", WithConn(nc))

	// Subscribe to receive the message
	sub, err := nc.SubscribeSync("test.subject")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	err = provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := sub.NextMsg(time.Second)
	if err != nil {
		t.Fatalf("failed to receive message: %v", err)
	}

	if string(msg.Data) != `{"test":"data"}` {
		t.Errorf("unexpected data: %s", msg.Data)
	}
}

func TestProvider_PublishNoConn(t *testing.T) {
	provider := New("test.subject")

	err := provider.Publish(context.Background(), []byte(`{"test":"data"}`), nil)
	if err == nil {
		t.Fatal("expected error with nil connection")
	}
}

func TestProvider_Subscribe(t *testing.T) {
	_, nc := startTestServer(t)
	provider := New("test.subject", WithConn(nc))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := provider.Subscribe(ctx)

	// Give subscription time to set up
	time.Sleep(50 * time.Millisecond)

	// Publish messages
	if err := nc.Publish("test.subject", []byte(`{"order":"1"}`)); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	if err := nc.Publish("test.subject", []byte(`{"order":"2"}`)); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	nc.Flush()

	var received [][]byte
	for i := 0; i < 2; i++ {
		select {
		case result := <-ch:
			if result.IsError() {
				t.Fatalf("unexpected error: %v", result.Error())
			}
			received = append(received, result.Value().Data)
		case <-time.After(time.Second):
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

func TestProvider_SubscribeNoConn(t *testing.T) {
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
	_, nc := startTestServer(t)
	provider := New("test.subject", WithConn(nc))

	ctx, cancel := context.WithCancel(context.Background())
	_ = provider.Subscribe(ctx)
	cancel()

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_CloseNil(t *testing.T) {
	provider := New("test.subject")

	err := provider.Close()
	if err != nil {
		t.Fatalf("unexpected error with nil connection: %v", err)
	}
}
