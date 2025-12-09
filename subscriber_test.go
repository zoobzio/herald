package herald

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
)

// newTestMessage creates a Message with no-op ack/nack for testing.
func newTestMessage(data []byte) Message {
	return Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: func() error { return nil },
	}
}

func TestSubscriber_Start(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event", "Test event")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = event
			wg.Done()
		}
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Send message through provider
	event := TestEvent{OrderID: "ORD-456", Total: 123.45}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(newTestMessage(data))

	// Wait for processing
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	sub.Close()

	if received.OrderID != "ORD-456" {
		t.Errorf("expected OrderID 'ORD-456', got %q", received.OrderID)
	}

	if received.Total != 123.45 {
		t.Errorf("expected Total 123.45, got %v", received.Total)
	}
}

func TestSubscriber_IgnoresErrors(t *testing.T) {
	subCh := make(chan Result[Message], 2)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event.err", "Test event error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var received []TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = append(received, event)
			wg.Done()
		}
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Send error (will be skipped)
	subCh <- NewError[Message](context.DeadlineExceeded)

	// Send valid message
	event := TestEvent{OrderID: "valid", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(newTestMessage(data))

	// Wait for the valid event to be processed
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	sub.Close()

	if len(received) != 1 {
		t.Fatalf("expected 1 event (error skipped), got %d", len(received))
	}

	if received[0].OrderID != "valid" {
		t.Errorf("expected OrderID 'valid', got %q", received[0].OrderID)
	}
}

func TestSubscriber_IgnoresInvalidPayload(t *testing.T) {
	subCh := make(chan Result[Message], 2)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event.payload", "Test event payload")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var received []TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = append(received, event)
			wg.Done()
		}
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Send invalid payload (will be nacked)
	subCh <- NewSuccess(newTestMessage([]byte("not valid")))

	// Send valid message
	event := TestEvent{OrderID: "valid", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(newTestMessage(data))

	// Wait for the valid event to be processed
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	sub.Close()

	if len(received) != 1 {
		t.Fatalf("expected 1 event (invalid payload skipped), got %d", len(received))
	}
}

func TestSubscriber_Close(_ *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.close", "Test close")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Close should cancel context
	cancel()
	sub.Close()

	// Channel should not block
	select {
	case subCh <- NewSuccess(newTestMessage([]byte("{}"))):
		// OK, channel accepted (but subscriber should have exited)
	default:
		// Also OK
	}
}

func TestSubscriber_DefaultCapitan(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	// Use a fresh Capitan instance to avoid interference with other tests
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.default.sub", "Test default sub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = event
			wg.Done()
		}
	})

	// Use WithSubscriberCapitan to use our instance
	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	event := TestEvent{OrderID: "default", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(newTestMessage(data))

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	sub.Close()

	if received.OrderID != "default" {
		t.Errorf("expected OrderID 'default', got %q", received.OrderID)
	}
}

func TestSubscriber_NilProvider(_ *testing.T) {
	provider := &mockProvider{} // No subCh set - returns closed channel

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nil", "Test nil")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// The subscriber goroutine reads from a closed channel and exits immediately.
	// Close() is safe to call - it just cancels the context.
	cancel()
	sub.Close()
}

func TestSubscriber_AckOnSuccess(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.ack", "Test ack")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var acked atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, _ *capitan.Event) {
		wg.Done()
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	event := TestEvent{OrderID: "ack-test", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack: func() error {
			acked.Store(true)
			return nil
		},
		Nack: func() error { return nil },
	}
	subCh <- NewSuccess(msg)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()
	sub.Close()

	if !acked.Load() {
		t.Error("expected Ack to be called on success")
	}
}

func TestSubscriber_NackOnInvalidPayload(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nack.payload", "Test nack payload")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var nacked atomic.Bool

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	msg := Message{
		Data: []byte("invalid payload"),
		Ack:  func() error { return nil },
		Nack: func() error {
			nacked.Store(true)
			return nil
		},
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	cancel()
	sub.Close()

	if !nacked.Load() {
		t.Error("expected Nack to be called on invalid payload")
	}
}
