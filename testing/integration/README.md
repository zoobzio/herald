# Integration Tests for herald

This directory contains integration tests that verify how herald components work together with capitan and real-world bridging scenarios.

## Test Categories

### Default Capitan Tests (`default_capitan_test.go`)
Tests herald's integration with the global capitan singleton:
- Subscriber emitting to default capitan instance
- Publisher observing from default capitan instance
- End-to-end message flow through global event system

## Running Integration Tests

### All Integration Tests
```bash
go test -v ./testing/integration/...
```

### With Race Detection
```bash
go test -v -race ./testing/integration/...
```

### Specific Tests
```bash
# Test default capitan integration
go test -v -run TestSubscriber_DefaultCapitanEmit ./testing/integration/...
go test -v -run TestPublisher_DefaultCapitanObserve ./testing/integration/...
```

### With Extended Timeout
```bash
go test -v -timeout=5m ./testing/integration/...
```

## Test Patterns

### Common Signal Patterns
- Bridge events: `integration.default.emit`, `integration.default.pub`
- Provider events: `integration.kafka`, `integration.redis`
- Error events: `integration.error`

### Common Message Types
```go
type TestEvent struct {
    OrderID string  `json:"order_id"`
    Total   float64 `json:"total"`
}
```

## Guidelines for Adding Integration Tests

### When to Add Integration Tests
- Testing herald + capitan interactions
- Testing publisher/subscriber coordination
- Verifying metadata propagation end-to-end
- Testing error signal flows

### Test Structure
1. **Setup**: Create provider, signal, key, and hook listeners
2. **Execute**: Start publisher/subscriber and emit events
3. **Verify**: Check event reception, field values, and state
4. **Cleanup**: Cancel context, close resources

### Naming Conventions
- `TestSubscriber_<Scenario>`
- `TestPublisher_<Scenario>`
- `TestBridge_<Pattern>`

### Best Practices
- Use `sync.WaitGroup` for async coordination
- Include timeout protection with `time.After`
- Clean up with `defer` for context cancellation and Close()
- Use unique signal names to avoid test interference

## Integration Test Dependencies

### Required
- Go 1.23+
- capitan package
- Context support for cancellation

### Optional
- Race detector for concurrency testing

## Performance Considerations

- Integration tests should complete in seconds
- Use buffered channels to avoid blocking
- Focus on correctness over throughput
- Include timeout assertions to catch hangs

---

Integration tests ensure herald correctly bridges capitan events to external providers and back.
