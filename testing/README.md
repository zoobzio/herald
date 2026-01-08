# Testing Infrastructure for herald

This directory contains the comprehensive testing infrastructure for the herald package, designed to support robust development and maintenance of the event bridging system.

## Directory Structure

```
testing/
├── README.md             # This file - testing strategy overview
├── helpers.go            # Shared test utilities and mocks
├── integration/          # Integration and end-to-end tests
│   └── default_capitan_test.go  # Default capitan instance tests
└── benchmarks/          # Performance benchmarks
    └── core_performance_test.go # Core component benchmarks
```

## Testing Strategy

### Unit Tests (Root Package)
- **Location**: Alongside source files (`*_test.go`)
- **Purpose**: Test individual components in isolation
- **Scope**: Publisher, Subscriber, Codec, Options, Result

### Provider Tests (Provider Directories)
- **Location**: Within each provider package
- **Purpose**: Test provider implementations with real brokers
- **Technology**: testcontainers for database/broker setup
- **Coverage**: Kafka, NATS, Pub/Sub, Redis, SQS, AMQP, SQL, BoltDB, Firestore, io

### Integration Tests (`testing/integration/`)
- **Purpose**: Test component interactions and real-world bridging patterns
- **Scope**: Publisher/Subscriber coordination, default capitan usage
- **Focus**: Ensure herald and capitan work together correctly

### Benchmarks (`testing/benchmarks/`)
- **Purpose**: Measure and track performance characteristics
- **Scope**: Publish/subscribe throughput, codec overhead, pipeline options
- **Focus**: Prevent performance regressions and identify bottlenecks

### Test Helpers (`testing/helpers.go`)
- **Purpose**: Provide reusable testing utilities for herald users
- **Scope**: Mock providers, message builders, assertion helpers
- **Focus**: Make testing herald-based applications easier

## Running Tests

### All Tests
```bash
# Run all tests with coverage
go test -v -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -html=coverage.out
```

### Unit Tests Only
```bash
# Run only unit tests (root package)
go test -v .
```

### Provider Tests
```bash
# Run provider tests (requires Docker for testcontainers)
cd kafka && go test -v ./...
cd redis && go test -v ./...
cd sql && go test -v ./...
```

### Integration Tests Only
```bash
# Run integration tests
go test -v ./testing/integration/...

# With race detection
go test -v -race ./testing/integration/...
```

### Benchmarks Only
```bash
# Run all benchmarks
go test -v -bench=. ./testing/benchmarks/...

# Run with memory allocation tracking
go test -v -bench=. -benchmem ./testing/benchmarks/...

# Run multiple times for statistical significance
go test -v -bench=. -count=5 ./testing/benchmarks/...
```

### Performance Monitoring
```bash
# Compare benchmarks over time
go test -bench=. -count=5 ./testing/benchmarks/... > current.txt
# ... after changes ...
go test -bench=. -count=5 ./testing/benchmarks/... > new.txt
benchstat current.txt new.txt
```

## Test Dependencies

### Required for Basic Testing
- Go 1.23+
- Standard library only

### Required for Provider Testing
- Docker (for testcontainers)
- testcontainers-go

### Required for Advanced Testing
- Race detector: `go test -race`
- Coverage tools: `go tool cover`

### Optional Performance Tools
- `benchstat` for comparing benchmark results
- `pprof` for performance profiling

## Best Practices

### Test Organization
1. **Hierarchical naming**: Use descriptive test names that form a hierarchy
2. **Consistent structure**: Follow the same pattern across all test files
3. **Isolated tests**: Each test should be completely independent
4. **Fast tests**: Keep unit tests fast, put slow tests in integration

### Herald Testing
1. **Test both publish and subscribe paths**
2. **Verify metadata propagation**
3. **Test Ack/Nack behavior per provider**
4. **Include error signal handling**

### Concurrency Testing
1. **Use `-race` flag regularly**
2. **Test concurrent Publish/Subscribe operations**
3. **Verify proper cleanup in failure scenarios**
4. **Test context cancellation**

### Performance Testing
1. **Benchmark realistic message patterns**
2. **Include both CPU and memory allocation metrics**
3. **Test pipeline option overhead**
4. **Compare codec implementations**

## Continuous Integration

### Pre-commit Checks
```bash
# Run this before committing
go test ./...           # All tests
go test -race ./...     # Race detection
golangci-lint run       # Code quality
```

### CI Pipeline Requirements
- All tests must pass
- No race conditions detected
- Benchmarks must not regress significantly

---

This testing infrastructure ensures herald remains reliable, performant, and easy to use while providing comprehensive tools for users to test their own herald-based applications.
