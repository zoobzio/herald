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

// mockCodec tracks calls for testing.
type mockCodec struct {
	marshalCalled   atomic.Bool
	unmarshalCalled atomic.Bool
	marshalFunc     func(v any) ([]byte, error)
	unmarshalFunc   func(data []byte, v any) error
	contentType     string
}

func (m *mockCodec) Marshal(v any) ([]byte, error) {
	m.marshalCalled.Store(true)
	if m.marshalFunc != nil {
		return m.marshalFunc(v)
	}
	return json.Marshal(v)
}

func (m *mockCodec) Unmarshal(data []byte, v any) error {
	m.unmarshalCalled.Store(true)
	if m.unmarshalFunc != nil {
		return m.unmarshalFunc(data, v)
	}
	return json.Unmarshal(data, v)
}

func (m *mockCodec) ContentType() string {
	if m.contentType != "" {
		return m.contentType
	}
	return "application/x-mock"
}

func TestJSONCodec_Marshal(t *testing.T) {
	codec := JSONCodec{}
	event := TestEvent{OrderID: "ORD-123", Total: 99.99}

	data, err := codec.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded TestEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.OrderID != event.OrderID {
		t.Errorf("expected OrderID %q, got %q", event.OrderID, decoded.OrderID)
	}
	if decoded.Total != event.Total {
		t.Errorf("expected Total %v, got %v", event.Total, decoded.Total)
	}
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	codec := JSONCodec{}
	data := []byte(`{"order_id":"ORD-456","total":123.45}`)

	var event TestEvent
	if err := codec.Unmarshal(data, &event); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if event.OrderID != "ORD-456" {
		t.Errorf("expected OrderID 'ORD-456', got %q", event.OrderID)
	}
	if event.Total != 123.45 {
		t.Errorf("expected Total 123.45, got %v", event.Total)
	}
}

func TestJSONCodec_ContentType(t *testing.T) {
	codec := JSONCodec{}
	if codec.ContentType() != "application/json" {
		t.Errorf("expected 'application/json', got %q", codec.ContentType())
	}
}

func TestPublisher_DefaultCodecIsJSON(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.default", "Test codec default")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	pub := NewPublisher(provider, signal, key, nil, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "ORD-789",
		Total:   50.00,
	}))

	c.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}

	// Verify it's valid JSON
	var event TestEvent
	if err := json.Unmarshal(published[0], &event); err != nil {
		t.Fatalf("default codec should produce valid JSON: %v", err)
	}

	if event.OrderID != "ORD-789" {
		t.Errorf("expected OrderID 'ORD-789', got %q", event.OrderID)
	}
}

func TestPublisher_CustomCodec(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.custom", "Test codec custom")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	codec := &mockCodec{}

	pub := NewPublisher(provider, signal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](codec),
	)
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "ORD-CUSTOM",
		Total:   75.00,
	}))

	c.Shutdown()
	pub.Close()

	if !codec.marshalCalled.Load() {
		t.Error("expected custom codec Marshal to be called")
	}

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}
}

func TestPublisher_ContentTypeInMetadata(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.contenttype", "Test codec content type")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	codec := &mockCodec{contentType: "application/x-custom"}

	pub := NewPublisher(provider, signal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](codec),
	)
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "ORD-CT",
		Total:   25.00,
	}))

	c.Shutdown()
	pub.Close()

	provider.mu.Lock()
	metadata := provider.metadata
	provider.mu.Unlock()

	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}

	ct, exists := metadata[0]["Content-Type"]
	if !exists {
		t.Fatal("expected Content-Type in metadata")
	}
	if ct != "application/x-custom" {
		t.Errorf("expected Content-Type 'application/x-custom', got %q", ct)
	}
}

func TestPublisher_ContentTypeNotOverwritten(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.ct.preserve", "Test codec content type preserve")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	codec := &mockCodec{contentType: "application/x-custom"}

	pub := NewPublisher(provider, signal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](codec),
	)
	pub.Start()

	// Emit with pre-set Content-Type
	ctx := ContextWithMetadata(context.Background(), Metadata{
		"Content-Type": "application/x-preset",
	})
	c.Emit(ctx, signal, key.Field(TestEvent{
		OrderID: "ORD-PRESET",
		Total:   30.00,
	}))

	c.Shutdown()
	pub.Close()

	provider.mu.Lock()
	metadata := provider.metadata
	provider.mu.Unlock()

	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}

	ct := metadata[0]["Content-Type"]
	if ct != "application/x-preset" {
		t.Errorf("expected Content-Type 'application/x-preset' (not overwritten), got %q", ct)
	}
}

func TestSubscriber_DefaultCodecIsJSON(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.codec.default", "Test sub codec default")
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

	// Send JSON-encoded message
	event := TestEvent{OrderID: "SUB-DEFAULT", Total: 100.00}
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

	if received.OrderID != "SUB-DEFAULT" {
		t.Errorf("expected OrderID 'SUB-DEFAULT', got %q", received.OrderID)
	}
}

func TestSubscriber_CustomCodec(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.codec.custom", "Test sub codec custom")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	codec := &mockCodec{}

	var wg sync.WaitGroup
	wg.Add(1)

	c.Hook(signal, func(_ context.Context, _ *capitan.Event) {
		wg.Done()
	})

	sub := NewSubscriber(provider, signal, key, nil,
		WithSubscriberCapitan[TestEvent](c),
		WithSubscriberCodec[TestEvent](codec),
	)
	sub.Start(context.Background())

	event := TestEvent{OrderID: "SUB-CUSTOM", Total: 200.00}
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

	if !codec.unmarshalCalled.Load() {
		t.Error("expected custom codec Unmarshal to be called")
	}
}

func TestCodec_RoundTrip(t *testing.T) {
	provider := &mockProvider{}
	subCh := make(chan Result[Message], 1)
	subProvider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	pubSignal := capitan.NewSignal("test.roundtrip.pub", "Test roundtrip pub")
	subSignal := capitan.NewSignal("test.roundtrip.sub", "Test roundtrip sub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	codec := &mockCodec{}

	// Create publisher with custom codec
	pub := NewPublisher(provider, pubSignal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](codec),
	)
	pub.Start()

	// Publish event
	c.Emit(context.Background(), pubSignal, key.Field(TestEvent{
		OrderID: "ROUNDTRIP",
		Total:   999.99,
	}))

	c.Shutdown()
	pub.Close()

	// Get the published data
	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}

	// Now test subscriber receives it correctly
	c2 := capitan.New()
	defer c2.Shutdown()

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	c2.Hook(subSignal, func(_ context.Context, e *capitan.Event) {
		event, ok := key.From(e)
		if ok {
			received = event
			wg.Done()
		}
	})

	sub := NewSubscriber(subProvider, subSignal, key, nil,
		WithSubscriberCapitan[TestEvent](c2),
		WithSubscriberCodec[TestEvent](codec),
	)
	sub.Start(context.Background())

	// Send the published data to subscriber
	subCh <- NewSuccess(newTestMessage(published[0]))

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

	if received.OrderID != "ROUNDTRIP" {
		t.Errorf("expected OrderID 'ROUNDTRIP', got %q", received.OrderID)
	}
	if received.Total != 999.99 {
		t.Errorf("expected Total 999.99, got %v", received.Total)
	}
}

func TestSubscriber_CodecMismatchEmitsError(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.mismatch", "Test codec mismatch")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Codec that always fails to unmarshal
	failCodec := &mockCodec{
		unmarshalFunc: func(_ []byte, _ any) error {
			return errors.New("codec mismatch: invalid format")
		},
	}

	var errorReceived atomic.Bool
	var errorMsg string
	var errorMu sync.Mutex

	c.Hook(ErrorSignal, func(_ context.Context, e *capitan.Event) {
		err, ok := ErrorKey.From(e)
		if ok {
			errorMu.Lock()
			errorMsg = err.Err
			errorMu.Unlock()
			errorReceived.Store(true)
		}
	})

	sub := NewSubscriber(provider, signal, key, nil,
		WithSubscriberCapitan[TestEvent](c),
		WithSubscriberCodec[TestEvent](failCodec),
	)
	sub.Start(context.Background())

	// Send valid JSON that the codec will reject
	data, err := json.Marshal(TestEvent{OrderID: "MISMATCH", Total: 1.0})
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(newTestMessage(data))

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	sub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted to ErrorSignal")
	}

	errorMu.Lock()
	if errorMsg != "codec mismatch: invalid format" {
		t.Errorf("expected error message 'codec mismatch: invalid format', got %q", errorMsg)
	}
	errorMu.Unlock()
}

func TestPublisher_NilCodecDefaultsToJSON(t *testing.T) {
	provider := &mockProvider{}
	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.nil.pub", "Test nil codec pub")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Explicitly set nil codec
	pub := NewPublisher(provider, signal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](nil),
	)
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "NIL-CODEC",
		Total:   42.00,
	}))

	c.Shutdown()
	pub.Close()

	published := provider.Published()
	if len(published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(published))
	}

	// Should be valid JSON (default codec applied)
	var event TestEvent
	if err := json.Unmarshal(published[0], &event); err != nil {
		t.Fatalf("nil codec should default to JSON: %v", err)
	}

	if event.OrderID != "NIL-CODEC" {
		t.Errorf("expected OrderID 'NIL-CODEC', got %q", event.OrderID)
	}
}

func TestSubscriber_NilCodecDefaultsToJSON(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New()
	defer c.Shutdown()

	signal := capitan.NewSignal("test.codec.nil.sub", "Test nil codec sub")
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

	// Explicitly set nil codec
	sub := NewSubscriber(provider, signal, key, nil,
		WithSubscriberCapitan[TestEvent](c),
		WithSubscriberCodec[TestEvent](nil),
	)
	sub.Start(context.Background())

	// Send JSON-encoded message
	event := TestEvent{OrderID: "NIL-SUB", Total: 77.00}
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

	if received.OrderID != "NIL-SUB" {
		t.Errorf("expected OrderID 'NIL-SUB', got %q", received.OrderID)
	}
}

func TestPublisher_MarshalErrorEmitsError(t *testing.T) {
	provider := &mockProvider{}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.marshal.error", "Test marshal error")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Codec that always fails to marshal
	failCodec := &mockCodec{
		marshalFunc: func(_ any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		},
	}

	var errorReceived atomic.Bool

	c.Hook(ErrorSignal, func(_ context.Context, e *capitan.Event) {
		_, ok := ErrorKey.From(e)
		if ok {
			errorReceived.Store(true)
		}
	})

	pub := NewPublisher(provider, signal, key, nil,
		WithPublisherCapitan[TestEvent](c),
		WithPublisherCodec[TestEvent](failCodec),
	)
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{
		OrderID: "FAIL",
		Total:   1.0,
	}))

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	c.Shutdown()
	pub.Close()

	if !errorReceived.Load() {
		t.Error("expected error to be emitted to ErrorSignal on marshal failure")
	}

	// Verify nothing was published
	published := provider.Published()
	if len(published) != 0 {
		t.Errorf("expected 0 published messages on marshal failure, got %d", len(published))
	}
}
