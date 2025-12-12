package herald

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

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
	provider := &mockProvider{}
	capitan.Configure()
	defer capitan.Shutdown()

	signal := capitan.NewSignal("test.default", "Test default")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	pub := NewPublisher(provider, signal, key, nil)
	pub.Start()

	capitan.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "default"}))

	capitan.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
}
