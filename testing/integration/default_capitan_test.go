package integration

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/herald"
)

// newTestMessage creates a Message with no-op ack/nack for testing.
func newTestMessage(data []byte) herald.Message {
	return herald.Message{
		Data: data,
		Ack:  func() error { return nil },
		Nack: func() error { return nil },
	}
}

type mockProvider struct {
	mu        sync.Mutex
	published [][]byte
	subCh     chan herald.Result[herald.Message]
	closed    bool
	onPublish func()
}

func (m *mockProvider) Publish(_ context.Context, data []byte, _ herald.Metadata) error {
	m.mu.Lock()
	m.published = append(m.published, data)
	onPublish := m.onPublish
	m.mu.Unlock()
	if onPublish != nil {
		onPublish()
	}
	return nil
}

func (m *mockProvider) Subscribe(_ context.Context) <-chan herald.Result[herald.Message] {
	if m.subCh == nil {
		ch := make(chan herald.Result[herald.Message])
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

type TestEvent struct {
	OrderID string  `json:"order_id"`
	Total   float64 `json:"total"`
}

// TestSubscriber_DefaultCapitanEmit tests the default capitan emit path.
// This test uses the global capitan singleton and must run in isolation.
func TestSubscriber_DefaultCapitanEmit(t *testing.T) {
	subCh := make(chan herald.Result[herald.Message], 1)
	provider := &mockProvider{subCh: subCh}

	signal := capitan.NewSignal("integration.default.emit", "Integration default emit")
	key := capitan.NewKey[TestEvent]("payload", "integration.event")

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	// Hook into the global capitan
	listener := capitan.Hook(signal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = event
			wg.Done()
		}
	})
	defer listener.Close()

	// Create subscriber WITHOUT WithSubscriberCapitan - uses default global capitan
	sub := herald.NewSubscriber(provider, signal, key, nil)
	sub.Start()

	event := TestEvent{OrderID: "default-emit", Total: 42.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- herald.NewSuccess(newTestMessage(data))

	// Wait for hook to be called
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

// TestPublisher_DefaultCapitanObserve tests the default capitan observe path.
func TestPublisher_DefaultCapitanObserve(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	provider := &mockProvider{
		onPublish: func() {
			wg.Done()
		},
	}

	signal := capitan.NewSignal("integration.default.pub", "Integration default pub")
	key := capitan.NewKey[TestEvent]("payload", "integration.event")

	// Create publisher WITHOUT WithPublisherCapitan - uses default global capitan
	pub := herald.NewPublisher(provider, signal, key, nil)
	pub.Start()

	// Emit to global capitan
	capitan.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "default-pub", Total: 99.0}))

	// Wait for publish to complete
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

	provider.mu.Lock()
	defer provider.mu.Unlock()

	if len(provider.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(provider.published))
	}

	var event TestEvent
	if err := json.Unmarshal(provider.published[0], &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if event.OrderID != "default-pub" {
		t.Errorf("expected OrderID 'default-pub', got %q", event.OrderID)
	}
}
