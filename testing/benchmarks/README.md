# Benchmark Suite for herald

This directory contains performance benchmarks for the herald package, measuring throughput, latency, and memory allocation patterns for event bridging operations.

## Benchmark Categories

### Core Performance (`core_performance_test.go`)
Benchmarks fundamental herald operations:
- **Publisher Throughput**: Emit-to-publish latency
- **Pipeline Options**: Retry, timeout overhead
- **JSON Codec**: Marshal/unmarshal performance
- **Result Type**: Success/error creation overhead
- **Message Sizes**: Performance across payload sizes
- **Concurrent Emit**: Parallel emission scalability

## Running Benchmarks

### All Benchmarks
```bash
go test -v -bench=. ./testing/benchmarks/...
```

### With Memory Allocation Tracking
```bash
go test -v -bench=. -benchmem ./testing/benchmarks/...
```

### Multiple Runs for Statistical Significance
```bash
go test -v -bench=. -count=5 ./testing/benchmarks/...
```

### Specific Benchmarks
```bash
# Publisher benchmarks
go test -v -bench=BenchmarkPublisher ./testing/benchmarks/...

# Codec benchmarks
go test -v -bench=BenchmarkJSONCodec ./testing/benchmarks/...

# Concurrent benchmarks
go test -v -bench=BenchmarkPublisher_ConcurrentEmit ./testing/benchmarks/...
```

### Performance Profiling
```bash
# CPU profiling
go test -bench=BenchmarkPublisher -cpuprofile=cpu.prof ./testing/benchmarks/...
go tool pprof cpu.prof

# Memory profiling
go test -bench=BenchmarkPublisher -memprofile=mem.prof ./testing/benchmarks/...
go tool pprof mem.prof
```

## Benchmark Metrics

### Performance Targets
Herald aims for minimal overhead over raw broker operations:
- **Publisher emit**: < 1µs/op overhead
- **JSON marshal**: Comparable to stdlib encoding/json
- **Result operations**: < 10ns/op, 0 allocs

### Zero-Allocation Paths
These operations should have zero allocations:
- Result.IsSuccess() / IsError()
- Result.Value() / Error()

## Interpreting Results

```
BenchmarkPublisher-8    500000    2500 ns/op    256 B/op    4 allocs/op
```
- **Name**: BenchmarkPublisher-8 (8 CPU cores)
- **Iterations**: 500,000 runs
- **Time**: 2500 nanoseconds per operation
- **Memory**: 256 bytes allocated per operation
- **Allocations**: 4 allocations per operation

## Performance Regression Detection

```bash
# Save baseline
go test -bench=. -benchmem ./testing/benchmarks/... > baseline.txt

# After changes
go test -bench=. -benchmem ./testing/benchmarks/... > current.txt

# Compare with benchstat
benchstat baseline.txt current.txt
```

## Adding New Benchmarks

### Naming Convention
- `BenchmarkComponent_Scenario`
- Examples: `BenchmarkPublisher_WithRetry`, `BenchmarkJSONCodec_Marshal`

### Structure Pattern
```go
func BenchmarkPublisher_Scenario(b *testing.B) {
    // Setup (not measured)
    c := capitan.New()
    defer c.Shutdown()

    provider := &mockProvider{}
    pub := herald.NewPublisher(provider, sig, key, nil,
        herald.WithPublisherCapitan[T](c))
    pub.Start(context.Background())
    defer pub.Close()

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        // Operation being measured
    }

    c.Shutdown() // Drain before next iteration
}
```

### Best Practices
- Use isolated capitan instances (`capitan.New()`)
- Call `c.Shutdown()` to drain queues before measurement ends
- Use `b.ResetTimer()` after setup
- Include `b.ReportAllocs()` for memory tracking
- Use `b.RunParallel()` for concurrency benchmarks

## Benchmark Environment

### Requirements
- Go 1.23+
- Stable system load
- Consistent CPU frequency (disable scaling if needed)

### Reproducibility Tips
```bash
# Set CPU governor to performance
echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor

# Run with elevated priority
nice -n -10 go test -bench=...
```

---

The benchmark suite ensures herald maintains low overhead while bridging events between capitan and external message brokers.
