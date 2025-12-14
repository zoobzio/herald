package benchmarks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/herald"
)

type benchMessage struct {
	ID      string  `json:"id"`
	Value   string  `json:"value"`
	Counter int     `json:"counter"`
	Amount  float64 `json:"amount"`
}

// mockProvider is a minimal provider for benchmarking.
type mockProvider struct {
	publishCount int
}

func (m *mockProvider) Publish(_ context.Context, _ []byte, _ herald.Metadata) error {
	m.publishCount++
	return nil
}

func (*mockProvider) Subscribe(_ context.Context) <-chan herald.Result[herald.Message] {
	ch := make(chan herald.Result[herald.Message])
	close(ch)
	return ch
}

func (*mockProvider) Ping(_ context.Context) error {
	return nil
}

func (*mockProvider) Close() error {
	return nil
}

// BenchmarkPublisher measures publisher throughput.
func BenchmarkPublisher(b *testing.B) {
	c := capitan.New()
	defer c.Shutdown()

	sig := capitan.NewSignal("bench.publish", "Benchmark publish signal")
	key := capitan.NewKey[benchMessage]("msg", "bench.Message")

	provider := &mockProvider{}

	pub := herald.NewPublisher(provider, sig, key, nil,
		herald.WithPublisherCapitan[benchMessage](c))
	pub.Start()
	defer pub.Close()

	msg := benchMessage{
		ID:      "bench-001",
		Value:   "benchmark test value",
		Counter: 42,
		Amount:  99.99,
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.Emit(ctx, sig, key.Field(msg))
	}

	c.Shutdown()
}

// BenchmarkPublisher_WithRetry measures publisher with retry option.
func BenchmarkPublisher_WithRetry(b *testing.B) {
	c := capitan.New()
	defer c.Shutdown()

	sig := capitan.NewSignal("bench.publish.retry", "Benchmark publish retry signal")
	key := capitan.NewKey[benchMessage]("msg", "bench.Message")

	provider := &mockProvider{}

	pub := herald.NewPublisher(provider, sig, key,
		[]herald.Option[benchMessage]{
			herald.WithRetry[benchMessage](3),
		},
		herald.WithPublisherCapitan[benchMessage](c))
	pub.Start()
	defer pub.Close()

	msg := benchMessage{ID: "bench-001", Value: "test"}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.Emit(ctx, sig, key.Field(msg))
	}

	c.Shutdown()
}

// BenchmarkPublisher_WithTimeout measures publisher with timeout option.
func BenchmarkPublisher_WithTimeout(b *testing.B) {
	c := capitan.New()
	defer c.Shutdown()

	sig := capitan.NewSignal("bench.publish.timeout", "Benchmark publish timeout signal")
	key := capitan.NewKey[benchMessage]("msg", "bench.Message")

	provider := &mockProvider{}

	pub := herald.NewPublisher(provider, sig, key,
		[]herald.Option[benchMessage]{
			herald.WithTimeout[benchMessage](5 * time.Second),
		},
		herald.WithPublisherCapitan[benchMessage](c))
	pub.Start()
	defer pub.Close()

	msg := benchMessage{ID: "bench-001", Value: "test"}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		c.Emit(ctx, sig, key.Field(msg))
	}

	c.Shutdown()
}

// BenchmarkJSONCodec_Marshal measures JSON codec marshaling.
func BenchmarkJSONCodec_Marshal(b *testing.B) {
	codec := herald.JSONCodec{}

	msg := benchMessage{
		ID:      "bench-001",
		Value:   "benchmark test value with some longer content to simulate real data",
		Counter: 42,
		Amount:  99.99,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = codec.Marshal(msg)
	}
}

// BenchmarkJSONCodec_Unmarshal measures JSON codec unmarshaling.
func BenchmarkJSONCodec_Unmarshal(b *testing.B) {
	codec := herald.JSONCodec{}

	msg := benchMessage{
		ID:      "bench-001",
		Value:   "benchmark test value with some longer content to simulate real data",
		Counter: 42,
		Amount:  99.99,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		b.Fatalf("failed to marshal test message: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var result benchMessage
		_ = codec.Unmarshal(data, &result)
	}
}

// BenchmarkResult measures Result type operations.
func BenchmarkResult(b *testing.B) {
	b.Run("NewSuccess", func(b *testing.B) {
		msg := herald.Message{Data: []byte("test")}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = herald.NewSuccess(msg)
		}
	})

	b.Run("NewError", func(b *testing.B) {
		err := herald.ErrNoWriter

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = herald.NewError[herald.Message](err)
		}
	})

	b.Run("IsSuccess", func(b *testing.B) {
		result := herald.NewSuccess(herald.Message{Data: []byte("test")})

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = result.IsSuccess()
		}
	})

	b.Run("Value", func(b *testing.B) {
		result := herald.NewSuccess(herald.Message{Data: []byte("test")})

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = result.Value()
		}
	})
}

// BenchmarkPublisher_MessageSizes measures publisher with different message sizes.
func BenchmarkPublisher_MessageSizes(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			c := capitan.New()
			defer c.Shutdown()

			sig := capitan.NewSignal("bench.size."+sz.name, "Benchmark size signal")
			key := capitan.NewKey[[]byte]("data", "bench.Data")

			provider := &mockProvider{}

			pub := herald.NewPublisher(provider, sig, key, nil,
				herald.WithPublisherCapitan[[]byte](c))
			pub.Start()
			defer pub.Close()

			data := make([]byte, sz.size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			ctx := context.Background()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				c.Emit(ctx, sig, key.Field(data))
			}

			c.Shutdown()
		})
	}
}

// BenchmarkPublisher_ConcurrentEmit measures concurrent emission performance.
func BenchmarkPublisher_ConcurrentEmit(b *testing.B) {
	c := capitan.New()
	defer c.Shutdown()

	sig := capitan.NewSignal("bench.concurrent", "Benchmark concurrent signal")
	key := capitan.NewKey[benchMessage]("msg", "bench.Message")

	provider := &mockProvider{}

	pub := herald.NewPublisher(provider, sig, key, nil,
		herald.WithPublisherCapitan[benchMessage](c))
	pub.Start()
	defer pub.Close()

	msg := benchMessage{ID: "bench-001", Value: "test"}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Emit(ctx, sig, key.Field(msg))
		}
	})

	c.Shutdown()
}
