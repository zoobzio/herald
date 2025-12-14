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

func TestPublisher_UseApply_TransformsValue(t *testing.T) {
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
		WithMiddleware(
			UseApply[TestEvent]("add-prefix", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				env.Value.OrderID = "PREFIX-" + env.Value.OrderID
				return env, nil
			}),
		),
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

func TestPublisher_UseApply_ErrorAborts(t *testing.T) {
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
		WithMiddleware(
			UseApply[TestEvent]("validate", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				if env.Value.Total < 0 {
					return env, errors.New("invalid total")
				}
				return env, nil
			}),
		),
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

func TestPublisher_UseEffect_SideEffect(t *testing.T) {
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
		WithMiddleware(
			UseEffect[TestEvent]("log", func(_ context.Context, _ *Envelope[TestEvent]) error {
				effectCalled.Store(true)
				return nil
			}),
		),
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

func TestPublisher_UseEffect_ErrorAborts(t *testing.T) {
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
		WithMiddleware(
			UseEffect[TestEvent]("check", func(_ context.Context, env *Envelope[TestEvent]) error {
				if env.Value.Total < 0 {
					return errors.New("invalid total")
				}
				return nil
			}),
		),
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

func TestPublisher_UseTransform_PureTransform(t *testing.T) {
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
		WithMiddleware(
			UseTransform[TestEvent]("uppercase", func(_ context.Context, env *Envelope[TestEvent]) *Envelope[TestEvent] {
				env.Value.OrderID = "TRANSFORMED-" + env.Value.OrderID
				return env
			}),
		),
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

func TestSubscriber_UseApply_TransformsValue(t *testing.T) {
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
		WithMiddleware(
			UseApply[TestEvent]("add-suffix", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				env.Value.OrderID += "-SUFFIX"
				return env, nil
			}),
		),
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

func TestSubscriber_UseApply_ErrorTriggersNack(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.apply.nack", "Test sub apply nack")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var nacked atomic.Bool

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseApply[TestEvent]("validate", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				if env.Value.Total < 0 {
					return env, errors.New("invalid total")
				}
				return env, nil
			}),
		),
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

func TestPublisher_MiddlewareComposesWithReliability(t *testing.T) {
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
		WithMiddleware(
			UseApply[TestEvent]("count", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				applyCalls.Add(1)
				return env, nil
			}),
		),
		WithRetry[TestEvent](3),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	// Options applied: retry wraps (middleware wraps terminal)
	// So middleware is inside retry, runs on each attempt
	if attempts.Load() != 3 {
		t.Errorf("expected 3 publish attempts, got %d", attempts.Load())
	}
	// Apply runs on each retry attempt
	if applyCalls.Load() != 3 {
		t.Errorf("expected apply to run 3 times (once per attempt), got %d", applyCalls.Load())
	}
}

func TestPublisher_WithMiddleware_ExecutionOrder(t *testing.T) {
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
		WithMiddleware(
			UseEffect[TestEvent]("first", func(_ context.Context, _ *Envelope[TestEvent]) error {
				mu.Lock()
				order = append(order, "first")
				mu.Unlock()
				return nil
			}),
			UseEffect[TestEvent]("second", func(_ context.Context, _ *Envelope[TestEvent]) error {
				mu.Lock()
				order = append(order, "second")
				mu.Unlock()
				return nil
			}),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	// Middleware executes in order: first -> second -> publish
	expected := []string{"first", "second", "publish"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("expected order[%d] = %q, got %q", i, exp, order[i])
		}
	}
}

func TestPublisher_UseMutate_ConditionTrue(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.mutate.true", "Test mutate condition true")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseMutate[TestEvent]("add-prefix",
				func(_ context.Context, env *Envelope[TestEvent]) *Envelope[TestEvent] {
					env.Value.OrderID = "MUTATED-" + env.Value.OrderID
					return env
				},
				func(_ context.Context, env *Envelope[TestEvent]) bool {
					return env.Value.Total > 50
				},
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 100.0}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OrderID != "MUTATED-123" {
		t.Errorf("expected OrderID 'MUTATED-123', got %q", result.OrderID)
	}
}

func TestPublisher_UseMutate_ConditionFalse(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.mutate.false", "Test mutate condition false")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseMutate[TestEvent]("add-prefix",
				func(_ context.Context, env *Envelope[TestEvent]) *Envelope[TestEvent] {
					env.Value.OrderID = "MUTATED-" + env.Value.OrderID
					return env
				},
				func(_ context.Context, env *Envelope[TestEvent]) bool {
					return env.Value.Total > 50
				},
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 10.0}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Condition false, no mutation
	if result.OrderID != "123" {
		t.Errorf("expected OrderID '123', got %q", result.OrderID)
	}
}

func TestPublisher_UseEnrich_Success(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.enrich.success", "Test enrich success")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseEnrich[TestEvent]("enrich", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				env.Value.OrderID = "ENRICHED-" + env.Value.OrderID
				return env, nil
			}),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 50.0}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OrderID != "ENRICHED-123" {
		t.Errorf("expected OrderID 'ENRICHED-123', got %q", result.OrderID)
	}
}

func TestPublisher_UseEnrich_Failure(t *testing.T) {
	var published []byte
	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.enrich.failure", "Test enrich failure")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseEnrich[TestEvent]("enrich", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
				return env, errors.New("enrichment failed")
			}),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 50.0}))

	c.Shutdown()
	pub.Close()

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Enrichment failed, original value preserved
	if result.OrderID != "123" {
		t.Errorf("expected OrderID '123' (original), got %q", result.OrderID)
	}
}

func TestPublisher_WithFilter_ConditionTrue(t *testing.T) {
	var publishCalled atomic.Bool
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			publishCalled.Store(true)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.filter.true", "Test filter condition true")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithFilter[TestEvent]("high-value", func(_ context.Context, env *Envelope[TestEvent]) bool {
			return env.Value.Total > 50
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 100.0}))

	c.Shutdown()
	pub.Close()

	if !publishCalled.Load() {
		t.Error("expected publish to be called when condition is true")
	}
}

func TestPublisher_WithFilter_ConditionFalse(t *testing.T) {
	var publishCalled atomic.Bool
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			publishCalled.Store(true)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.filter.false", "Test filter condition false")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithFilter[TestEvent]("high-value", func(_ context.Context, env *Envelope[TestEvent]) bool {
			return env.Value.Total > 50
		}),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123", Total: 10.0}))

	c.Shutdown()
	pub.Close()

	// Filter condition false - wrapped pipeline (including publish) is skipped
	if publishCalled.Load() {
		t.Error("expected publish to be skipped when filter condition is false")
	}
}

func TestPublisher_WithMiddleware_NestedProcessors(t *testing.T) {
	var attempts atomic.Int32
	var published []byte

	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.middleware.nested", "Test nested middleware")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseRetry(3,
				UseApply[TestEvent]("flaky", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
					if attempts.Add(1) < 3 {
						return env, errors.New("transient")
					}
					env.Value.OrderID = "SUCCESS-" + env.Value.OrderID
					return env, nil
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}

	var result TestEvent
	if err := json.Unmarshal(published, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OrderID != "SUCCESS-123" {
		t.Errorf("expected OrderID 'SUCCESS-123', got %q", result.OrderID)
	}
}

func TestPublisher_Envelope_MetadataAccess(t *testing.T) {
	var publishedMetadata Metadata
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, m Metadata) error {
			publishedMetadata = m
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.envelope.metadata", "Test envelope metadata")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseEffect[TestEvent]("set-headers", func(_ context.Context, env *Envelope[TestEvent]) error {
				env.Metadata["X-Trace-ID"] = "trace-123"
				env.Metadata["X-Correlation-ID"] = "corr-456"
				return nil
			}),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "123"}))

	c.Shutdown()
	pub.Close()

	if publishedMetadata["X-Trace-ID"] != "trace-123" {
		t.Errorf("expected X-Trace-ID 'trace-123', got %q", publishedMetadata["X-Trace-ID"])
	}
	if publishedMetadata["X-Correlation-ID"] != "corr-456" {
		t.Errorf("expected X-Correlation-ID 'corr-456', got %q", publishedMetadata["X-Correlation-ID"])
	}
}

func TestSubscriber_Envelope_MetadataAccess(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.envelope.metadata", "Test subscriber envelope metadata")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var receivedTraceID string
	var wg sync.WaitGroup
	wg.Add(1)

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseEffect[TestEvent]("read-headers", func(_ context.Context, env *Envelope[TestEvent]) error {
				receivedTraceID = env.Metadata["X-Trace-ID"]
				wg.Done()
				return nil
			}),
		),
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	sub.Start(context.Background())

	event := TestEvent{OrderID: "123", Total: 50.0}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	subCh <- NewSuccess(Message{
		Data: data,
		Metadata: Metadata{
			"X-Trace-ID": "trace-from-broker",
		},
		Ack: func() error { return nil },
	})

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

	if receivedTraceID != "trace-from-broker" {
		t.Errorf("expected X-Trace-ID 'trace-from-broker', got %q", receivedTraceID)
	}
}

func TestPublisher_UseBackoff(t *testing.T) {
	var attempts atomic.Int32
	var published []byte

	provider := &mockProvider{
		publishFunc: func(_ context.Context, data []byte, _ Metadata) error {
			published = data
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.backoff", "Test UseBackoff")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseBackoff(3, 1*time.Millisecond,
				UseApply[TestEvent]("flaky", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
					if attempts.Add(1) < 3 {
						return env, errors.New("transient")
					}
					return env, nil
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "backoff-test"}))

	c.Shutdown()
	pub.Close()

	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
	if published == nil {
		t.Error("expected message to be published after retries")
	}
}

func TestPublisher_UseTimeout(t *testing.T) {
	var timedOut atomic.Bool

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.timeout", "Test UseTimeout")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseTimeout(50*time.Millisecond,
				UseApply[TestEvent]("slow", func(ctx context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
					select {
					case <-ctx.Done():
						timedOut.Store(true)
						return env, ctx.Err()
					case <-time.After(time.Second):
						return env, nil
					}
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "timeout-test"}))

	c.Shutdown()
	pub.Close()

	if !timedOut.Load() {
		t.Error("expected operation to time out")
	}
}

func TestPublisher_UseFallback(t *testing.T) {
	var primaryCalled atomic.Bool
	var fallbackCalled atomic.Bool

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.fallback", "Test UseFallback")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseFallback(
				UseApply[TestEvent]("primary", func(_ context.Context, env *Envelope[TestEvent]) (*Envelope[TestEvent], error) {
					primaryCalled.Store(true)
					return env, errors.New("primary failed")
				}),
				UseEffect[TestEvent]("fallback", func(_ context.Context, _ *Envelope[TestEvent]) error {
					fallbackCalled.Store(true)
					return nil
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "fallback-test"}))

	c.Shutdown()
	pub.Close()

	if !primaryCalled.Load() {
		t.Error("expected primary to be called")
	}
	if !fallbackCalled.Load() {
		t.Error("expected fallback to be called after primary failed")
	}
}

func TestPublisher_UseFilter(t *testing.T) {
	var processorCalled atomic.Bool

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.filter", "Test UseFilter")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseFilter("high-value",
				func(_ context.Context, env *Envelope[TestEvent]) bool {
					return env.Value.Total > 100
				},
				UseEffect[TestEvent]("process", func(_ context.Context, _ *Envelope[TestEvent]) error {
					processorCalled.Store(true)
					return nil
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	// Low value - should skip processor
	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "low", Total: 50.0}))

	c.Shutdown()
	pub.Close()

	if processorCalled.Load() {
		t.Error("expected processor to be skipped for low-value order")
	}
}

func TestPublisher_UseFilter_ConditionTrue(t *testing.T) {
	var processorCalled atomic.Bool

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.filter.true", "Test UseFilter true")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseFilter("high-value",
				func(_ context.Context, env *Envelope[TestEvent]) bool {
					return env.Value.Total > 100
				},
				UseEffect[TestEvent]("process", func(_ context.Context, _ *Envelope[TestEvent]) error {
					processorCalled.Store(true)
					return nil
				}),
			),
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	// High value - should call processor
	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "high", Total: 200.0}))

	c.Shutdown()
	pub.Close()

	if !processorCalled.Load() {
		t.Error("expected processor to be called for high-value order")
	}
}

func TestPublisher_UseRateLimit(t *testing.T) {
	var published atomic.Int32

	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			published.Add(1)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.use.ratelimit", "Test UseRateLimit")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithMiddleware(
			UseRateLimit[TestEvent](100, 5), // High rate to not block test
		),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start()

	for i := 0; i < 3; i++ {
		c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "rate-test"}))
	}

	c.Shutdown()
	pub.Close()

	if published.Load() != 3 {
		t.Errorf("expected 3 published, got %d", published.Load())
	}
}
