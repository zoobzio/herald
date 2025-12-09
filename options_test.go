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
	"github.com/zoobzio/pipz"
)

// Note: errors import used for TestPublisher_WithBackoff

func TestPublisher_WithBackoff(t *testing.T) {
	var attempts atomic.Int32
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

	signal := capitan.NewSignal("test.backoff", "Test backoff")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithBackoff[TestEvent](3, 10*time.Millisecond),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "retry-test"}))

	c.Shutdown()
	pub.Close()

	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestPublisher_WithTimeout(t *testing.T) {
	provider := &mockProvider{
		publishFunc: func(ctx context.Context, _ []byte, _ Metadata) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return nil
			}
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.timeout", "Test timeout")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	opts := []Option[TestEvent]{
		WithTimeout[TestEvent](50 * time.Millisecond),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	start := time.Now()
	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "timeout-test"}))
	elapsed := time.Since(start)

	c.Shutdown()
	pub.Close()

	// Should timeout quickly, not wait full second
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected timeout within 200ms, took %v", elapsed)
	}
}

func TestPublisher_WithRateLimit(t *testing.T) {
	var published atomic.Int32
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			published.Add(1)
			return nil
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.ratelimit", "Test rate limit")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// 10 per second, burst of 2
	opts := []Option[TestEvent]{
		WithRateLimit[TestEvent](10, 2),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	// Emit 5 events quickly
	for i := 0; i < 5; i++ {
		c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "rate-test"}))
	}

	c.Shutdown()
	pub.Close()

	// All should eventually be published (rate limiter waits by default)
	if published.Load() != 5 {
		t.Errorf("expected 5 published, got %d", published.Load())
	}
}

func TestSubscriber_WithTimeout(t *testing.T) {
	subCh := make(chan Result[Message], 1)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.timeout", "Test sub timeout")
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
		WithTimeout[TestEvent](5 * time.Second), // Long timeout, should succeed
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	event := TestEvent{OrderID: "sub-timeout", Total: 1.0}
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

	if received.OrderID != "sub-timeout" {
		t.Errorf("expected OrderID 'sub-timeout', got %q", received.OrderID)
	}
}

func TestSubscriber_WithRateLimit(t *testing.T) {
	subCh := make(chan Result[Message], 5)
	provider := &mockProvider{subCh: subCh}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.sub.ratelimit", "Test sub rate limit")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	var received atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	c.Hook(signal, func(_ context.Context, e *capitan.Event) {
		_, ok := key.From(e)
		if ok {
			received.Add(1)
			wg.Done()
		}
	})

	// 10 per second, burst of 2
	opts := []Option[TestEvent]{
		WithRateLimit[TestEvent](10, 2),
	}

	sub := NewSubscriber(provider, signal, key, opts, WithSubscriberCapitan[TestEvent](c))
	ctx, cancel := context.WithCancel(context.Background())
	sub.Start(ctx)

	// Send 3 events
	for i := 0; i < 3; i++ {
		event := TestEvent{OrderID: "rate-test", Total: float64(i)}
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("failed to marshal event: %v", err)
		}
		subCh <- NewSuccess(newTestMessage(data))
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for events")
	}

	cancel()
	sub.Close()

	if received.Load() != 3 {
		t.Errorf("expected 3 received, got %d", received.Load())
	}
}

func TestWithPipeline_CustomPipeline(t *testing.T) {
	var transformed atomic.Bool
	provider := &mockProvider{}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.custom", "Test custom pipeline")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Custom pipeline that marks transformation
	custom := pipz.Transform[TestEvent]("mark", func(_ context.Context, e TestEvent) TestEvent {
		transformed.Store(true)
		return e
	})

	opts := []Option[TestEvent]{
		WithPipeline[TestEvent](custom),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "custom"}))

	c.Shutdown()
	pub.Close()

	if !transformed.Load() {
		t.Error("expected custom pipeline to be invoked")
	}
}

func TestPublisher_WithCircuitBreaker(t *testing.T) {
	var attempts atomic.Int32
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			attempts.Add(1)
			return errors.New("always fails")
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.circuit", "Test circuit breaker")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Circuit opens after 2 failures, recovers after 50ms
	opts := []Option[TestEvent]{
		WithCircuitBreaker[TestEvent](2, 50*time.Millisecond),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	// Emit 5 events - first 2 should fail and open circuit, rest should be rejected
	for i := 0; i < 5; i++ {
		c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "circuit-test"}))
	}

	c.Shutdown()
	pub.Close()

	// Only 2 attempts should reach the provider (circuit opens after 2 failures)
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts before circuit opened, got %d", attempts.Load())
	}
}

func TestPublisher_WithErrorHandler(t *testing.T) {
	var handledErrors atomic.Int32
	provider := &mockProvider{
		publishFunc: func(_ context.Context, _ []byte, _ Metadata) error {
			return errors.New("publish failed")
		},
	}

	c := capitan.New(capitan.WithSyncMode())
	defer c.Shutdown()

	signal := capitan.NewSignal("test.errorhandler", "Test error handler")
	key := capitan.NewKey[TestEvent]("payload", "test.event")

	// Error handler that counts errors
	errorHandler := pipz.Effect("count-errors", func(_ context.Context, err *pipz.Error[TestEvent]) error {
		handledErrors.Add(1)
		return nil
	})

	opts := []Option[TestEvent]{
		WithErrorHandler[TestEvent](errorHandler),
	}

	pub := NewPublisher(provider, signal, key, opts, WithPublisherCapitan[TestEvent](c))
	pub.Start(context.Background())

	c.Emit(context.Background(), signal, key.Field(TestEvent{OrderID: "error-test"}))

	c.Shutdown()
	pub.Close()

	if handledErrors.Load() != 1 {
		t.Errorf("expected 1 handled error, got %d", handledErrors.Load())
	}
}
