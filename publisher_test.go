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

type mockProvider struct {
	mu          sync.Mutex
	published   [][]byte
	metadata    []Metadata
	subCh       chan Result[Message]
	closed      bool
	publishFunc func(ctx context.Context, data []byte, metadata Metadata) error
}

func (m *mockProvider) Publish(ctx context.Context, data []byte, metadata Metadata) error {
	m.mu.Lock()
	m.published = append(m.published, data)
	m.metadata = append(m.metadata, metadata)
	publishFunc := m.publishFunc
	m.mu.Unlock()

	if publishFunc != nil {
		return publishFunc(ctx, data, metadata)
	}
	return nil
}

func (m *mockProvider) Subscribe(_ context.Context) <-chan Result[Message] {
	if m.subCh == nil {
		ch := make(chan Result[Message])
		close(ch)
		return ch
	}
	return m.subCh
}

func (*mockProvider) Ping(_ context.Context) error {
	return nil
}

func (m *mockProvider) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockProvider) Published() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.published
}

type TestEvent struct {
	OrderID string  `json:"order_id"`
	Total   float64 `json:"total"`
}

func TestPublisher_Start(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event", "Test event")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "ORD-123",
		Total:   99.99,
	}))

	c.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}

	var event TestEvent
	if err := json.Unmarshal(published[0], &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if event.OrderID != "ORD-123" {
		t.Errorf("expected OrderID 'ORD-123', got %q", event.OrderID)
	}

	if event.Total != 99.99 {
		t.Errorf("expected Total 99.99, got %v", event.Total)
	}
}

func TestPublisher_IgnoresOtherSignals(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event", "Test event")
	otherSignal := capitan.NewSignal("other.event", "Other event")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	// Emit to other signal
	c.Emit(context.Background(), otherSignal, key.Field(TestEvent{OrderID: "ignored"}))

	c.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 0 {
		t.Errorf("expected 0 published messages, got %d", len(published))
	}
}

func TestPublisher_IgnoresMissingField(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.event", "Test event")
	key := capitan.NewKey[TestEvent]("payload", "test.event")
	otherKey := capitan.NewStringKey("other")

	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	// Emit with wrong field type
	c.Emit(context.Background(), signal, otherKey.Field("not the right type"))

	c.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 0 {
		t.Errorf("expected 0 published messages (field type mismatch), got %d", len(published))
	}
}

func TestPublisher_Close(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.close", "Test close")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()
	pub.Close()

	// Emit after close
	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "after-close"}))

	c.Shutdown()

	published := provider.Published()
	if len(published) != 0 {
		t.Errorf("expected 0 published messages after close, got %d", len(published))
	}
}

func TestPublisher_DefaultCapitan(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			wg.Done()
			return nil
		},
	}

	signal := capitan.NewSignal("test.default", "Test default")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Publisher WITHOUT WithPublisherCapitan - uses global
	pub := NewPublisher(provider, signal, key, nil)
	pub.Start()

	capitan.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "default"}))

	// Wait for publish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for publish")
	}

	pub.Close()

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
}

func TestPublisher_DefaultCapitan_EmitError(t *testing.T) {
	// Provider that always fails
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return errors.New("publish failed")
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.default.error", "Test default error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var errorReceived atomic.Bool
	c.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})

	// Use custom capitan to test error path with isolated instance
	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "will-fail"}))

	// Give time for async processing
	time.Sleep(50 * time.Millisecond)

	c.Shutdown()
	pub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted via capitan")
	}
}

func TestPublisher_CloseBeforeStart(t *testing.T) {
	provider := &mockProvider{}

	signal := capitan.NewSignal("test.close.before", "Test close before start")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Create publisher but don't start - observer will be nil
	pub := NewPublisher(provider, signal, key, nil)

	// Close should not panic with nil observer
	err := pub.Close()
	if err != nil {
		t.Errorf("expected no error closing unstarted publisher, got %v", err)
	}
}

func TestPublisher_CloseWithNilPipeline(t *testing.T) {
	// Directly create a Publisher with nil pipeline to test defensive code path
	pub := &Publisher[TestEvent]{}

	// Close should handle nil pipeline gracefully
	err := pub.Close()
	if err != nil {
		t.Errorf("expected no error closing publisher with nil pipeline, got %v", err)
	}
}

func TestPublisher_EmitErrorWithoutCustomCapitan(t *testing.T) {
	// Provider that always fails
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return errors.New("publish failed")
		},
	}

	signal := capitan.NewSignal("test.emit.error.global", "Test emit error global")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Hook into global capitan to catch error
	var errorReceived atomic.Bool
	listener := capitan.Hook(ErrorSignal, func(_ context.Context, _ *capitan.Event) {
		errorReceived.Store(true)
	})
	defer listener.Close()

	// Publisher WITHOUT WithPublisherCapitan - uses global
	pub := NewPublisher(provider, signal, key, nil)
	pub.Start()

	// Emit via global capitan
	capitan.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "global-error"}))

	// Give time for async processing
	time.Sleep(100 * time.Millisecond)

	pub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted via global capitan")
	}
}
