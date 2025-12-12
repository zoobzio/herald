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

func TestPublisher_WithApply_TransformsValue(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.apply.transform", "Test apply transform")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithApply[TestEvent]("add-prefix", func(_ context.Context, e TestEvent) (TestEvent, error) {
			e.OrderID = "PREFIX-" + e.OrderID
			return e, nil
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 99.99}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OrderID != "PREFIX-123" {
		t.Errorf("expected OrderID 'PREFIX-123', got %q", result.OrderID)
	}
}

func TestPublisher_WithApply_ErrorAborts(t *testing.T) {
	var publishCalled atomic.Bool
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			publishCalled.Store(true)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.apply.abort", "Test apply abort")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var capturedErr Error
	c.Hook(ErrorSignal, func(_ context.Context, e *capitan.Event) {
		capturedErr, _ = ErrorKey.From(e)
	})

	opts := []Option[TestEvent]{
		WithApply[TestEvent]("validate", func(_ context.Context, e TestEvent) (TestEvent, error) {
			if e.Total < 0 {
				return e, errors.New("invalid total")
			}
			return e, nil
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: -5.0}))

	c.Shutdown()
	pub.Close()

	if publishCalled.Load() {
		t.Error("publish should not have been called")
	}

	if capturedErr.Operation != "publish" {
		t.Errorf("expected operation 'publish', got %q", capturedErr.Operation)
	}
}

func TestPublisher_WithEffect_SideEffect(t *testing.T) {
	var effectCalled atomic.Bool
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.effect", "Test effect")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithEffect[TestEvent]("log", func(_ context.Context, _ TestEvent) error {
			effectCalled.Store(true)
			return nil
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 99.99}))

	c.Shutdown()
	pub.Close()

	if !effectCalled.Load() {
		t.Error("effect should have been called")
	}

	// Effect doesn't modify data
	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.OrderID != "123" {
		t.Errorf("expected OrderID '123', got %q", result.OrderID)
	}
}

func TestPublisher_WithEffect_ErrorAborts(t *testing.T) {
	var publishCalled atomic.Bool
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			publishCalled.Store(true)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.effect.abort", "Test effect abort")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithEffect[TestEvent]("check", func(_ context.Context, e TestEvent) error {
			if e.Total < 0 {
				return errors.New("invalid total")
			}
			return nil
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: -5.0}))

	c.Shutdown()
	pub.Close()

	if publishCalled.Load() {
		t.Error("publish should not have been called")
	}
}

func TestPublisher_WithTransform_PureTransform(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.transform", "Test transform")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithTransform[TestEvent]("uppercase", func(_ context.Context, e TestEvent) TestEvent {
			e.OrderID = "TRANSFORMED-" + e.OrderID
			return e
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 99.99}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OrderID != "TRANSFORMED-123" {
		t.Errorf("expected OrderID 'TRANSFORMED-123', got %q", result.OrderID)
	}
}

func TestSubscriber_WithApply_TransformsValue(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.apply.transform", "Test sub apply transform")
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

	opts := []Option[TestEvent]{
		WithApply[TestEvent]("add-suffix", func(_ context.Context, e TestEvent) (TestEvent, error) {
			e.OrderID += "-SUFFIX"
			return e, nil
		}),
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "123", Total: 50.0}
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

	if received.OrderID != "123-SUFFIX" {
		t.Errorf("expected OrderID '123-SUFFIX', got %q", received.OrderID)
	}
}

func TestSubscriber_WithApply_ErrorTriggersNack(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.apply.nack", "Test sub apply nack")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var nacked atomic.Bool

	opts := []Option[TestEvent]{
		WithApply[TestEvent]("validate", func(_ context.Context, e TestEvent) (TestEvent, error) {
			if e.Total < 0 {
				return e, errors.New("invalid total")
			}
			return e, nil
		}),
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "123", Total: -5.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: func() error { nacked.Store(true); return nil },
	})

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	if !nacked.Load() {
		t.Error("expected message to be nack'd")
	}
}

func TestPublisher_WithApply_ComposesWithReliability(t *testing.T) {
	var attempts atomic.Int32
	var applyCalls atomic.Int32

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			if attempts.Add(1) < 3 {
				return errors.New("transient error")
			}
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.apply.retry", "Test apply with retry")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithApply[TestEvent]("count", func(_ context.Context, e TestEvent) (TestEvent, error) {
			applyCalls.Add(1)
			return e, nil
		}),
		WithRetry[TestEvent](3),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	// Options applied: retry wraps (apply wraps terminal)
	// So apply is inside retry, runs on each attempt
	if attempts.Load() != 3 {
		t.Errorf("expected 3 publish attempts, got %d", attempts.Load())
	}
	// Apply runs on each retry attempt
	if applyCalls.Load() != 3 {
		t.Errorf("expected apply to run 3 times (once per attempt), got %d", applyCalls.Load())
	}
}

func TestPublisher_MultipleOptions_WrapOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			mu.Lock()
			order = append(order, "publish")
			mu.Unlock()
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.wrap.order", "Test wrap order")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithEffect[TestEvent]("first", func(_ context.Context, _ TestEvent) error {
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
			return nil
		}),
		WithEffect[TestEvent]("second", func(_ context.Context, _ TestEvent) error {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			return nil
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	// Options wrap inside-out: second wraps (first wraps terminal)
	// Execution: second -> first -> publish
	expected := []string{"second", "first", "publish"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("expected order[%d] = %q, got %q", i, exp, order[i])
		}
	}
}
