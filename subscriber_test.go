package herald

import (
	"context"
	"encoding/json"
	"errors"
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
	sub.Start(context.Background())

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
	sub.Start(context.Background())

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
	sub.Start(context.Background())

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
	sub.Start(context.Background())

	// Close should cancel internal context
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
	sub.Start(context.Background())

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
	sub.Start(context.Background())

	// The subscriber goroutine reads from a closed channel and exits immediately.
	// Close() is safe to call - it just cancels the context.
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
	sub.Start(context.Background())

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
	sub.Start(context.Background())

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

	sub.Close()

	if !nacked.Load() {
		t.Error("expected Nack to be called on invalid payload")
	}
}

func TestSubscriber_DefaultCapitan_EmitAndError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.default.sub.emit", "Test default sub emit")
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

	// Use custom capitan with isolated instance
	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "default-emit", Total: 1.0}
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

	sub.Close()

	if received.OrderID != "default-emit" {
		t.Errorf("expected OrderID 'default-emit', got %q", received.OrderID)
	}
}

func TestSubscriber_DefaultCapitan_EmitError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.default.sub.error", "Test default sub error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorReceived atomic.Bool
	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	// Use custom capitan with isolated instance
	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send invalid payload to trigger error
	subCh <- NewSuccess(newTestMessage([]byte("invalid")))

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted via capitan")
	}
}

func TestSubscriber_CloseBeforeStart(t *testing.T) {
	provider := &mockProvider{}

	signal := capitan.NewSignal("test.close.before.sub", "Test close before start sub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Create subscriber but don't start - cancel will be nil
	sub := NewSubscriber(provider, signal, key, nil)

	// Close should not panic with nil cancel
	err := sub.Close()
	if err != nil {
		t.Errorf("expected no error closing unstarted subscriber, got %v", err)
	}
}

func TestSubscriber_CloseWithNilPipeline(t *testing.T) {
	// Directly create a Subscriber with nil pipeline to test defensive code path
	sub := &Subscriber[TestEvent]{}

	// Close should handle nil pipeline gracefully
	err := sub.Close()
	if err != nil {
		t.Errorf("expected no error closing subscriber with nil pipeline, got %v", err)
	}
}

func TestSubscriber_MessageWithMetadata(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.metadata", "Test metadata")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var receivedMetadata Metadata
	var wg sync.WaitGroup
	wg.Add(1)

	// Middleware that captures metadata from envelope
	opts := []Option[TestEvent]{
		WithMiddleware(
			UseEffect[TestEvent]("capture-metadata", func(_ context.Context, env *Envelope[TestEvent]) error {
				receivedMetadata = env.Metadata
				wg.Done()
				return nil
			}),
		),
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send message with metadata
	event := TestEvent{OrderID: "with-metadata", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Metadata: Metadata{
			"trace-id":     "abc123",
			"Content-Type": "application/json",
		},
		Ack:  func() error { return nil },
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

	sub.Close()

	if receivedMetadata == nil {
		t.Fatal("expected metadata to be accessible via envelope")
	}
	if receivedMetadata["trace-id"] != "abc123" {
		t.Errorf("expected trace-id 'abc123', got %q", receivedMetadata["trace-id"])
	}
}

func TestSubscriber_EmitErrorWithoutCustomCapitan(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	signal := capitan.NewSignal("test.emit.error.global.sub", "Test emit error global sub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Hook into global capitan to catch error
	var errorReceived atomic.Bool
	listener := capitan.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})
	defer listener.Close()

	// Subscriber WITHOUT WithSubscriberCapitan - uses global
	sub := NewSubscriber(provider, signal, key, nil)
	sub.Start(context.Background())

	// Send invalid payload to trigger unmarshal error
	subCh <- NewSuccess(newTestMessage([]byte("invalid json")))

	// Give time for async processing
	time.Sleep(100 * time.Millisecond)

	sub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted via global capitan")
	}
}

func TestSubscriber_EmitWithoutCustomCapitan(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	signal := capitan.NewSignal("test.emit.global.sub", "Test emit global sub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Hook into global capitan to catch event
	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	listener := capitan.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = event
			wg.Done()
		}
	})
	defer listener.Close()

	// Subscriber WITHOUT WithSubscriberCapitan - uses global
	sub := NewSubscriber(provider, signal, key, nil)
	sub.Start(context.Background())

	event := TestEvent{OrderID: "global-emit", Total: 1.0}
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

	sub.Close()

	if received.OrderID != "global-emit" {
		t.Errorf("expected OrderID 'global-emit', got %q", received.OrderID)
	}
}

func TestSubscriber_NilNackOnInvalidPayload(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nil.nack", "Test nil nack")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorReceived atomic.Bool
	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send invalid payload with nil Nack function
	msg := Message{
		Data: []byte("invalid"),
		Ack:  func() error { return nil },
		Nack: nil, // nil Nack
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	// Error should still be emitted even with nil Nack
	if !errorReceived.Load() {
		t.Error("expected error to be emitted even with nil Nack")
	}
}

func TestSubscriber_NilAckOnSuccess(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nil.ack", "Test nil ack")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, _ *capitan.Event) {
		wg.Done()
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send valid payload with nil Ack function
	event := TestEvent{OrderID: "nil-ack", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack:  nil, // nil Ack
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

	sub.Close()
	// Test passes if no panic occurred with nil Ack
}

func TestSubscriber_NilNackOnPipelineError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nil.nack.pipeline", "Test nil nack pipeline")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorReceived atomic.Bool
	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	// Pipeline that always fails
	failingPipeline := []Option[TestEvent]{
		WithMiddleware(
			UseApply[TestEvent]("fail", func(_ context.Context, _ *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				return nil, errors.New("pipeline failed")
			}),
		),
	}

	sub := NewSubscriber(provider, signal, key, failingPipeline, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send valid payload with nil Nack
	event := TestEvent{OrderID: "nil-nack-pipeline", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: nil, // nil Nack - pipeline will fail but can't nack
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	// Error should still be emitted
	if !errorReceived.Load() {
		t.Error("expected error to be emitted on pipeline failure with nil Nack")
	}
}

func TestSubscriber_NackError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nack.error", "Test nack error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorCount atomic.Int32
	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorCount.Add(1)
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send invalid payload with nack that fails
	msg := Message{
		Data: []byte("invalid"),
		Ack:  func() error { return nil },
		Nack: func() error {
			return errors.New("nack failed")
		},
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	// Should have 2 errors: one for nack failure, one for unmarshal
	if errorCount.Load() < 2 {
		t.Errorf("expected at least 2 errors (nack + unmarshal), got %d", errorCount.Load())
	}
}

func TestSubscriber_AckError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.ack.error", "Test ack error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorReceived atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, _ *capitan.Event) {
		wg.Done()
	})

	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	sub := NewSubscriber(provider, signal, key, nil, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	// Send valid payload with ack that fails
	event := TestEvent{OrderID: "ack-fail", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack: func() error {
			return errors.New("ack failed")
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

	// Give time for ack error to be emitted
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted when ack fails")
	}
}

func TestSubscriber_NackOnPipelineError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nack.pipeline", "Test nack pipeline")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var nacked atomic.Bool
	var errorReceived atomic.Bool

	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	// Pipeline that always fails
	failingPipeline := []Option[TestEvent]{
		WithMiddleware(
			UseApply[TestEvent]("fail", func(_ context.Context, _ *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				return nil, errors.New("pipeline failed")
			}),
		),
	}

	sub := NewSubscriber(provider, signal, key, failingPipeline, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "pipeline-fail", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: func() error {
			nacked.Store(true)
			return nil
		},
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	if !nacked.Load() {
		t.Error("expected Nack to be called on pipeline failure")
	}
	if !errorReceived.Load() {
		t.Error("expected error to be emitted on pipeline failure")
	}
}

func TestSubscriber_NackErrorOnPipelineError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.nack.error.pipeline", "Test nack error pipeline")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorCount atomic.Int32

	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorCount.Add(1)
	})

	// Pipeline that always fails
	failingPipeline := []Option[TestEvent]{
		WithMiddleware(
			UseApply[TestEvent]("fail", func(_ context.Context, _ *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				return nil, errors.New("pipeline failed")
			}),
		),
	}

	sub := NewSubscriber(provider, signal, key, failingPipeline, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "nack-error-pipeline", Total: 1.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	msg := Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: func() error {
			return errors.New("nack also failed")
		},
	}
	subCh <- NewSuccess(msg)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	// Should have 2 errors: one for nack failure, one for pipeline
	if errorCount.Load() < 2 {
		t.Errorf("expected at least 2 errors (nack + pipeline), got %d", errorCount.Load())
	}
}
